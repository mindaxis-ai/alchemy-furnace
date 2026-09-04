// 服用事务（§3.2）
// 步骤：验证 agent/实例/版本（归档不阻止已有实例）→ 同版本活跃能力预检 →
// CAS 消耗实例 + 写能力快照（深拷贝）→ 权重/排序 → EffectsRevision++ + 同事务失效缓存 →
// 保存 effect_id/consumed_item_ids/operation_id。任何一步失败整体回滚。
// 服用成功以本地能力快照提交为准；模型不可用不代表丹药未服用（不后台再次扣料）。
package pill_inventory_service

import (
	"context"
	stderrors "errors"
	"strconv"
	"strings"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// consumeHash Consume 的标准化负载哈希：
// weight 规范化后参与（同 key 两次请求 weight<=0 均按 1.0 计算）；
// sortOrder 用请求原值参与（<=0 的最终默认值在事务内决定，同 key 重试 hash 保持一致）
func consumeHash(req service.ConsumePillRequest, weight float64) string {
	return payloadHash("consume",
		req.AgentID.String(), req.ItemID.String(),
		strconv.FormatFloat(weight, 'g', -1, 64),
		strconv.Itoa(req.SortOrder))
}

// Consume 服用一枚金丹：消耗库存并生成能力快照；幂等
func (s *Inventory) Consume(ctx context.Context, req service.ConsumePillRequest) (*service.PillOperationResult, errors.Error) {
	if req.AgentID == uuid.Nil || req.ItemID == uuid.Nil {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "pill.invalid_request", "缺少道人或实例标识")
	}
	weight := req.Weight
	if weight <= 0 {
		weight = 1.0 // 与旧 BindPill 行为一致；权重不是剂量，0.5 也消耗完整一枚
	}
	return s.runOperation(ctx, req.OperationID, "consume", consumeHash(req, weight),
		func(tx *gorm.DB, op *model.PillOperation) (*service.PillOperationResult, error) {
			// 1) 验证道人存在
			agent, err := dao.AgentByUUID(tx, req.AgentID)
			if stderrors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.ErrorRecordNotFound("agent.not_found")
			}
			if err != nil {
				return nil, err
			}
			// 2) 实例可用 + 版本可读（归档丹方不阻止已有实例，§3.2 步骤 1）
			item, err := dao.PillItemByUUID(tx, req.ItemID)
			if stderrors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.ErrorRecordNotFound("pill.not_found")
			}
			if err != nil {
				return nil, err
			}
			rev, err := dao.PillRecipeRevisionByID(tx, item.RecipeRevisionID)
			if err != nil {
				return nil, err
			}
			// 3) CAS 消耗：available→consumed_by_agent；竞争/重复/已消耗 0 行 → 409。
			//    先于活跃能力预检：同实例二次服用报「实例不可用」，不误导为能力重复
			ok, err := dao.ConsumePillItemCAS(tx, item.ID, s.now(), op.ID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errors.New(errors.ErrorTypeConflict, "pill.not_available",
					"金丹不可服用（已被服用/融合/弃置）")
			}
			// 4) 同版本活跃能力预检（唯一索引兜底并发）；失败时 CAS 随事务一起回滚
			n, err := dao.CountActiveEffectByAgentRevision(tx, agent.UUID.String(), rev.ID)
			if err != nil {
				return nil, err
			}
			if n > 0 {
				return nil, errors.New(errors.ErrorTypeConflict, "pill.effect_already_active",
					"该道人已吸收同版本丹方的能力，请调整已吸收能力")
			}
			// 5) 能力快照：名称 + 完整 schema 深拷贝（不保存指向请求方可变对象的引用）
			sortOrder := req.SortOrder
			if sortOrder <= 0 {
				maxOrder, err := dao.MaxEffectSortOrder(tx, agent.UUID.String())
				if err != nil {
					return nil, err
				}
				sortOrder = maxOrder + 1
			}
			ef := &model.AgentPillEffect{
				AgentID:          agent.UUID.String(),
				ItemID:           item.ID,
				RecipeRevisionID: rev.ID,
				NameSnapshot:     rev.Name,
				SchemaSnapshot:   deepCopySchema(rev.SkillSchema),
				Weight:           weight,
				SortOrder:        sortOrder,
				CreatedAt:        s.now(),
			}
			if err := dao.CreateAgentPillEffect(tx, ef); err != nil {
				if isActiveEffectUniqueViolation(err) {
					return nil, errors.New(errors.ErrorTypeConflict, "pill.effect_already_active",
						"该道人已吸收同版本丹方的能力（并发冲突）")
				}
				return nil, err
			}
			// 6) 编排版本递增 + 同事务失效缓存
			if err := dao.IncrementEffectsRevision(tx, agent.UUID.String()); err != nil {
				return nil, err
			}
			if err := dao.InvalidateLanguagePatternTx(tx, agent.UUID.String()); err != nil {
				return nil, err
			}
			return &service.PillOperationResult{
				OperationID:     op.UUID,
				EffectID:        &ef.UUID,
				ConsumedItemIDs: []uuid.UUID{item.UUID},
			}, nil
		})
}

// isActiveEffectUniqueViolation 同版本活跃能力唯一索引冲突检测
// （SQLite 消息形如 "UNIQUE constraint failed: agent_pill_effects.agent_id, agent_pill_effects.recipe_revision_id"）
func isActiveEffectUniqueViolation(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "agent_pill_effects") &&
		(strings.Contains(msg, "recipe_revision_id") || strings.Contains(msg, "agent_id"))
}
