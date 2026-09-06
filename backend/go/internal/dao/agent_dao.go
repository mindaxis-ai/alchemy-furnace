// Package dao 道人数据访问实现(新架构 internal 分层;业务主键统一 uuid 文本,对外标识解析在此完成)
package dao

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	idao "github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AgentDao dao.Agent 接口实现
type AgentDao struct{}

// NewAgentDao 构造道人 DAO
func NewAgentDao() *AgentDao {
	return &AgentDao{}
}

// TakeAgentByUUID 按对外 UUID 查询道人
func (d *AgentDao) TakeAgentByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error) {
	var agent model.DaoAgent
	if err := GetDB().WithContext(ctx).Where("dao_agent_id = ?", uid.String()).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.agent.take_by_uuid")
		}
		return nil, errors.ErrorServerInternalError("dao.agent.take_by_uuid")
	}
	return &agent, nil
}

// TakeAgentDetailByUUID 按 UUID 查询道人详情(预加载已吸收能力快照+语言模式缓存)
// 任务 8 旧入口审计: 不再预加载遗留 AgentPills(旧表仅保留供回滚),能力数据以
// AgentPillEffects(removed_at IS NULL)为事实来源
func (d *AgentDao) TakeAgentDetailByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error) {
	var agent model.DaoAgent
	if err := GetDB().WithContext(ctx).
		Preload("AgentPillEffects", func(db *gorm.DB) *gorm.DB {
			return db.Where("removed_at IS NULL").Order("sort_order ASC, agent_pill_effect_id ASC")
		}).
		Preload("AgentPillEffects.Item").
		Preload("LanguagePattern").
		Where("dao_agent_id = ?", uid.String()).
		First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.agent.take_detail_by_uuid")
		}
		return nil, errors.ErrorServerInternalError("dao.agent.take_detail_by_uuid")
	}
	return &agent, nil
}

// FindAgents 分页查询道人列表
func (d *AgentDao) FindAgents(ctx context.Context, page int, size int, status string) (int64, []*model.DaoAgent, errors.Error) {
	db := GetDB().WithContext(ctx).Model(&model.DaoAgent{})
	if status != "" {
		db = db.Where("status = ?", status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.agent.find_agents_count")
	}
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}

	var agents []*model.DaoAgent
	if err := db.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&agents).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.agent.find_agents")
	}
	return total, agents, nil
}

// SaveAgent 新建道人
func (d *AgentDao) SaveAgent(ctx context.Context, agent *model.DaoAgent) errors.Error {
	if err := GetDB().WithContext(ctx).Create(agent).Error; err != nil {
		return errors.ErrorServerInternalError("dao.agent.save_agent")
	}
	return nil
}

// UpdateAgent 部分更新道人字段
func (d *AgentDao) UpdateAgent(ctx context.Context, agent *model.DaoAgent, updates map[string]any) errors.Error {
	if err := GetDB().WithContext(ctx).Model(agent).Updates(updates).Error; err != nil {
		return errors.ErrorServerInternalError("dao.agent.update_agent")
	}
	return nil
}

// DeleteAgent 删除道人
func (d *AgentDao) DeleteAgent(ctx context.Context, agent *model.DaoAgent) errors.Error {
	if err := GetDB().WithContext(ctx).Delete(agent).Error; err != nil {
		return errors.ErrorServerInternalError("dao.agent.delete_agent")
	}
	return nil
}

// CountSessionsByAgentID 统计道人参与的去重会话数(单聊 + 群聊成员)
func (d *AgentDao) CountSessionsByAgentID(ctx context.Context, agentID string) (int64, errors.Error) {
	// 单聊: chat_sessions.agent_id 直挂
	var singleCount int64
	if err := GetDB().WithContext(ctx).Model(&model.ChatSession{}).
		Where("agent_id = ?", agentID).
		Count(&singleCount).Error; err != nil {
		return 0, errors.ErrorServerInternalError("dao.agent.count_sessions_single")
	}
	// 群聊: session_members 按 session 去重(一个会话只算一次)
	var groupCount int64
	if err := GetDB().WithContext(ctx).Model(&model.SessionMember{}).
		Where("agent_id = ?", agentID).
		Distinct("session_id").
		Count(&groupCount).Error; err != nil {
		return 0, errors.ErrorServerInternalError("dao.agent.count_sessions_group")
	}
	return singleCount + groupCount, nil
}

