// Package dao 模型配置数据访问实现(新架构 internal 分层;UUID 边界在此解析,内部联结仍用自增 ID)
package dao

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ModelDao 模型配置查询实现
type ModelDao struct{}

// NewModelDao 构造模型 DAO
func NewModelDao() *ModelDao {
	return &ModelDao{}
}

// CountEnabledModelByName 统计「已启用供应商下的已启用模型」中指定模型名的数量
func (d *ModelDao) CountEnabledModelByName(ctx context.Context, name string) (int64, errors.Error) {
	var count int64
	if err := GetDB().WithContext(ctx).Table("llm_models").
		Joins("JOIN llm_providers ON llm_providers.id = llm_models.provider_id").
		Where("llm_models.name = ? AND llm_models.is_enabled = ? AND llm_providers.is_enabled = ?", name, true, true).
		Count(&count).Error; err != nil {
		return 0, errors.ErrorServerInternalError("dao.model.count_enabled_by_name")
	}
	return count, nil
}

// TakeModelByUUID 按对外 UUID 查询模型(预加载 Provider)
func (d *ModelDao) TakeModelByUUID(ctx context.Context, uid uuid.UUID) (*model.LLMModel, errors.Error) {
	var m model.LLMModel
	if err := GetDB().WithContext(ctx).Preload("Provider").Where("uuid = ?", uid.String()).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.model.take_by_uuid")
		}
		return nil, errors.ErrorServerInternalError("dao.model.take_by_uuid")
	}
	return &m, nil
}

// TakeModelByID 按内部自增 ID 查询模型(预加载 Provider)
func (d *ModelDao) TakeModelByID(ctx context.Context, id uint) (*model.LLMModel, errors.Error) {
	var m model.LLMModel
	if err := GetDB().WithContext(ctx).Preload("Provider").First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.model.take_by_id")
		}
		return nil, errors.ErrorServerInternalError("dao.model.take_by_id")
	}
	return &m, nil
}

// FindModelsByProvider 分页查询指定供应商下的模型列表(按 sort_order,id 排序)
func (d *ModelDao) FindModelsByProvider(ctx context.Context, providerID uint, page, size int) (int64, []*model.LLMModel, errors.Error) {
	db := GetDB().WithContext(ctx).Model(&model.LLMModel{}).Where("provider_id = ?", providerID)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.model.find_by_provider_count")
	}
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}

	var models []*model.LLMModel
	if err := db.Order("sort_order ASC, id ASC").Offset((page - 1) * size).Limit(size).Find(&models).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.model.find_by_provider")
	}
	return total, models, nil
}

// CountModelsByNameInProvider 统计同供应商下同名模型数量(excludeID=0 时不排除)
func (d *ModelDao) CountModelsByNameInProvider(ctx context.Context, providerID uint, name string, excludeID uint) (int64, errors.Error) {
	var count int64
	q := GetDB().WithContext(ctx).Model(&model.LLMModel{}).Where("provider_id = ? AND name = ?", providerID, name)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	if err := q.Count(&count).Error; err != nil {
		return 0, errors.ErrorServerInternalError("dao.model.count_by_name_in_provider")
	}
	return count, nil
}

