// Package fusion_service 金丹融合业务逻辑（两阶段，任务 4）
// 第一阶段 PreviewFusion：读事务加载材料 → 结束事务 → 模型生成（LLM 不在事务内）→
// schema 校验 → 持久化 FusionPreview（15 分钟有效期）。不扣料不占料。
// 第二阶段 ConfirmFusion 在 pill_inventory_service（本包只负责生成预览）。
// 对应 RESTful API：/api/v1/fusion/*（任务 5 接入）
package fusion_service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/pill_inventory_service"
	"github.com/alchemy-furnace/server/internal/service/pill_service"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Fusion service.Fusion 接口实现
type Fusion struct {
	db         *gorm.DB
	fusion     synthesis.FusionClient // 接口,便于单测 mock
	credential credential.Resolver
	now        func() time.Time
}

// New 构造金丹融合业务实例（任务 4：直连库存表，不再依赖旧 Pill DAO）
func New(db *gorm.DB, fusionClient synthesis.FusionClient, credential credential.Resolver) *Fusion {
	return NewWithClock(db, fusionClient, credential, time.Now)
}

// NewWithClock 注入时钟构造（测试固定 now；生产走 New 的真实时钟）
func NewWithClock(db *gorm.DB, fusionClient synthesis.FusionClient, credential credential.Resolver, now func() time.Time) *Fusion {
	return &Fusion{db: db, fusion: fusionClient, credential: credential, now: now}
}

// previewTTL 预览有效期（§3.3：15 分钟）
const previewTTL = 15 * time.Minute

// loadFusionInputs 读事务加载材料并核对可用性；返回按请求顺序的引擎输入与实例业务主键。
// 任一材料缺失 404 / 非可用 409；读取与模型调用分离——LLM 请求绝不持有事务。
func (s *Fusion) loadFusionInputs(ctx context.Context, itemUUIDs []uuid.UUID) ([]synthesis.PillInput, []uuid.UUID, errors.Error) {
	itemsByID := make(map[string]model.PillItem, len(itemUUIDs))
	revsByID := make(map[string]model.PillRecipeRevision, len(itemUUIDs))
	terr := s.db.Transaction(func(tx *gorm.DB) error {
		items, err := dao.ListPillItemsByUUIDs(tx, itemUUIDs)
		if err != nil {
			return err
		}
		for i := range items {
			it := items[i]
			// 预览不占料：材料在预览期间可能被服用/融合/弃置，任一非可用即拒绝
			if it.State != model.PillAvailable {
				return errors.New(errors.ErrorTypeConflict, "pill.not_available",
					"金丹不可用（已被服用/融合/弃置）")
			}
			itemsByID[it.PillItemID] = it
			rev, err := dao.PillRecipeRevisionByID(tx, it.RecipeRevisionID)
			if err != nil {
				return err
			}
			revsByID[it.RecipeRevisionID] = *rev
		}
		return nil
	})
	if terr != nil {
		if ee, ok := terr.(errors.Error); ok {
			return nil, nil, ee
		}
		return nil, nil, errors.ErrorServerInternalError("service.fusion.load_items")
	}
	// 按请求 UUID 顺序组装输入；任一缺失 404（与旧 trial/fusion 语义对齐）。
	// 全部命中后 ids 与请求序恒等,直接复用请求切片(业务主键即 uuid 文本)
	inputs := make([]synthesis.PillInput, 0, len(itemUUIDs))
	ids := make([]uuid.UUID, 0, len(itemUUIDs))
	for _, uid := range itemUUIDs {
		it, ok := itemsByID[uid.String()]
		if !ok {
			return nil, nil, errors.New(errors.ErrorTypeRecordNotFound, "service.fusion.pill_missing",
				"金丹(id=%s)不存在", uid.String())
		}
		rev := revsByID[it.RecipeRevisionID]
		inputs = append(inputs, synthesis.PillInput{
			ID:          it.PillItemID,
			Name:        rev.Name,
			SkillSchema: rev.SkillSchema,
		})
		ids = append(ids, uid)
	}
	return inputs, ids, nil
}