// SaveAgentPill 新建服用记录
func (d *AgentDao) SaveAgentPill(ctx context.Context, agentPill *model.AgentPill) errors.Error {
	if err := GetDB().WithContext(ctx).Create(agentPill).Error; err != nil {
		return errors.ErrorServerInternalError("dao.agent.save_agent_pill")
	}
	return nil
}

// UpdateAgentPill 部分更新服用记录
func (d *AgentDao) UpdateAgentPill(ctx context.Context, agentPill *model.AgentPill, updates map[string]any) errors.Error {
	if err := GetDB().WithContext(ctx).Model(agentPill).Updates(updates).Error; err != nil {
		return errors.ErrorServerInternalError("dao.agent.update_agent_pill")
	}
	return nil
}

// FindPillsByAgentID 道人已服用金丹列表(按服用顺序)
// agentUID 为道人 UUID 文本(011 业务键): agent_pills.agent_id/pill_id 均为 UUID 文本列,
// JOIN 按金丹业务键 elixir_pills.elixir_pill_id 对齐
func (d *AgentDao) FindPillsByAgentID(ctx context.Context, agentUID string) ([]*model.ElixirPill, errors.Error) {
	var pills []*model.ElixirPill
	if err := GetDB().WithContext(ctx).Table("elixir_pills").
		Select("elixir_pills.*").
		Joins("JOIN agent_pills ON agent_pills.pill_id = elixir_pills.elixir_pill_id").
		Where("agent_pills.agent_id = ?", agentUID).
		Order("agent_pills.sort_order ASC, agent_pills.agent_pill_id ASC").
		Find(&pills).Error; err != nil {
		return nil, errors.ErrorServerInternalError("dao.agent.find_pills_by_agent")
	}
	return pills, nil
}

// ReplaceAgentPills 原子替换道人的完整服丹编排
// 单事务内: 删除全部旧关系 → 按请求顺序写新关系(sort_order=1..n) → 失效语言模式缓存
// 任一步失败由 GORM Transaction 回滚,旧关系与缓存状态保持不变
func (d *AgentDao) ReplaceAgentPills(ctx context.Context, agentUID string, pills []idao.AgentPillInput) errors.Error {
	txErr := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 删除旧关系
		if err := tx.Where("agent_id = ?", agentUID).Delete(&model.AgentPill{}).Error; err != nil {
			return err
		}
		// 2) 校验 + 按请求顺序批量写新关系(sort_order 从 1 开始)
		if len(pills) > 0 {
			// 数据完整性: 重复 pill 拒绝、缺失 pill 回滚(不依赖各驱动 FK 配置差异)
			ids := make([]string, 0, len(pills))
			seen := make(map[string]struct{}, len(pills))
			for _, p := range pills {
				if _, dup := seen[p.PillID]; dup {
					return fmt.Errorf("duplicate pill uuid %s", p.PillID)
				}
				seen[p.PillID] = struct{}{}
				ids = append(ids, p.PillID)
			}
			var existCount int64
			if err := tx.Model(&model.ElixirPill{}).Where("elixir_pill_id IN ?", ids).Count(&existCount).Error; err != nil {
				return err
			}
			if existCount != int64(len(ids)) {
				return fmt.Errorf("pill not found")
			}

			rows := make([]model.AgentPill, 0, len(pills))
			for i, p := range pills {
				rows = append(rows, model.AgentPill{
					AgentID:   agentUID,
					PillID:    p.PillID,
					Weight:    p.Weight,
					SortOrder: i + 1,
				})
			}
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		// 3) 同事务失效语言模式缓存(无记录时影响 0 行,不视为错误)
		if err := tx.Model(&model.LanguagePattern{}).
			Where("agent_id = ?", agentUID).
			Update("is_valid", false).Error; err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		return errors.ErrorServerInternalError("dao.agent.replace_pills")
	}
	return nil
}

