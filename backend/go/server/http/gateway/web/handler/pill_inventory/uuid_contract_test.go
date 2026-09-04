// UUID 业务键 HTTP 契约锁定测试(011 重构 Task 8)
// 契约: JSON 响应中所有名为 id 或以 _id 结尾的业务字段必须是 UUID 字符串
// (uuid.Parse 通过);响应不得出现 uuid 键,也不得出现数字内部主键
// (BIGSERIAL id 泄漏,JSON 解码后即 float64)。
// 覆盖: 丹方列表、金丹实例列表,以及道人能力快照关系面
// GET /api/v1/agents/:uuid/effects(011 Task 8 旧入口审计后,道人详情不再内嵌
// 金丹/能力快照,该端点是能力快照的唯一 HTTP 出口,契约由本包锁定)。
// 夹具: 复用同包既有 setupTestDB/setupRouter/doJSON/seedRecipeAndItem/minSchema,
// 真实 sqlite 内存库,关系字段一律用父实体 .UUID.String()(禁用内部 .ID)。
package pill_inventory

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// seedContractRecipeAndItem 直落库: 丹方 + v1 不可变版本(回填当前版本) +
// 成功操作 + 可用实例,返回 (丹方UUID, 版本UUID, 实例UUID)。
// 关系列(RecipeID/CurrentRevisionID/RecipeRevisionID/OriginOperationID)一律
// 取父实体 UUID 文本(011 契约: 禁用内部 .ID)。
func seedContractRecipeAndItem(t *testing.T, db *gorm.DB) (string, string, string) {
	t.Helper()
	recipe := model.PillRecipe{}
	if err := db.Create(&recipe).Error; err != nil {
		t.Fatalf("建丹方失败: %v", err)
	}
	rev := model.PillRecipeRevision{
		RecipeID:    recipe.UUID.String(),
		Revision:    1,
		Name:        "契约丹方",
		SkillSchema: minSchema(),
	}
	if err := db.Create(&rev).Error; err != nil {
		t.Fatalf("建丹方版本失败: %v", err)
	}
	if err := db.Model(&model.PillRecipe{}).Where("uuid = ?", recipe.UUID).
		Update("current_revision_id", rev.UUID.String()).Error; err != nil {
		t.Fatalf("回填当前版本失败: %v", err)
	}
	op := model.PillOperation{Kind: "craft", PayloadHash: "sha256:contract", ResultJSON: model.JSONMap{}}
	if err := db.Create(&op).Error; err != nil {
		t.Fatalf("建成功操作失败: %v", err)
	}
	item := model.PillItem{
		RecipeRevisionID:  rev.UUID.String(),
		State:             model.PillAvailable,
		OriginOperationID: op.UUID.String(),
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("建金丹实例失败: %v", err)
	}
	return recipe.UUID.String(), rev.UUID.String(), item.UUID.String()
}

// TestUUIDContractRecipeList 丹方列表: id/current_revision_id 为业务 UUID,
// 整个响应包络(含嵌套)无 uuid 键、无数字 id
func TestUUIDContractRecipeList(t *testing.T) {
	db := setupTestDB(t)
	recipeUUID, revisionUUID, _ := seedContractRecipeAndItem(t, db)
	r := setupRouter()

	status, envelope := doJSON(t, r, http.MethodGet, "/api/v1/recipes", "", "")
	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}

	data, _ := envelope["data"].(map[string]interface{})
	items, ok := data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("丹方列表应含 1 条: %v", envelope)
	}
	first, _ := items[0].(map[string]interface{})
	if first["id"] != recipeUUID {
		t.Fatalf("丹方列表 id 应为业务 UUID %s, 实际 %v", recipeUUID, first["id"])
	}
	if first["current_revision_id"] != revisionUUID {
		t.Fatalf("current_revision_id 应为业务 UUID %s, 实际 %v", revisionUUID, first["current_revision_id"])
	}

	assertUUIDContract(t, envelope, "GET /recipes")
}

