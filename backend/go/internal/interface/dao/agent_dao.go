// Package dao 数据访问接口定义(对齐 Luna-CY 模板 internal/interface/dao)
// 接口入参为对外 UUID 或内部模型,返回内部模型;错误一律为 errors.Error 类型化错误
package dao

import (
	"context"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// AgentPillInput 服丹编排输入项(金丹业务 UUID 文本 + 权重)
type AgentPillInput struct {
	PillID string
	Weight float64
}

// Agent 道人数据访问接口
type Agent interface {
	// TakeAgentByUUID 按对外 UUID 查询道人,不存在返回 ErrorTypeRecordNotFound
	TakeAgentByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error)

	// TakeAgentDetailByUUID 按 UUID 查询道人详情(预加载已吸收能力快照+语言模式缓存)
	TakeAgentDetailByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error)

	// TakeAgentDetailByID 按内部自增 ID 查询道人详情(预加载已吸收能力快照+语言模式缓存)
	// 供语言模式服务按 agentID 加载性格/能力快照/已有缓存
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
	CountSessionsByAgentID(ctx context.Context, agentID string) (int64, errors.Error)

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
	// 任务 3 起已无生产调用方(旧入口 410 封禁);仅保留历史表维护能力
	ReplaceAgentPills(ctx context.Context, agentUID string, pills []AgentPillInput) errors.Error

	// RemoveAgentPillEffect 移除道人的已吸收能力(软删保留历史,任务 3)
	// 单事务: 软删活跃能力(removed_at=now) → 递增 EffectsRevision → 失效语言模式缓存
	// 无活跃能力返回 ErrorTypeRecordNotFound;原金丹实例保持 consumed_by_agent 不返还库存
	// itemUUID 为金丹实例 UUID(与语言模式指纹/turn policy 身份一致)
	// agentUID 为道人 UUID 文本(011 业务键)
	RemoveAgentPillEffect(ctx context.Context, agentUID string, itemUUID uuid.UUID, now time.Time) errors.Error

	// UpdateAgentPillEffect 更新活跃能力权重/顺序(实例 UUID 标识,任务 3)
	// 单事务: 更新(weight/sortOrder 均为 nil 时仅校验存在) → 递增 EffectsRevision → 失效缓存
	// 无活跃能力返回 ErrorTypeRecordNotFound
	UpdateAgentPillEffect(ctx context.Context, agentUID string, itemUUID uuid.UUID, weight *float64, sortOrder *int) errors.Error

	// InvalidateLanguagePattern 将道人语言模式缓存标记为失效
	// agentUID 为道人 UUID 文本(011 业务键)
	InvalidateLanguagePattern(ctx context.Context, agentUID string) errors.Error

	// SaveLanguagePattern 写入/更新语言模式缓存(GORM Save: ID==0 创建,否则全字段更新)
	SaveLanguagePattern(ctx context.Context, pattern *model.LanguagePattern) errors.Error

	// SaveLanguagePatternIfRevision 写入/更新语言模式缓存,带能力编排版本 CAS 核对:
	// 读取时记录 EffectsRevision,写回前事务内核对,已变则返回 ErrorTypeConflict
	// "agent.effects_conflict"(可重试) — 防止旧并发编译覆盖新能力(缓存保护 §3.2)
	SaveLanguagePatternIfRevision(ctx context.Context, pattern *model.LanguagePattern, expectedEffectsRevision int) errors.Error

	// ListActiveEffects 道人活跃能力列表（按 sort_order 升序，含来源实例/版本对外标识；任务 5）
	// agentUID 为道人 UUID 文本(011 业务键)
	ListActiveEffects(ctx context.Context, agentUID string) ([]EffectWithSource, errors.Error)

	// RemoveAgentPillEffectByUUID 移除道人的已吸收能力（按能力 UUID；任务 5）
	// 单事务: 软删活跃能力(removed_at=now) → 递增 EffectsRevision → 失效语言模式缓存
	// 无活跃能力返回 ErrorTypeRecordNotFound;原金丹实例保持 consumed_by_agent 不返还
	// agentUID 为道人 UUID 文本(011 业务键)
	RemoveAgentPillEffectByUUID(ctx context.Context, agentUID string, effectUUID uuid.UUID, now time.Time) errors.Error

	// UpdateActiveEffectsCAS 全量编排提交（任务 5）: 单事务乐观锁
	// （effects_revision 必须等于 expectedEffectsRevision,否则不写任何变更返回 false）
	// → 逐条更新 weight/sort_order → 递增 EffectsRevision → 失效语言模式缓存。
	// 集合校验（提交集==活跃集）由调用方在读取快照后执行;快照过期由乐观锁拦截
	// agentUID 为道人 UUID 文本(011 业务键)
	UpdateActiveEffectsCAS(ctx context.Context, agentUID string, expectedEffectsRevision int, writes []EffectWrite) (bool, errors.Error)
}

// EffectWithSource 能力快照 + 来源实例/版本对外标识（任务 5 列表输出）
type EffectWithSource struct {
	Effect       model.AgentPillEffect
	ItemUUID     uuid.UUID
	RevisionUUID uuid.UUID
}

// EffectWrite 全量编排写入项（内部 ID + 权重 + 顺序；任务 5）
type EffectWrite struct {
	EffectID  uint
	Weight    float64
	SortOrder int
}
