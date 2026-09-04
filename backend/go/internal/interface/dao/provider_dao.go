// Package dao 供应商数据访问接口(新架构 internal 分层;UUID 边界在实现层解析)
package dao

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// Provider 供应商配置数据访问接口
type Provider interface {
	// TakeProviderByUUID 按对外 UUID 查询供应商
	TakeProviderByUUID(ctx context.Context, uid uuid.UUID) (*model.LLMProvider, errors.Error)
	// FindProviders 分页查询供应商列表(enabled=nil 不筛选)
	FindProviders(ctx context.Context, page, size int, enabled *bool) (int64, []*model.LLMProvider, errors.Error)
	// SaveProvider 新建供应商
	SaveProvider(ctx context.Context, provider *model.LLMProvider) errors.Error
	// UpdateProvider 部分更新供应商字段
	UpdateProvider(ctx context.Context, provider *model.LLMProvider, updates map[string]any) errors.Error
	// DeleteProvider 删除供应商
	DeleteProvider(ctx context.Context, provider *model.LLMProvider) errors.Error
	// CountModelsByProvider 统计供应商下模型数量(删除前引用检查);providerUID 为供应商 UUID 文本,直接匹配 llm_models.provider_id
	CountModelsByProvider(ctx context.Context, providerUID string) (int64, errors.Error)
	// CountProvidersByName 统计同名供应商数量(唯一性校验,excludeUID 为空串时不排除)
	CountProvidersByName(ctx context.Context, name string, excludeUID string) (int64, errors.Error)
}