// TestUUIDContractPillItemsList 金丹实例列表: id/recipe_id/revision_id 均为业务 UUID
func TestUUIDContractPillItemsList(t *testing.T) {
	db := setupTestDB(t)
	recipeUUID, revisionUUID, itemUUID := seedContractRecipeAndItem(t, db)
	r := setupRouter()

	status, envelope := doJSON(t, r, http.MethodGet, "/api/v1/pill-items", "", "")
	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}

	data, _ := envelope["data"].(map[string]interface{})
	items, ok := data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("实例列表应含 1 条: %v", envelope)
	}
	first, _ := items[0].(map[string]interface{})
	if first["id"] != itemUUID {
		t.Fatalf("实例 id 应为业务 UUID %s, 实际 %v", itemUUID, first["id"])
	}
	if first["recipe_id"] != recipeUUID || first["revision_id"] != revisionUUID {
		t.Fatalf("来源关系应为业务 UUID (recipe=%s revision=%s), 实际 %v/%v",
			recipeUUID, revisionUUID, first["recipe_id"], first["revision_id"])
	}

	assertUUIDContract(t, envelope, "GET /pill-items")
}

// TestUUIDContractAgentEffects 道人能力快照关系面: 真实服用链路后,服用响应与
// GET /effects 的 id/item_id/revision_id 均为业务 UUID(实例消耗后仍指向原实例)
func TestUUIDContractAgentEffects(t *testing.T) {
	db := setupTestDB(t)
	agentUUID, itemUUID := seedRecipeAndItem(t, db)
	r := setupRouter()

	status, envelope := doJSON(t, r, http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/consume", agentUUID),
		fmt.Sprintf(`{"item_id":%q,"weight":2,"sort_order":1}`, itemUUID), uuid.NewString())
	if status != http.StatusOK {
		t.Fatalf("服用失败: %d, body: %v", status, envelope)
	}
	assertUUIDContract(t, envelope, fmt.Sprintf("POST /agents/%s/consume", agentUUID))

	status, envelope = doJSON(t, r, http.MethodGet,
		fmt.Sprintf("/api/v1/agents/%s/effects", agentUUID), "", "")
	if status != http.StatusOK {
		t.Fatalf("GET effects 期望 200, 实际 %d, body: %v", status, envelope)
	}
	data, _ := envelope["data"].(map[string]interface{})
	effects, ok := data["effects"].([]interface{})
	if !ok || len(effects) != 1 {
		t.Fatalf("能力快照应含 1 条: %v", envelope)
	}
	first, _ := effects[0].(map[string]interface{})
	if first["item_id"] != itemUUID {
		t.Fatalf("能力 item_id 应指向消耗实例 %s, 实际 %v", itemUUID, first["item_id"])
	}
	assertUUIDContract(t, envelope, fmt.Sprintf("GET /agents/%s/effects", agentUUID))
}

// TestUUIDContractDetectorSelfCheck 违约检测器自检: 构造含数字 id / uuid 键 /
// 嵌套违约 / 非法字符串 id 的假响应喂给检测器,断言逐一定位报错;守约响应零误报。
// 锁定检测器本身有效,防止断言函数退化为恒真。
func TestUUIDContractDetectorSelfCheck(t *testing.T) {
	// 数字内部主键泄漏: id=7
	violations := uuidContractViolations(map[string]interface{}{
		"code": float64(0), "data": map[string]interface{}{"id": float64(7)},
	}, "fake")
	if len(violations) != 1 || !strings.Contains(violations[0], "fake.data.id") {
		t.Fatalf("检测器未定位数字 id 违约: %v", violations)
	}

	// uuid 键违约(值即使是合法 UUID 也违约: 对外标识必须命名为 id)
	violations = uuidContractViolations(map[string]interface{}{
		"data": map[string]interface{}{"uuid": uuid.NewString()},
	}, "fake")
	if len(violations) != 1 || !strings.Contains(violations[0], "fake.data.uuid") {
		t.Fatalf("检测器未报告 uuid 键违约: %v", violations)
	}

	// 嵌套数组内 *_id 数字泄漏,路径须可定位
	violations = uuidContractViolations(map[string]interface{}{
		"data": map[string]interface{}{"items": []interface{}{
			map[string]interface{}{"id": uuid.NewString()},
			map[string]interface{}{"revision_id": float64(3)},
		}},
	}, "fake")
	if len(violations) != 1 || !strings.Contains(violations[0], "fake.data.items[1].revision_id") {
		t.Fatalf("检测器未定位嵌套违约路径: %v", violations)
	}

	// 非法字符串 id 同样违约
	violations = uuidContractViolations(map[string]interface{}{"effect_id": "not-a-uuid"}, "fake")
	if len(violations) != 1 || !strings.Contains(violations[0], "fake.effect_id") {
		t.Fatalf("检测器未报告非法字符串 id: %v", violations)
	}

	// 守约响应零误报(含 request_id、非 id 的数字/字符串字段、数组)
	clean := map[string]interface{}{
		"code": float64(0), "request_id": uuid.NewString(),
		"data": map[string]interface{}{"items": []interface{}{
			map[string]interface{}{"id": uuid.NewString(), "recipe_id": uuid.NewString(),
				"name": "x", "available_count": float64(2)},
		}},
	}
	if got := uuidContractViolations(clean, "ok"); len(got) != 0 {
		t.Fatalf("守约响应被误报: %v", got)
	}

	// memory_id 编排复合键 "<agentUUID>#<序号>" 豁免(非 HTTP 契约业务键);
	// 垃圾形态仍违约,豁免不得放行任意文本
	if got := uuidContractViolations(map[string]interface{}{"memory_id": uuid.NewString() + "#3"}, "ok"); len(got) != 0 {
		t.Fatalf("编排复合键被误报: %v", got)
	}
	if got := uuidContractViolations(map[string]interface{}{"memory_id": "junk#x"}, "ok"); len(got) != 1 {
		t.Fatalf("垃圾 memory_id 应违约: %v", got)
	}
}