// RemoveAgentPillEffect 移除道人的已吸收能力(软删保留历史,任务 3)
// 单事务: 软删活跃能力 → 递增 EffectsRevision → 失效语言模式缓存,任一步失败整体回滚;
// 无活跃能力返回 ErrorTypeRecordNotFound;原实例保持 consumed_by_agent 不返还
func (d *AgentDao) RemoveAgentPillEffect(ctx context.Context, agentUID string, itemUUID uuid.UUID, now time.Time) errors.Error {
	txErr := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ok, err := RemoveActiveEffectByItemUUID(tx, agentUID, itemUUID, now)
		if err != nil {
			return err
		}
		if !ok {
			return errors.ErrorRecordNotFound("dao.agent.remove_effect")
		}
		if err := IncrementEffectsRevision(tx, agentUID); err != nil {
			return err
		}
		return InvalidateLanguagePatternTx(tx, agentUID)
	})
	if txErr == nil {
		return nil
	}
	if appErr, ok := txErr.(errors.Error); ok {
		return appErr
	}
	return errors.ErrorServerInternalError("dao.agent.remove_effect")
}

// UpdateAgentPillEffect 更新活跃能力权重/顺序(实例 UUID 标识,任务 3)
// 单事务: 更新(weight/sortOrder 均为 nil 时仅校验存在) → 递增 EffectsRevision → 失效缓存;
// 无活跃能力返回 ErrorTypeRecordNotFound
func (d *AgentDao) UpdateAgentPillEffect(ctx context.Context, agentUID string, itemUUID uuid.UUID, weight *float64, sortOrder *int) errors.Error {
	txErr := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ok, err := UpdateActiveEffectByItemUUID(tx, agentUID, itemUUID, weight, sortOrder)
		if err != nil {
			return err
		}
		if !ok {
			return errors.ErrorRecordNotFound("dao.agent.update_effect")
		}
		if weight != nil || sortOrder != nil {
			if err := IncrementEffectsRevision(tx, agentUID); err != nil {
				return err
			}
			return InvalidateLanguagePatternTx(tx, agentUID)
		}
		return nil
	})
	if txErr == nil {
		return nil
	}
	if appErr, ok := txErr.(errors.Error); ok {
		return appErr
	}
	return errors.ErrorServerInternalError("dao.agent.update_effect")
}

// InvalidateLanguagePattern 失效道人语言模式缓存
func (d *AgentDao) InvalidateLanguagePattern(ctx context.Context, agentUID string) errors.Error {
	if err := GetDB().WithContext(ctx).Model(&model.LanguagePattern{}).
		Where("agent_id = ?", agentUID).
		Update("is_valid", false).Error; err != nil {
		return errors.ErrorServerInternalError("dao.agent.invalidate_pattern")
	}
	return nil
}

// SaveLanguagePattern 写入/更新语言模式缓存(GORM Save: 业务主键为空创建,否则全字段更新)
func (d *AgentDao) SaveLanguagePattern(ctx context.Context, pattern *model.LanguagePattern) errors.Error {
	if err := GetDB().WithContext(ctx).Save(pattern).Error; err != nil {
		return errors.ErrorServerInternalError("dao.agent.save_language_pattern")
	}
	return nil
}

// SaveLanguagePatternIfRevision 带能力编排版本 CAS 核对的缓存写入(缓存保护 §3.2):
// 同一事务内先读 dao_agents.effects_revision,与读取时记录值不一致(并发服用/移除已提交)
// 返回 409 agent.effects_conflict,由调用方丢弃本次编译结果并重读重试。
// 版本一致则原样保存,写入与核对同事务(核对后不会有并发窗口)。
func (d *AgentDao) SaveLanguagePatternIfRevision(ctx context.Context, pattern *model.LanguagePattern, expectedEffectsRevision int) errors.Error {
	txErr := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cur int
		if err := tx.Model(&model.DaoAgent{}).
			Where("dao_agent_id = ?", pattern.AgentID).
			Select("effects_revision").
			Scan(&cur).Error; err != nil {
			return err
		}
		if cur != expectedEffectsRevision {
			return errors.ErrorConflict("agent.effects_conflict",
				"能力编排已变更,语言模式缓存写入被拒绝,请重试")
		}
		return tx.Save(pattern).Error
	})
	if txErr == nil {
		return nil
	}
	if appErr, ok := txErr.(errors.Error); ok {
		return appErr
	}
	return errors.ErrorServerInternalError("dao.agent.save_language_pattern_if_revision")
}

