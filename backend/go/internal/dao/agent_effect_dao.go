// 服用事务专用 DAO（任务 3）
// 约定与 pill_inventory_dao.go 一致：所有函数第一参数是事务句柄 tx，
// 绝不内部使用全局 DB —— 服用、能力写入、EffectsRevision、缓存失效必须同一事务。
package dao

import (
	stderrors "errors"
	"time"

	idao "github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AgentByUUID 事务内按对外 UUID 查道人
func AgentByUUID(tx *gorm.DB, uid uuid.UUID) (*model.DaoAgent, error) {
	var agent model.DaoAgent
	if err := tx.Where("uuid = ?", uid).First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

// ConsumePillItemCAS 消耗实例：available→consumed_by_agent 并写消耗去向；
// RowsAffected==1 才返回 true（竞争/重复/已消耗均 false）
func ConsumePillItemCAS(tx *gorm.DB, itemID uint, now time.Time, opID uint) (bool, error) {
	res := tx.Model(&model.PillItem{}).
		Where("id = ? AND state = ?", itemID, model.PillAvailable).
		Updates(map[string]any{
			"state":                model.PillConsumedByAgent,
			"consumed_at":          now,
			"consume_operation_id": opID,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// MaxEffectSortOrder 道人当前最大能力顺序（无记录返回 0）；agentUID 为道人 UUID 文本
func MaxEffectSortOrder(tx *gorm.DB, agentUID string) (int, error) {
	var maxOrder int
	if err := tx.Model(&model.AgentPillEffect{}).
		Where("agent_id = ? AND removed_at IS NULL", agentUID).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error; err != nil {
		return 0, err
	}
	return maxOrder, nil
}

// CountActiveEffectByAgentRevision 同版本活跃能力预检（§3.2 步骤 2；唯一索引仅兜底并发）
// agentUID 为道人 UUID 文本；revisionID 为丹方版本内部 id（任务 3 翻转）
func CountActiveEffectByAgentRevision(tx *gorm.DB, agentUID string, revisionID uint) (int64, error) {
	var n int64
	err := tx.Model(&model.AgentPillEffect{}).
		Where("agent_id = ? AND recipe_revision_id = ? AND removed_at IS NULL", agentUID, revisionID).
		Count(&n).Error
	return n, err
}

// CreateAgentPillEffect 写能力快照；唯一约束（同版本活跃能力）由调用方映射错误码
func CreateAgentPillEffect(tx *gorm.DB, ef *model.AgentPillEffect) error {
	return tx.Create(ef).Error
}

// IncrementEffectsRevision 单调递增能力编排版本（服用/移除/调权重顺序时同事务调用）
// agentUID 为道人 UUID 文本（011 业务键；行定位绑 dao_agents.uuid）
func IncrementEffectsRevision(tx *gorm.DB, agentUID string) error {
	return tx.Model(&model.DaoAgent{}).
		Where("uuid = ?", agentUID).
		Update("effects_revision", gorm.Expr("effects_revision + 1")).Error
}

// InvalidateLanguagePatternTx 同事务失效语言模式缓存（无缓存记录时 0 行，不视为错误）
// agentUID 为道人 UUID 文本
func InvalidateLanguagePatternTx(tx *gorm.DB, agentUID string) error {
	return tx.Model(&model.LanguagePattern{}).
		Where("agent_id = ?", agentUID).
		Update("is_valid", false).Error
}

// RemoveActiveEffectByItemUUID 事务内软删活跃能力（removed_at=now）：
// 实例 UUID 经 pill_items 子查询解析（跨 SQLite/Postgres/MySQL 兼容）。
// 无活跃能力返回 false；原实例保持 consumed_by_agent 不返还（§产品规则 移除不返还）。
func RemoveActiveEffectByItemUUID(tx *gorm.DB, agentUID string, itemUUID uuid.UUID, now time.Time) (bool, error) {
	res := tx.Model(&model.AgentPillEffect{}).
		Where("agent_id = ? AND removed_at IS NULL AND item_id IN (SELECT id FROM pill_items WHERE uuid = ?)",
			agentUID, itemUUID).
		Update("removed_at", now)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// UpdateActiveEffectByItemUUID 事务内更新活跃能力权重/顺序（实例 UUID 标识）：
// weight/sortOrder 均为 nil 时仅做存在性检查（无活跃能力返回 false）。
// 更新后由调用方同事务递增 EffectsRevision + 失效缓存。
func UpdateActiveEffectByItemUUID(tx *gorm.DB, agentUID string, itemUUID uuid.UUID, weight *float64, sortOrder *int) (bool, error) {
	updates := map[string]any{}
	if weight != nil {
		updates["weight"] = *weight
	}
	if sortOrder != nil {
		updates["sort_order"] = *sortOrder
	}
	query := tx.Model(&model.AgentPillEffect{}).
		Where("agent_id = ? AND removed_at IS NULL AND item_id IN (SELECT id FROM pill_items WHERE uuid = ?)",
			agentUID, itemUUID)
	if len(updates) == 0 {
		var n int64
		if err := query.Count(&n).Error; err != nil {
			return false, err
		}
		return n == 1, nil
	}
	res := query.Updates(updates)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// ActiveEffectsByAgent 事务内读道人活跃能力（按 sort_order,id 升序；任务 5 全量编排读取）
// agentUID 为道人 UUID 文本
func ActiveEffectsByAgent(tx *gorm.DB, agentUID string) ([]model.AgentPillEffect, error) {
	var effects []model.AgentPillEffect
	err := tx.Where("agent_id = ? AND removed_at IS NULL", agentUID).
		Order("sort_order ASC, id ASC").Find(&effects).Error
	return effects, err
}

// RemoveActiveEffectByUUID 事务内按能力 UUID 软删活跃能力（归属校验：agent_id 必须匹配）；
// 无活跃能力返回 false；原实例保持 consumed_by_agent 不返还（任务 5）
func RemoveActiveEffectByUUID(tx *gorm.DB, agentUID string, effectUUID uuid.UUID, now time.Time) (bool, error) {
	res := tx.Model(&model.AgentPillEffect{}).
		Where("agent_id = ? AND removed_at IS NULL AND uuid = ?", agentUID, effectUUID).
		Update("removed_at", now)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// UpdateActiveEffectsCASCAS 事务内全量编排提交（任务 5）：
// 乐观锁（effects_revision 必须等于 expected）→ 逐条更新 weight/sort_order →
// 递增 EffectsRevision → 失效语言模式缓存。乐观锁失败（0 行）返回 false 不写任何变更；
// 逐条更新 0 行（理论不可达，调用方已校验集合）视为内部错误回滚。
func UpdateActiveEffectsCASCAS(tx *gorm.DB, agentUID string, expectedEffectsRevision int, writes []idao.EffectWrite) (bool, error) {
	res := tx.Model(&model.DaoAgent{}).
		Where("uuid = ? AND effects_revision = ?", agentUID, expectedEffectsRevision).
		Update("effects_revision", gorm.Expr("effects_revision + 1"))
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected != 1 {
		return false, nil
	}
	for _, w := range writes {
		upd := tx.Model(&model.AgentPillEffect{}).
			Where("id = ? AND agent_id = ? AND removed_at IS NULL", w.EffectID, agentUID).
			Updates(map[string]any{"weight": w.Weight, "sort_order": w.SortOrder})
		if upd.Error != nil {
			return false, upd.Error
		}
		if upd.RowsAffected != 1 {
			return false, stderrors.New("dao.agent.update_effect_missing")
		}
	}
	if err := InvalidateLanguagePatternTx(tx, agentUID); err != nil {
		return false, err
	}
	return true, nil
}