// ModelNameExistsInProvider 同供应商下模型名是否已被其他记录占用
func (d *ModelDao) ModelNameExistsInProvider(ctx context.Context, providerID uint, name string, excludeID uint) (bool, errors.Error) {
	count, err := d.CountModelsByNameInProvider(ctx, providerID, name, excludeID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// SaveModel 新建模型;is_default/is_synthesis/is_fusion 为 true 时事务内先清除其他记录
func (d *ModelDao) SaveModel(ctx context.Context, m *model.LLMModel) errors.Error {
	if err := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if m.IsDefault {
			if err := tx.Model(&model.LLMModel{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if m.IsSynthesis {
			if err := tx.Model(&model.LLMModel{}).Where("is_synthesis = ?", true).Update("is_synthesis", false).Error; err != nil {
				return err
			}
		}
		if m.IsFusion {
			if err := tx.Model(&model.LLMModel{}).Where("is_fusion = ?", true).Update("is_fusion", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(m).Error
	}); err != nil {
		return errors.ErrorServerInternalError("dao.model.save")
	}
	return nil
}

// UpdateModel 部分更新模型字段;is_default/is_synthesis/is_fusion 为 true 时事务内先清除其他记录(排除自身)
// 标志位从 updates map 中按 bool 读取
func (d *ModelDao) UpdateModel(ctx context.Context, m *model.LLMModel, updates map[string]any) errors.Error {
	if err := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if v, ok := updates["is_default"]; ok {
			if b, ok := toBool(v); ok && b {
				if err := tx.Model(&model.LLMModel{}).Where("is_default = ? AND id <> ?", true, m.ID).Update("is_default", false).Error; err != nil {
					return err
				}
			}
		}
		if v, ok := updates["is_synthesis"]; ok {
			if b, ok := toBool(v); ok && b {
				if err := tx.Model(&model.LLMModel{}).Where("is_synthesis = ? AND id <> ?", true, m.ID).Update("is_synthesis", false).Error; err != nil {
					return err
				}
			}
		}
		if v, ok := updates["is_fusion"]; ok {
			if b, ok := toBool(v); ok && b {
				if err := tx.Model(&model.LLMModel{}).Where("is_fusion = ? AND id <> ?", true, m.ID).Update("is_fusion", false).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(m).Updates(updates).Error
	}); err != nil {
		return errors.ErrorServerInternalError("dao.model.update")
	}
	return nil
}

// DeleteModel 删除模型
func (d *ModelDao) DeleteModel(ctx context.Context, m *model.LLMModel) errors.Error {
	if err := GetDB().WithContext(ctx).Delete(m).Error; err != nil {
		return errors.ErrorServerInternalError("dao.model.delete")
	}
	return nil
}

// CountAgentReferencesByName 统计引用指定模型名的道人数量(dao_agents.model_name = name)
func (d *ModelDao) CountAgentReferencesByName(ctx context.Context, name string) (int64, errors.Error) {
	var count int64
	if err := GetDB().WithContext(ctx).Model(&model.DaoAgent{}).Where("model_name = ?", name).Count(&count).Error; err != nil {
		return 0, errors.ErrorServerInternalError("dao.model.count_agent_refs")
	}
	return count, nil
}

// CountAgentReferencesByNames 批量统计引用数量,返回 model_name -> count 映射
func (d *ModelDao) CountAgentReferencesByNames(ctx context.Context, names []string) (map[string]int64, errors.Error) {
	counts := make(map[string]int64)
	if len(names) == 0 {
		return counts, nil
	}
	type row struct {
		ModelName string
		Cnt       int64
	}
	var rows []row
	if err := GetDB().WithContext(ctx).Model(&model.DaoAgent{}).
		Select("model_name, COUNT(*) AS cnt").
		Where("model_name IN ?", names).
		Group("model_name").
		Scan(&rows).Error; err != nil {
		return nil, errors.ErrorServerInternalError("dao.model.count_agent_refs_batch")
	}
	for _, r := range rows {
		counts[r.ModelName] = r.Cnt
	}
	return counts, nil
}

// FindModelsByName 按模型名查询全部记录(预加载 Provider,按 sort_order,id 排序),供凭证解析链使用
func (d *ModelDao) FindModelsByName(ctx context.Context, name string) ([]*model.LLMModel, errors.Error) {
	var models []*model.LLMModel
	if err := GetDB().WithContext(ctx).Preload("Provider").
		Where("name = ?", name).
		Order("sort_order ASC, id ASC").
		Find(&models).Error; err != nil {
		return nil, errors.ErrorServerInternalError("dao.model.find_by_name")
	}
	return models, nil
}

// TakeDefaultEnabled 取已启用的默认模型(预加载 Provider),不存在返回 ErrorTypeRecordNotFound
func (d *ModelDao) TakeDefaultEnabled(ctx context.Context) (*model.LLMModel, errors.Error) {
	var m model.LLMModel
	if err := GetDB().WithContext(ctx).Preload("Provider").
		Where("is_default = ? AND is_enabled = ?", true, true).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.model.take_default")
		}
		return nil, errors.ErrorServerInternalError("dao.model.take_default")
	}
	return &m, nil
}

// TakeSynthesisEnabled 取已启用的合成专用模型(预加载 Provider),不存在返回 ErrorTypeRecordNotFound
func (d *ModelDao) TakeSynthesisEnabled(ctx context.Context) (*model.LLMModel, errors.Error) {
	var m model.LLMModel
	if err := GetDB().WithContext(ctx).Preload("Provider").
		Where("is_synthesis = ? AND is_enabled = ?", true, true).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.model.take_synthesis")
		}
		return nil, errors.ErrorServerInternalError("dao.model.take_synthesis")
	}
	return &m, nil
}

// TakeFusionEnabled 取已启用的金丹融合专用模型(预加载 Provider),不存在返回 ErrorTypeRecordNotFound
func (d *ModelDao) TakeFusionEnabled(ctx context.Context) (*model.LLMModel, errors.Error) {
	var m model.LLMModel
	if err := GetDB().WithContext(ctx).Preload("Provider").
		Where("is_fusion = ? AND is_enabled = ?", true, true).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.model.take_fusion")
		}
		return nil, errors.ErrorServerInternalError("dao.model.take_fusion")
	}
	return &m, nil
}

