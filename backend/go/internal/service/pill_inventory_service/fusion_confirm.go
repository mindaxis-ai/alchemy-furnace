// 融合确认事务（§3.3 两阶段第二阶段）
// 校验顺序（幂等包装内）：查预览（404）→ 已确认（409 带已有操作 UUID）→ 过期（410）→
// 输入集合哈希核对 → 材料可用性核对 → 批量 CAS 全部材料 → 建新丹方/v1/一枚产物 →
// 深拷贝输出附加服务器 lineage → 单 SQL 条件绑定确认操作（并发双确认兜底）。
// 任何一步失败整体回滚：材料不部分消耗、预览不绑定、无产物。
// 确认只能改名称/描述（§3.3）；其他输出字段以预览持久化的模型结果为准。
package pill_inventory_service

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// confirmFusionHash ConfirmFusion 的标准化负载哈希（预览 ID + 可改字段参与）
func confirmFusionHash(req service.ConfirmFusionRequest) string {
	return payloadHash("confirm_fusion",
		req.PreviewID.String(), req.Name, req.Description)
}

// ConfirmFusion 原子确认融合：消耗全部材料并产出新丹方 v1 + 一枚新金丹；幂等
func (s *Inventory) ConfirmFusion(ctx context.Context, req service.ConfirmFusionRequest) (*service.PillOperationResult, errors.Error) {
	if req.PreviewID == uuid.Nil {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "pill.invalid_request", "缺少预览标识")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "recipe.invalid_schema", "融合丹方名称不能为空")
	}
	return s.runOperation(ctx, req.OperationID, "confirm_fusion", confirmFusionHash(req),
		func(tx *gorm.DB, op *model.PillOperation) (*service.PillOperationResult, error) {
			// 1) 预览存在（404）
			preview, err := dao.FusionPreviewByUUID(tx, req.PreviewID)
			if stderrors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.ErrorRecordNotFound("fusion.preview_not_found")
			}
			if err != nil {
				return nil, err
			}
			// 2) 已确认：不同 key 二次确认 409，消息携带已有操作 UUID（前端可据以恢复）
			if preview.ConfirmedOperationID != nil {
				existing, err := dao.PillOperationByID(tx, *preview.ConfirmedOperationID)
				if err != nil {
					return nil, err
				}
				return nil, errors.New(errors.ErrorTypeConflict, "fusion.preview_already_confirmed",
					"该预览已被操作(%s)确认，请刷新后重试", existing.UUID.String())
			}
			// 3) 过期（410）：预览只保证 15 分钟内材料未动
			if s.now().After(preview.ExpiresAt) {
				return nil, errors.New(errors.ErrorTypeGone, "fusion.preview_expired",
					"融合预览已过期，请重新生成")
			}
			// 4) 输入集合哈希核对：预览行被外部改动时拒绝（正常路径恒等）
			inputIDs, err := parsePreviewInputs(preview.InputItemsJSON)
			if err != nil {
				return nil, err
			}
			if FusionInputHash(inputIDs) != preview.InputHash {
				return nil, errors.New(errors.ErrorTypeConflict, "pill.not_available",
					"融合材料集合与预览不一致，请重新生成预览")
			}
			// 5) 材料可用性核对（CAS 前预检；任一致命即整体拒绝，不部分消耗）
			items, err := dao.ListPillItemsByUUIDs(tx, inputIDs)
			if err != nil {
				return nil, err
			}
			if len(items) != len(inputIDs) {
				return nil, errors.New(errors.ErrorTypeConflict, "pill.not_available",
					"融合材料已变化，请重新生成预览")
			}
			itemIDs := make([]uint, 0, len(items))
			parentNames := make([]string, 0, len(items))
			parentRevisions := make([]string, 0, len(items))
			for _, item := range items {
				if item.State != model.PillAvailable {
					return nil, errors.New(errors.ErrorTypeConflict, "pill.not_available",
						"金丹不可用（已被服用/融合/弃置）")
				}
				itemIDs = append(itemIDs, item.ID)
				// lineage 需要父版本名称/版本 UUID 快照（不可变版本，引用安全）
				rev, err := dao.PillRecipeRevisionByID(tx, item.RecipeRevisionID)
				if err != nil {
					return nil, err
				}
				parentNames = append(parentNames, rev.Name)
				parentRevisions = append(parentRevisions, rev.UUID.String())
			}
			// 6) 批量 CAS 全部材料：available→consumed_by_fusion。
			//    条件更新原子性：任一材料已被并发消耗 → 0 行 → 整体回滚
			ok, err := dao.ConsumeFusionItemsCAS(tx, itemIDs, s.now(), op.UUID.String())
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errors.New(errors.ErrorTypeConflict, "pill.not_available",
					"金丹不可用（并发冲突），请重新生成预览")
			}
			// 7) 建新丹方 + 不可变 v1 + 一枚产物（schema 深拷贝自预览输出，不共享引用）
			recipe := &model.PillRecipe{CreatedAt: s.now()}
			if err := dao.CreatePillRecipe(tx, recipe); err != nil {
				return nil, err
			}
			output := deepCopySchema(preview.OutputJSON)
			schema, ok := output["skill_schema"].(map[string]any)
			if !ok {
				return nil, errors.ErrorServerInternalError("fusion.preview_corrupt")
			}
			rev := &model.PillRecipeRevision{
				RecipeID:     recipe.UUID.String(),
				Revision:     1,
				Name:         req.Name,
				Description:  req.Description,
				SkillSchema:  model.JSONMap(schema),
				Tags:         model.JSONList{},
				VersionLabel: "1.0.0",
				CreatedAt:    s.now(),
			}
			if err := dao.CreatePillRecipeRevision(tx, rev); err != nil {
				return nil, err
			}
			if err := dao.SetPillRecipeCurrentRevision(tx, recipe.ID, rev.UUID.String()); err != nil {
				return nil, err
			}
			item := &model.PillItem{
				RecipeRevisionID:  rev.UUID.String(),
				State:             model.PillAvailable,
				OriginOperationID: op.UUID.String(),
				OriginIndex:       0,
				CreatedAt:         s.now(),
			}
			if err := dao.CreatePillItem(tx, item); err != nil {
				return nil, err
			}
			// 8) 服务器 lineage 附加到输出深拷贝（§3.3 落点）：
			//    父实例 UUID / 父版本 UUID / 名称快照 / 操作 UUID / 操作者快照
			output["lineage"] = map[string]any{
				"operation_id":     op.UUID.String(),
				"parent_items":     uuidStrings(inputIDs),
				"parent_names":     parentNames,
				"parent_revisions": parentRevisions,
				"operator":         preview.OperatorSnapshot,
				"confirmed_at":     s.now().Format(time.RFC3339),
			}
			// 9) 单 SQL「写 lineage + 条件绑定确认操作」：
			//    RowsAffected==0 表示并发双确认已抢先 → 409，事务整体回滚（材料归还、产物撤销）
			bound, err := dao.ConfirmFusionPreviewCAS(tx, preview.ID, op.UUID.String(), output)
			if err != nil {
				return nil, err
			}
			if !bound {
				return nil, errors.New(errors.ErrorTypeConflict, "fusion.preview_already_confirmed",
					"该预览已被其他请求确认，请刷新后重试")
			}
			return &service.PillOperationResult{
				OperationID:     op.UUID,
				RecipeID:        &recipe.UUID,
				RevisionID:      &rev.UUID,
				ItemIDs:         []uuid.UUID{item.UUID},
				ConsumedItemIDs: inputIDs,
			}, nil
		})
}

// parsePreviewInputs 预览输入列表（JSONList of string）→ 实例 UUID 列表；
// 格式损坏视为数据异常（500），不静默丢材料
func parsePreviewInputs(list model.JSONList) ([]uuid.UUID, errors.Error) {
	out := make([]uuid.UUID, 0, len(list))
	for i, v := range list {
		s, ok := v.(string)
		if !ok {
			return nil, errors.ErrorServerInternalError("fusion.preview_corrupt")
		}
		uid, err := uuid.Parse(s)
		if err != nil {
			return nil, errors.New(errors.ErrorTypeServerInternalError, "fusion.preview_corrupt",
				"预览输入列表损坏（第 %d 项）", i+1)
		}
		out = append(out, uid)
	}
	return out, nil
}
