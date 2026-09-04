// Package dao 金丹数据访问实现(新架构 internal 分层;UUID 边界在此解析)
package dao

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PillDao dao.Pill 接口实现
type PillDao struct{}

// NewPillDao 构造金丹 DAO
func NewPillDao() *PillDao {
	return &PillDao{}
}

// TakePillByUUID 按对外 UUID 查询金丹
func (d *PillDao) TakePillByUUID(ctx context.Context, uid uuid.UUID) (*model.ElixirPill, errors.Error) {
	var pill model.ElixirPill
	if err := GetDB().WithContext(ctx).Where("uuid = ?", uid.String()).First(&pill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.pill.take_by_uuid")
		}
		return nil, errors.ErrorServerInternalError("dao.pill.take_by_uuid")
	}
	return &pill, nil
}

// FindPillsByUUIDs 按 UUID 批量查询金丹(结果顺序不保证,调用方自行排序;空入参返回 nil)
func (d *PillDao) FindPillsByUUIDs(ctx context.Context, uids []uuid.UUID) ([]*model.ElixirPill, errors.Error) {
	if len(uids) == 0 {
		return nil, nil
	}
	strs := make([]string, 0, len(uids))
	for _, u := range uids {
		strs = append(strs, u.String())
	}
	var pills []*model.ElixirPill
	if err := GetDB().WithContext(ctx).Where("uuid IN ?", strs).Find(&pills).Error; err != nil {
		return nil, errors.ErrorServerInternalError("dao.pill.find_by_uuids")
	}
	return pills, nil
}

// FindPills 分页查询金丹列表
func (d *PillDao) FindPills(ctx context.Context, page int, size int, keyword string, isBuiltin *bool) (int64, []*model.ElixirPill, errors.Error) {
	db := GetDB().WithContext(ctx).Model(&model.ElixirPill{})
	if keyword != "" {
		db = db.Where("name LIKE ? OR description LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if isBuiltin != nil {
		db = db.Where("is_builtin = ?", *isBuiltin)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.pill.find_pills_count")
	}
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}

	var pills []*model.ElixirPill
	if err := db.Order("is_builtin DESC, updated_at DESC").Offset((page - 1) * size).Limit(size).Find(&pills).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.pill.find_pills")
	}
	return total, pills, nil
}

// SavePill 新建金丹
func (d *PillDao) SavePill(ctx context.Context, pill *model.ElixirPill) errors.Error {
	if err := GetDB().WithContext(ctx).Create(pill).Error; err != nil {
		return errors.ErrorServerInternalError("dao.pill.save_pill")
	}
	return nil
}

// UpdatePill 部分更新金丹字段
func (d *PillDao) UpdatePill(ctx context.Context, pill *model.ElixirPill, updates map[string]any) errors.Error {
	if err := GetDB().WithContext(ctx).Model(pill).Updates(updates).Error; err != nil {
		return errors.ErrorServerInternalError("dao.pill.update_pill")
	}
	return nil
}

// DeletePill 删除金丹及服用记录(事务)
func (d *PillDao) DeletePill(ctx context.Context, pill *model.ElixirPill) errors.Error {
	if err := Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Where("pill_id = ?", pill.UUID.String()).Delete(&model.AgentPill{}).Error; err != nil {
			return err
		}
		return tx.WithContext(ctx).Delete(pill).Error
	}); err != nil {
		return errors.ErrorServerInternalError("dao.pill.delete_pill")
	}
	return nil
}

// FindAgentIDsByPillID 查询服用了指定金丹的道人 UUID 文本列表
// (agent_pills.agent_id/pill_id 列即 UUID 文本,无内部 ID 换算)
func (d *PillDao) FindAgentIDsByPillID(ctx context.Context, pillUID string) ([]string, errors.Error) {
	var agentIDs []string
	if err := GetDB().WithContext(ctx).Model(&model.AgentPill{}).
		Where("pill_id = ?", pillUID).
		Pluck("agent_id", &agentIDs).Error; err != nil {
		return nil, errors.ErrorServerInternalError("dao.pill.find_agent_ids")
	}
	return agentIDs, nil
}

// InvalidateLanguagePatternsByAgentIDs 批量失效道人(按 UUID 文本)的语言模式缓存
// (language_patterns.agent_id 列即 UUID 文本,无内部 ID 换算)
func (d *PillDao) InvalidateLanguagePatternsByAgentIDs(ctx context.Context, agentUIDs []string) errors.Error {
	if len(agentUIDs) == 0 {
		return nil
	}
	if err := GetDB().WithContext(ctx).Model(&model.LanguagePattern{}).
		Where("agent_id IN ?", agentUIDs).
		Update("is_valid", false).Error; err != nil {
		return errors.ErrorServerInternalError("dao.pill.invalidate_patterns")
	}
	return nil
}