// FindEnabledOptions 已启用供应商下的已启用模型精简列表(供道人表单下拉)
func (d *ModelDao) FindEnabledOptions(ctx context.Context) ([]model.LLMModelOption, errors.Error) {
	type optionRow struct {
		Name                string
		DisplayName         string
		IsDefault           bool
		ProviderName        string
		ProviderDisplayName string
	}
	var rows []optionRow
	if err := GetDB().WithContext(ctx).Table("llm_models").
		Select("llm_models.name, llm_models.display_name, llm_models.is_default, llm_providers.name AS provider_name, llm_providers.display_name AS provider_display_name").
		Joins("JOIN llm_providers ON llm_providers.id = llm_models.provider_id").
		Where("llm_models.is_enabled = ? AND llm_providers.is_enabled = ?", true, true).
		Order("llm_providers.sort_order ASC, llm_providers.id ASC, llm_models.sort_order ASC, llm_models.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, errors.ErrorServerInternalError("dao.model.find_enabled_options")
	}

	options := make([]model.LLMModelOption, 0, len(rows))
	for _, r := range rows {
		options = append(options, model.LLMModelOption{
			Name:                r.Name,
			DisplayName:         r.DisplayName,
			ProviderName:        r.ProviderName,
			ProviderDisplayName: r.ProviderDisplayName,
			IsDefault:           r.IsDefault,
		})
	}
	return options, nil
}

// FindFirstEnabledModelByProvider 取供应商下第一个已启用模型(连接测试回退用),无则 ErrorTypeRecordNotFound
func (d *ModelDao) FindFirstEnabledModelByProvider(ctx context.Context, providerID uint) (*model.LLMModel, errors.Error) {
	var m model.LLMModel
	if err := GetDB().WithContext(ctx).
		Where("provider_id = ? AND is_enabled = ?", providerID, true).
		Order("sort_order ASC, id ASC").First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.model.find_first_enabled")
		}
		return nil, errors.ErrorServerInternalError("dao.model.find_first_enabled")
	}
	return &m, nil
}

// toBool 将任意值安全转换为 bool(updates map 中的 is_default/is_synthesis 标志)
func toBool(v any) (bool, bool) {
	if b, ok := v.(bool); ok {
		return b, true
	}
	return false, false
}
