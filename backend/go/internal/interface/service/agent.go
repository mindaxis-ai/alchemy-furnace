package service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// Agent 道人业务逻辑接口
type Agent interface {
	// ListAgents 分页查询道人列表(status 为空不过滤)
	ListAgents(ctx context.Context, page int, size int, status string) (int64, []*model.DaoAgent, errors.Error)

	// GetAgentDetailByUUID 按 UUID 获取道人详情(含服用记录与语言模式缓存)
	GetAgentDetailByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error)

	// CreateAgent 创建道人;model_name 非空时校验其引用已启用模型配置
	// proactivity 为 nil 时取默认值 50;合法区间 0-100,越界返回 InvalidRequest
	CreateAgent(ctx context.Context, name string, avatar string, personality string, modelName string, proactivity *int) (*model.DaoAgent, errors.Error)

	// UpdateAgent 按 UUID 部分更新道人(nil 字段不更新);性格变化时失效语言模式缓存
	// proactivity 合法区间 0-100;nil=不更新
	UpdateAgent(ctx context.Context, uid uuid.UUID, name *string, avatar *string, personality *string, modelName *string, status *string, proactivity *int) (*model.DaoAgent, errors.Error)

	// DeleteAgent 按 UUID 删除道人
	DeleteAgent(ctx context.Context, uid uuid.UUID) errors.Error

	// BindPill 道人服用金丹(双方按 UUID 解析);已绑定返回 ErrorTypeConflict
	BindPill(ctx context.Context, agentUID uuid.UUID, pillUID uuid.UUID, weight float64, sortOrder int) errors.Error

	// UpdateAgentPill 更新服用记录(weight/sort_order 为 nil 时不更新对应字段);未绑定返回 ErrorTypeRecordNotFound
	UpdateAgentPill(ctx context.Context, agentUID uuid.UUID, pillUID uuid.UUID, weight *float64, sortOrder *int) errors.Error

	// UnbindPill 解除绑定;未绑定返回 ErrorTypeRecordNotFound
	UnbindPill(ctx context.Context, agentUID uuid.UUID, pillUID uuid.UUID) errors.Error

	// ListAgentPills 道人已服用金丹列表(按服用顺序)
	ListAgentPills(ctx context.Context, agentUID uuid.UUID) ([]*model.ElixirPill, errors.Error)
}
