// Package pill_inventory_service 丹方与消耗性金丹库存服务（金丹消耗品重构）
// 职责：SaveRecipe/CraftOne/Consume/ConfirmFusion 等写操作的本地事务编排；
// 所有写操作经 operation.go 的幂等包装执行，业务函数只接收事务句柄。
// 构造函数显式注入数据库与时钟（plan §3.1）：测试可固定 now。
package pill_inventory_service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/pill_service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Inventory 丹方与库存服务；只负责本地事务（预览模型调用在 fusion_service）
type Inventory struct {
	db  *gorm.DB
	now func() time.Time
}

// New 构造库存服务；now 传 nil 时用 time.Now
func New(db *gorm.DB, now func() time.Time) *Inventory {
	if now == nil {
		now = time.Now
	}
	return &Inventory{db: db, now: now}
}

// ---------- 请求校验 ----------

// validateRecipeDraft 丹方草稿基础校验：名称非空 + 共享 schema 校验。
// 校验逻辑唯一事实源在 pill_service.ValidateSkillSchema（禁止复制分叉）。
func validateRecipeDraft(draft service.RecipeDraft) errors.Error {
	if strings.TrimSpace(draft.Name) == "" {
		return errors.New(errors.ErrorTypeInvalidRequest, "recipe.invalid_schema", "丹方名称不能为空")
	}
	if err := pill_service.ValidateSkillSchema(draft.SkillSchema); err != nil {
		// 共享校验逻辑，错误码按本域映射（HTTP 400 recipe.invalid_schema）
		return errors.New(errors.ErrorTypeInvalidRequest, "recipe.invalid_schema", err.Error())
	}
	return nil
}

// ---------- 幂等负载哈希 ----------

// payloadHash 操作种类 + 标准化参数的 SHA-256（§3.1）
// 参数顺序固定；JSON 由 encoding/json 对 map 按键排序输出，保证确定性
func payloadHash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// canonicalJSON 结构化字段的确定性序列化（Go map 递归按键排序）
func canonicalJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(raw)
}

// FusionInputHash 融合输入集合哈希（§3.3）：排序后材料 UUID 列表的 SHA-256。
// 预览时由 fusion_service 计算持久化，确认时重算核对输入集合未变。
func FusionInputHash(itemUUIDs []uuid.UUID) string {
	ids := append([]uuid.UUID(nil), itemUUIDs...)
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = id.String()
	}
	return payloadHash("fusion_input", strings.Join(parts, ","))
}

// ---------- 结果序列化（ResultJSON 契约） ----------

// resultToJSON 成功操作结果 → 持久化 JSONMap（UUID 存字符串）
func resultToJSON(r *service.PillOperationResult) model.JSONMap {
	m := model.JSONMap{"operation_id": r.OperationID.String()}
	if r.RecipeID != nil {
		m["recipe_id"] = r.RecipeID.String()
	}
	if r.RevisionID != nil {
		m["revision_id"] = r.RevisionID.String()
	}
	if r.EffectID != nil {
		m["effect_id"] = r.EffectID.String()
	}
	if len(r.ItemIDs) > 0 {
		m["item_ids"] = uuidStrings(r.ItemIDs)
	}
	if len(r.ConsumedItemIDs) > 0 {
		m["consumed_item_ids"] = uuidStrings(r.ConsumedItemIDs)
	}
	return m
}

// resultFromJSON 持久化结果 → DTO（GetOperation 断线恢复用）
func resultFromJSON(m model.JSONMap) (*service.PillOperationResult, errors.Error) {
	r := &service.PillOperationResult{}
	if v, ok := m["operation_id"].(string); ok {
		r.OperationID, _ = uuid.Parse(v)
	}
	if v, ok := m["recipe_id"].(string); ok {
		if u, err := uuid.Parse(v); err == nil {
			r.RecipeID = &u
		}
	}
	if v, ok := m["revision_id"].(string); ok {
		if u, err := uuid.Parse(v); err == nil {
			r.RevisionID = &u
		}
	}
	if v, ok := m["effect_id"].(string); ok {
		if u, err := uuid.Parse(v); err == nil {
			r.EffectID = &u
		}
	}
	r.ItemIDs = parseUUIDList(m["item_ids"])
	r.ConsumedItemIDs = parseUUIDList(m["consumed_item_ids"])
	return r, nil
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

func parseUUIDList(v any) []uuid.UUID {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]uuid.UUID, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			if u, err := uuid.Parse(s); err == nil {
				out = append(out, u)
			}
		}
	}
	return out
}

// ---------- 模型 UUID 文本 → 接口契约 uuid.UUID 转换 ----------
// 主键统一 uuid 文本后,模型字段为 string,而 service 接口契约(PillOperationResult 等)
// 仍以 uuid.UUID 承载对外标识;以下两个 helper 只在边界做一次文本→UUID 回填。
// 模型主键恒由 BeforeCreate 以 uuid.New().String() 生成,正常路径必可解析。

// uuidVal UUID 文本 → uuid.UUID(解析失败返回 uuid.Nil,DTO 字段退化为零值)
func uuidVal(s string) uuid.UUID {
	u, _ := uuid.Parse(s)
	return u
}

// uuidPtr UUID 文本 → *uuid.UUID(解析失败返回 nil,DTO 字段省略)
func uuidPtr(s string) *uuid.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &u
}
