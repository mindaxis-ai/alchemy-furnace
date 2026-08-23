// Package dao 道人数据访问实现(新架构 internal 分层;UUID 边界在此解析,内部联结仍用自增 ID)
package dao

import (
	"context"
	"fmt"

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
	if err := GetDB().WithContext(ctx).Where("uuid = ?", uid.String()).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.agent.take_by_uuid")
		}
		return nil, errors.ErrorServerInternalError("dao.agent.take_by_uuid")
	}
	return &agent, nil
}

// TakeAgentDetailByUUID 按 UUID 查询道人详情(预加载服用记录+金丹+语言模式缓存)
func (d *AgentDao) TakeAgentDetailByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error) {
	var agent model.DaoAgent
	if err := GetDB().WithContext(ctx).
		Preload("AgentPills", func(db *gorm.DB) *gorm.DB {
			return db.Order("agent_pills.sort_order ASC, agent_pills.id ASC")
		}).
		Preload("AgentPills.Pill").
		Preload("LanguagePattern").
		Where("uuid = ?", uid.String()).
		First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.agent.take_detail_by_uuid")
		}
		return nil, errors.ErrorServerInternalError("dao.agent.take_detail_by_uuid")
	}
	return &agent, nil
}

// TakeAgentDetailByID 按内部自增 ID 查询道人详情(预加载服用记录+金丹+语言模式缓存)
func (d *AgentDao) TakeAgentDetailByID(ctx context.Context, agentID uint) (*model.DaoAgent, errors.Error) {
	var agent model.DaoAgent
	if err := GetDB().WithContext(ctx).
		Preload("AgentPills", func(db *gorm.DB) *gorm.DB {
			return db.Order("agent_pills.sort_order ASC, agent_pills.id ASC")
		}).
		Preload("AgentPills.Pill").
		Preload("LanguagePattern").
		Where("id = ?", agentID).
		First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.agent.take_detail_by_id")
		}
		return nil, errors.ErrorServerInternalError("dao.agent.take_detail_by_id")
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
func (d *AgentDao) CountSessionsByAgentID(ctx context.Context, agentID uint) (int64, errors.Error) {
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

// TakeAgentPill 查询单条服用记录
func (d *AgentDao) TakeAgentPill(ctx context.Context, agentID uint, pillID uint) (*model.AgentPill, errors.Error) {
	var ap model.AgentPill
	if err := GetDB().WithContext(ctx).Where("agent_id = ? AND pill_id = ?", agentID, pillID).First(&ap).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.agent.take_agent_pill")
		}
		return nil, errors.ErrorServerInternalError("dao.agent.take_agent_pill")
	}
	return &ap, nil
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

// DeleteAgentPill 删除服用记录
func (d *AgentDao) DeleteAgentPill(ctx context.Context, agentID uint, pillID uint) (int64, errors.Error) {
	result := GetDB().WithContext(ctx).Where("agent_id = ? AND pill_id = ?", agentID, pillID).Delete(&model.AgentPill{})
	if result.Error != nil {
		return 0, errors.ErrorServerInternalError("dao.agent.delete_agent_pill")
	}
	return result.RowsAffected, nil
}

// MaxAgentPillSortOrder 道人当前最大服用顺序
func (d *AgentDao) MaxAgentPillSortOrder(ctx context.Context, agentID uint) (int, errors.Error) {
	var maxOrder int
	if err := GetDB().WithContext(ctx).Model(&model.AgentPill{}).
		Where("agent_id = ?", agentID).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error; err != nil {
		return 0, errors.ErrorServerInternalError("dao.agent.max_sort_order")
	}
	return maxOrder, nil
}

// FindPillsByAgentID 道人已服用金丹列表(按服用顺序)
func (d *AgentDao) FindPillsByAgentID(ctx context.Context, agentID uint) ([]*model.ElixirPill, errors.Error) {
	var pills []*model.ElixirPill
	if err := GetDB().WithContext(ctx).Table("elixir_pills").
		Select("elixir_pills.*").
		Joins("JOIN agent_pills ON agent_pills.pill_id = elixir_pills.id").
		Where("agent_pills.agent_id = ?", agentID).
		Order("agent_pills.sort_order ASC, agent_pills.id ASC").
		Find(&pills).Error; err != nil {
		return nil, errors.ErrorServerInternalError("dao.agent.find_pills_by_agent")
	}
	return pills, nil
}

// ReplaceAgentPills 原子替换道人的完整服丹编排
// 单事务内: 删除全部旧关系 → 按请求顺序写新关系(sort_order=1..n) → 失效语言模式缓存
// 任一步失败由 GORM Transaction 回滚,旧关系与缓存状态保持不变
func (d *AgentDao) ReplaceAgentPills(ctx context.Context, agentID uint, pills []idao.AgentPillInput) errors.Error {
	txErr := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 删除旧关系
		if err := tx.Where("agent_id = ?", agentID).Delete(&model.AgentPill{}).Error; err != nil {
			return err
		}
		// 2) 校验 + 按请求顺序批量写新关系(sort_order 从 1 开始)
		if len(pills) > 0 {
			// 数据完整性: 重复 pill 拒绝、缺失 pill 回滚(不依赖各驱动 FK 配置差异)
			ids := make([]uint, 0, len(pills))
			seen := make(map[uint]struct{}, len(pills))
			for _, p := range pills {
				if _, dup := seen[p.PillID]; dup {
					return fmt.Errorf("duplicate pill id %d", p.PillID)
				}
				seen[p.PillID] = struct{}{}
				ids = append(ids, p.PillID)
			}
			var existCount int64
			if err := tx.Model(&model.ElixirPill{}).Where("id IN ?", ids).Count(&existCount).Error; err != nil {
				return err
			}
			if existCount != int64(len(ids)) {
				return fmt.Errorf("pill not found")
			}

			rows := make([]model.AgentPill, 0, len(pills))
			for i, p := range pills {
				rows = append(rows, model.AgentPill{
					AgentID:   agentID,
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
			Where("agent_id = ?", agentID).
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

// InvalidateLanguagePattern 失效道人语言模式缓存
func (d *AgentDao) InvalidateLanguagePattern(ctx context.Context, agentID uint) errors.Error {
	if err := GetDB().WithContext(ctx).Model(&model.LanguagePattern{}).
		Where("agent_id = ?", agentID).
		Update("is_valid", false).Error; err != nil {
		return errors.ErrorServerInternalError("dao.agent.invalidate_pattern")
	}
	return nil
}

// SaveLanguagePattern 写入/更新语言模式缓存(GORM Save: ID==0 创建,否则全字段更新)
func (d *AgentDao) SaveLanguagePattern(ctx context.Context, pattern *model.LanguagePattern) errors.Error {
	if err := GetDB().WithContext(ctx).Save(pattern).Error; err != nil {
		return errors.ErrorServerInternalError("dao.agent.save_language_pattern")
	}
	return nil
}