// ListActiveEffects 道人活跃能力列表(任务 5,effect UUID 语义):
// 单事务内读活跃能力 + 批量加载来源实例/版本,组装对外 UUID 标识;
// 来源实例或版本缺失(理论不可达:能力由服用生成且版本不可删)视为内部错误
func (d *AgentDao) ListActiveEffects(ctx context.Context, agentUID string) ([]idao.EffectWithSource, errors.Error) {
	var out []idao.EffectWithSource
	txErr := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		effects, err := ActiveEffectsByAgent(tx, agentUID)
		if err != nil {
			return err
		}
		itemIDs := make([]string, 0, len(effects))
		revIDs := make([]string, 0, len(effects))
		for _, ef := range effects {
			itemIDs = append(itemIDs, ef.ItemID)
			revIDs = append(revIDs, ef.RecipeRevisionID)
		}
		items, err := PillItemsByIDs(tx, itemIDs)
		if err != nil {
			return err
		}
		revs, err := PillRecipeRevisionsByIDs(tx, revIDs)
		if err != nil {
			return err
		}
		out = make([]idao.EffectWithSource, 0, len(effects))
		for _, ef := range effects {
			it, okItem := items[ef.ItemID]
			rv, okRev := revs[ef.RecipeRevisionID]
			if !okItem || !okRev {
				return stderrors.New("dao.agent.effect_source_missing")
			}
			// 业务主键已统一为 uuid 文本:对外标识在此解析为 uuid.UUID(理论不可达的脏文本视为内部错误)
			itemUID, itemParseErr := uuid.Parse(it.PillItemID)
			if itemParseErr != nil {
				return stderrors.New("dao.agent.item_uuid_invalid")
			}
			revUID, revParseErr := uuid.Parse(rv.PillRecipeRevisionID)
			if revParseErr != nil {
				return stderrors.New("dao.agent.revision_uuid_invalid")
			}
			out = append(out, idao.EffectWithSource{
				Effect:       ef,
				ItemUUID:     itemUID,
				RevisionUUID: revUID,
			})
		}
		return nil
	})
	if txErr != nil {
		if appErr, ok := txErr.(errors.Error); ok {
			return nil, appErr
		}
		return nil, errors.ErrorServerInternalError("dao.agent.list_active_effects")
	}
	return out, nil
}

// RemoveAgentPillEffectByUUID 移除道人的已吸收能力(按能力 UUID,任务 5):
// 单事务: 软删活跃能力 → 递增 EffectsRevision → 失效语言模式缓存;
// 无活跃能力/跨道人返回 ErrorTypeRecordNotFound;原实例保持 consumed_by_agent 不返还
func (d *AgentDao) RemoveAgentPillEffectByUUID(ctx context.Context, agentUID string, effectUUID uuid.UUID, now time.Time) errors.Error {
	txErr := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ok, err := RemoveActiveEffectByUUID(tx, agentUID, effectUUID, now)
		if err != nil {
			return err
		}
		if !ok {
			return errors.ErrorRecordNotFound("dao.agent.remove_effect_uuid")
		}
		if err := IncrementEffectsRevision(tx, agentUID); err != nil {
			return err
		}
		return InvalidateLanguagePatternTx(tx, agentUID)
	})
	if txErr == nil {
		return nil
	}
	if appErr, ok := txErr.(errors.Error); ok {
		return appErr
	}
	return errors.ErrorServerInternalError("dao.agent.remove_effect_uuid")
}

// UpdateActiveEffectsCAS 全量编排提交(任务 5): 单事务乐观锁
// (effects_revision 必须等于 expected,否则不写任何变更返回 false)
// → 逐条更新 weight/sort_order → 递增 EffectsRevision → 失效语言模式缓存。
// 集合校验(提交集==活跃集)由 service 在读取快照后执行;快照过期由乐观锁拦截
func (d *AgentDao) UpdateActiveEffectsCAS(ctx context.Context, agentUID string, expectedEffectsRevision int, writes []idao.EffectWrite) (bool, errors.Error) {
	ok := false
	txErr := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		ok, err = UpdateActiveEffectsCASCAS(tx, agentUID, expectedEffectsRevision, writes)
		return err
	})
	if txErr != nil {
		if appErr, ok := txErr.(errors.Error); ok {
			return false, appErr
		}
		return false, errors.ErrorServerInternalError("dao.agent.update_active_effects")
	}
	return ok, nil
}
