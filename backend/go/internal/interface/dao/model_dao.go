package dao

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// Model 模型配置数据访问接口(US2 完整模型域;对外标识为 uuid 文本业务键,入参 uid 仍为 uuid.UUID 类型)
type Model interface {
	// CountEnabledModelByName 统计「已启用供应商下的已启用模型」中指定模型名的数量(道人模型名校验)
	CountEnabledModelByName(ctx context.Context, name string) (int64, errors.Error)

	// TakeModelByUUID 按对外 UUID 查询模型(预加载 Provider),不存在返回 ErrorTypeRecordNotFound
	TakeModelByUUID(ctx context.Context, uid uuid.UUID) (*model.LLMModel, errors.Error)

	// FindModelsByProvider 分页查询指定供应商下的模型列表(按 sort_order,llm_model_id 排序);providerID 为供应商业务键文本
	FindModelsByProvider(ctx context.Context, providerID string, page, size int) (int64, []*model.LLMModel, errors.Error)

	// CountModelsByNameInProvider 统计同供应商下同名模型数量(excludeUID 为空串时不排除);providerID/excludeUID 均为业务键文本
	CountModelsByNameInProvider(ctx context.Context, providerID string, name string, excludeUID string) (int64, errors.Error)

	// ModelNameExistsInProvider 同供应商下模型名是否已被其他记录占用(excludeUID 为空串时不排除)
	ModelNameExistsInProvider(ctx context.Context, providerID string, name string, excludeUID string) (bool, errors.Error)

	// SaveModel 新建模型;is_default/is_synthesis 为 true 时事务内先清除其他记录
	SaveModel(ctx context.Context, m *model.LLMModel) errors.Error

	// UpdateModel 部分更新模型字段;is_default/is_synthesis 为 true 时事务内先清除其他记录(排除自身)
	// 标志位从 updates map 中按 bool 读取
	UpdateModel(ctx context.Context, m *model.LLMModel, updates map[string]any) errors.Error

	// DeleteModel 删除模型
	DeleteModel(ctx context.Context, m *model.LLMModel) errors.Error

	// CountAgentReferencesByName 统计引用指定模型名的道人数量(dao_agents.model_name = name)
	CountAgentReferencesByName(ctx context.Context, name string) (int64, errors.Error)

	// CountAgentReferencesByNames 批量统计引用数量,返回 model_name -> count 映射
	CountAgentReferencesByNames(ctx context.Context, names []string) (map[string]int64, errors.Error)

	// FindModelsByName 按模型名查询全部记录(预加载 Provider,按 sort_order,llm_model_id 排序),供凭证解析链使用
	FindModelsByName(ctx context.Context, name string) ([]*model.LLMModel, errors.Error)

	// TakeDefaultEnabled 取已启用的默认模型(预加载 Provider),不存在返回 ErrorTypeRecordNotFound
	TakeDefaultEnabled(ctx context.Context) (*model.LLMModel, errors.Error)

	// TakeSynthesisEnabled 取已启用的合成专用模型(预加载 Provider),不存在返回 ErrorTypeRecordNotFound
	TakeSynthesisEnabled(ctx context.Context) (*model.LLMModel, errors.Error)

	// TakeFusionEnabled 取已启用的金丹融合专用模型(预加载 Provider),不存在返回 ErrorTypeRecordNotFound
	TakeFusionEnabled(ctx context.Context) (*model.LLMModel, errors.Error)

	// FindEnabledOptions 已启用供应商下的已启用模型精简列表(供道人表单下拉)
	FindEnabledOptions(ctx context.Context) ([]model.LLMModelOption, errors.Error)

	// FindFirstEnabledModelByProvider 取供应商下第一个已启用模型(连接测试回退用),无则 ErrorTypeRecordNotFound;providerID 为供应商业务键文本
	FindFirstEnabledModelByProvider(ctx context.Context, providerID string) (*model.LLMModel, errors.Error)
}
