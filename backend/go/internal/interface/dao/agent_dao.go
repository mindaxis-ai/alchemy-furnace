// Package dao 数据访问接口定义(对齐 Luna-CY 模板 internal/interface/dao)
// 接口入参为对外 UUID 或内部模型,返回内部模型;错误一律为 errors.Error 类型化错误
package dao

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// AgentPillInput 服丹编排输入项(已解析的内部金丹 ID + 权重)
// DAO 只接受内部自增 ID,不解析 UUID、不做产品文案校验
type AgentPillInput struct {
	PillID uint
	Weight float64
}

// Agent 道人数据访问接口
type Agent interface {
	// TakeAgentByUUID 按对外 UUID 查询道人,不存在返回 ErrorTypeRecordNotFound
	TakeAgentByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error)

	// TakeAgentDetailByUUID 按 UUID 查询道人详情(预加载服用记录+金丹+语言模式缓存)
	TakeAgentDetailByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error)

	// TakeAgentDetailByID 按内部自增 ID 查询道人详情(预加载服用记录+金丹+语言模式缓存)
	// 供语言模式服务按 agentID 加载性格/金丹/已有缓存
	TakeAgentDetailByID(ctx context.Context, agentID uint) (*model.DaoAgent, errors.Error)

	// FindAgents 分页查询道人列表(status 为空不过滤),返回总数与当页数据
	FindAgents(ctx context.Context, page int, size int, status string) (int64, []*model.DaoAgent, errors.Error)

	// SaveAgent 新建道人
	SaveAgent(ctx context.Context, agent *model.DaoAgent) errors.Error

	// UpdateAgent 按字段 map 部分更新
	UpdateAgent(ctx context.Context, agent *model.DaoAgent, updates map[string]any) errors.Error

	// DeleteAgent 删除道人(会话/服用记录/语言缓存由 FK CASCADE 清理)
	DeleteAgent(ctx context.Context, agent *model.DaoAgent) errors.Error

	// CountSessionsByAgentID 统计道人参与的去重会话数
	// 单聊经 chat_sessions.agent_id,群聊经 session_members.agent_id(按 session 去重)
	CountSessionsByAgentID(ctx context.Context, agentID uint) (int64, errors.Error)

	// TakeAgentPill 查询单条服用记录,不存在返回 ErrorTypeRecordNotFound
	TakeAgentPill(ctx context.Context, agentID uint, pillID uint) (*model.AgentPill, errors.Error)

	// SaveAgentPill 新建服用记录
	SaveAgentPill(ctx context.Context, agentPill *model.AgentPill) errors.Error

	// UpdateAgentPill 按字段 map 部分更新服用记录
	UpdateAgentPill(ctx context.Context, agentPill *model.AgentPill, updates map[string]any) errors.Error

	// DeleteAgentPill 删除服用记录,返回受影响行数
	DeleteAgentPill(ctx context.Context, agentID uint, pillID uint) (int64, errors.Error)

	// MaxAgentPillSortOrder 道人当前最大服用顺序(无记录返回 0)
	MaxAgentPillSortOrder(ctx context.Context, agentID uint) (int, errors.Error)

	// FindPillsByAgentID 道人已服用金丹列表(按 sort_order,id 升序)
	FindPillsByAgentID(ctx context.Context, agentID uint) ([]*model.ElixirPill, errors.Error)

	// ReplaceAgentPills 原子替换道人的完整服丹编排
	// 单个事务: 删除全部旧关系 → 按请求顺序写新关系(sort_order=1..n) → 失效语言模式缓存
	// 任一步失败整体回滚,旧关系与缓存失效保持不变
	ReplaceAgentPills(ctx context.Context, agentID uint, pills []AgentPillInput) errors.Error

	// InvalidateLanguagePattern 将道人语言模式缓存标记为失效
	InvalidateLanguagePattern(ctx context.Context, agentID uint) errors.Error

	// SaveLanguagePattern 写入/更新语言模式缓存(GORM Save: ID==0 创建,否则全字段更新)
	SaveLanguagePattern(ctx context.Context, pattern *model.LanguagePattern) errors.Error
}