// ---------- 递归断言器 ----------

// assertUUIDContract 递归断言 v 满足 UUID 契约,违约逐条 t.Errorf(路径可定位)
func assertUUIDContract(t *testing.T, v interface{}, path string) {
	t.Helper()
	for _, violation := range uuidContractViolations(v, path) {
		t.Errorf("UUID 契约违约: %s", violation)
	}
}

// uuidContractViolations 深度遍历 v(json.Unmarshal 产物: map/slice/标量),
// 收集 UUID 契约违约描述;空返回 = 守约。规则:
//   - 键名恰为 id 或以 _id 结尾: 值必须是 uuid.Parse 通过的字符串;
//     数字(float64,即内部 BIGSERIAL 主键)/null/其他类型一律违约
//   - 键名恰为 uuid: 违约(对外标识一律命名为 id)
//   - 例外: memory_id 允许编排内部复合键 "<agentUUID>#<序号>"
//     (internal/service/orchestration/types.go 的图状态键,非 HTTP 契约业务键),
//     但前缀仍须为合法 UUID 且序号为纯数字
func uuidContractViolations(v interface{}, path string) []string {
	switch node := v.(type) {
	case map[string]interface{}:
		var out []string
		for key, val := range node {
			child := path + "." + key
			if key == "uuid" {
				out = append(out, child+" 出现 uuid 键(对外标识必须命名为 id)")
				continue
			}
			if key == "id" || strings.HasSuffix(key, "_id") {
				if msg := contractIDValueMsg(key, val, child); msg != "" {
					out = append(out, msg)
				}
				continue
			}
			out = append(out, uuidContractViolations(val, child)...)
		}
		return out
	case []interface{}:
		var out []string
		for i, item := range node {
			out = append(out, uuidContractViolations(item, fmt.Sprintf("%s[%d]", path, i))...)
		}
		return out
	}
	return nil
}

// contractIDValueMsg 校验单个 id 类键值;空串 = 守约
func contractIDValueMsg(key string, val interface{}, path string) string {
	s, ok := val.(string)
	if !ok {
		return fmt.Sprintf("%s 须为 UUID 字符串, 实际 %T(%v)(内部主键泄漏)", path, val, val)
	}
	if _, err := uuid.Parse(s); err == nil {
		return ""
	}
	if key == "memory_id" && isOrchestrationMemoryKey(s) {
		return ""
	}
	return fmt.Sprintf("%s 不是合法 UUID: %q", path, s)
}

// isOrchestrationMemoryKey 判断 s 是否编排内部复合键 "<UUID>#<纯数字序号>"
func isOrchestrationMemoryKey(s string) bool {
	idx := strings.LastIndex(s, "#")
	if idx < 0 {
		return false
	}
	if _, err := uuid.Parse(s[:idx]); err != nil {
		return false
	}
	seq := s[idx+1:]
	if seq == "" {
		return false
	}
	for _, r := range seq {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
