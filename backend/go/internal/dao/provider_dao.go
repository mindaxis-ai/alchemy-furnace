// Package dao 供应商数据访问实现(新架构 internal 分层;UUID 边界在此解析,内部联结仍用自增 ID)
package dao

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProviderDao dao.Provider 接口实现
type ProviderDao struct{}

// NewProviderDao 构造供应商 DAO
func NewProviderDao() *ProviderDao {
	return &ProviderDao{}
}

// TakeProviderByUUID 按对外 UUID 查询供应商
func (d *ProviderDao) TakeProviderByUUID(ctx context.Context, uid uuid.UUID) (*model.LLMProvider, errors.Error) {
	var p model.LLMProvider
	if err := GetDB().WithContext(ctx).Where("uuid = ?", uid.String()).First(&p).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.provider.take_by_uuid")
		}
		return nil, errors.ErrorServerInternalError("dao.provider.take_by_uuid")
	}
	return &p, nil
}

// FindProviders 分页查询供应商列表
func (d *ProviderDao) FindProviders(ctx context.Context, page, size int, enabled *bool) (int64, []*model.LLMProvider, errors.Error) {
	db := GetDB().WithContext(ctx).Model(&model.LLMProvider{})
	if enabled != nil {
		db = db.Where("is_enabled = ?", *enabled)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.provider.find_count")
	}
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}

	var providers []*model.LLMProvider
	if err := db.Order("sort_order ASC, id ASC").Offset((page - 1) * size).Limit(size).Find(&providers).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.provider.find")
	}
	return total, providers, nil
}

// SaveProvider 新建供应商
func (d *ProviderDao) SaveProvider(ctx context.Context, provider *model.LLMProvider) errors.Error {
	if err := GetDB().WithContext(ctx).Create(provider).Error; err != nil {
		return errors.ErrorServerInternalError("dao.provider.save")
	}
	return nil
}

// UpdateProvider 部分更新供应商字段
func (d *ProviderDao) UpdateProvider(ctx context.Context, provider *model.LLMProvider, updates map[string]any) errors.Error {
	if err := GetDB().WithContext(ctx).Model(provider).Updates(updates).Error; err != nil {
		return errors.ErrorServerInternalError("dao.provider.update")
	}
	return nil
}

// DeleteProvider 删除供应商
func (d *ProviderDao) DeleteProvider(ctx context.Context, provider *model.LLMProvider) errors.Error {
	if err := GetDB().WithContext(ctx).Delete(provider).Error; err != nil {
		return errors.ErrorServerInternalError("dao.provider.delete")
	}
	return nil
}

// CountModelsByProvider 统计供应商下模型数量;providerUID 为供应商 UUID 文本,直接匹配 llm_models.provider_id(011 业务键)
func (d *ProviderDao) CountModelsByProvider(ctx context.Context, providerUID string) (int64, errors.Error) {
	var count int64
	if err := GetDB().WithContext(ctx).Model(&model.LLMModel{}).Where("provider_id = ?", providerUID).Count(&count).Error; err != nil {
		return 0, errors.ErrorServerInternalError("dao.provider.count_models")
	}
	return count, nil
}

// CountProvidersByName 统计同名供应商数量(excludeUID 为空串时不排除;排除按业务键 uuid)
func (d *ProviderDao) CountProvidersByName(ctx context.Context, name string, excludeUID string) (int64, errors.Error) {
	var count int64
	q := GetDB().WithContext(ctx).Model(&model.LLMProvider{}).Where("name = ?", name)
	if excludeUID != "" {
		q = q.Where("uuid != ?", excludeUID)
	}
	if err := q.Count(&count).Error; err != nil {
		return 0, errors.ErrorServerInternalError("dao.provider.count_by_name")
	}
	return count, nil
}