// callFusion 解析融合专用模型凭证并调用引擎。
// 凭证解析失败返回 400 硬性错误，引导去设置中配置融合专用模型（不静默回退道人默认）
func (s *Fusion) callFusion(ctx context.Context, inputs []synthesis.PillInput, excludeOperatorID string) (*synthesis.FuseResponse, errors.Error) {
	var creds *credential.ModelCredentials
	if s.credential != nil {
		c, e := s.credential.ResolveFusionCredentials(ctx)
		if e != nil {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.fusion.resolve_fusion", e.Error())
		}
		creds = c
	}
	resp, e := s.fusion.Fuse(ctx, inputs, excludeOperatorID, creds)
	if e != nil {
		return nil, errors.New(errors.ErrorTypeServerInternalError, "service.fusion.fuse", e.Error())
	}
	return resp, nil
}

// PreviewFusion 融合预览（§3.3）：校验 → 读事务加载 → 模型生成（事务外）→
// schema 校验 → 持久化预览。失败路径（模型失败/输出不合法）不产生任何库存或预览变更。
func (s *Fusion) PreviewFusion(ctx context.Context, req service.PreviewFusionRequest) (*service.FusionPreviewResult, errors.Error) {
	if len(req.ItemIDs) < 2 {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.fusion.too_few", "融合至少需要 2 枚金丹")
	}
	// 去重校验：同一枚实例不能同时作为两份材料
	seen := make(map[uuid.UUID]struct{}, len(req.ItemIDs))
	for _, uid := range req.ItemIDs {
		if _, dup := seen[uid]; dup {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.fusion.duplicate_items", "融合材料不能重复")
		}
		seen[uid] = struct{}{}
	}

	inputs, itemIDs, err := s.loadFusionInputs(ctx, req.ItemIDs)
	if err != nil {
		return nil, err
	}
	resp, err := s.callFusion(ctx, inputs, req.ExcludeOperatorID)
	if err != nil {
		return nil, err
	}
	// 输出 schema 校验：不给可确认的坏预览（错误码对齐丹方域）
	if err := pill_service.ValidateSkillSchema(resp.SkillSchema); err != nil {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "recipe.invalid_schema", err.Error())
	}

	now := s.now()
	preview := &model.FusionPreview{
		InputItemsJSON: makeInputList(itemIDs),
		InputHash:      pill_inventory_service.FusionInputHash(itemIDs),
		OutputJSON:     outputToJSON(resp),
		OperatorSnapshot: model.JSONMap{
			"id":   resp.Operator.ID,
			"name": resp.Operator.Name,
		},
		Base:      model.Base{CreatedAt: now},
		ExpiresAt: now.Add(previewTTL),
	}
	if req.ExcludeOperatorID != "" {
		preview.OperatorSnapshot["exclude_operator_id"] = req.ExcludeOperatorID
	}
	if err := s.db.Create(preview).Error; err != nil {
		return nil, errors.ErrorServerInternalError("fusion.preview_save_failed")
	}
	return &service.FusionPreviewResult{
		PreviewID:   preview.FusionPreviewID,
		ExpiresAt:   preview.ExpiresAt,
		Name:        resp.Name,
		Description: resp.Description,
		SkillSchema: resp.SkillSchema,
		Operator:    resp.Operator,
		Model:       resp.Model,
		Degraded:    resp.Degraded,
	}, nil
}

// Fuse 旧融合预览入口：按新库存加载可用实例并调用模型，不落库。
// 任务 5 将把 HTTP 层切换为 PreviewFusion；此方法保留服务层语义不回归。
func (s *Fusion) Fuse(ctx context.Context, pillUUIDs []uuid.UUID, excludeOperatorID string) (*synthesis.FuseResponse, errors.Error) {
	if len(pillUUIDs) < 2 {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.fusion.too_few", "融合至少需要 2 枚金丹")
	}
	inputs, _, err := s.loadFusionInputs(ctx, pillUUIDs)
	if err != nil {
		return nil, err
	}
	return s.callFusion(ctx, inputs, excludeOperatorID)
}

// ---------- 小工具 ----------

// makeInputList 实例 UUID 列表 → 预览输入 JSON 列表（保持请求顺序）
func makeInputList(ids []uuid.UUID) model.JSONList {
	out := make(model.JSONList, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

// outputToJSON 模型响应 → 持久化 JSONMap（序列化往返即深拷贝，不共享引用）
func outputToJSON(resp *synthesis.FuseResponse) model.JSONMap {
	raw, err := json.Marshal(resp)
	if err != nil {
		return model.JSONMap{}
	}
	var out model.JSONMap
	if err := json.Unmarshal(raw, &out); err != nil {
		return model.JSONMap{}
	}
	return out
}
