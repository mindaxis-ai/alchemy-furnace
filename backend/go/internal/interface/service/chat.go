// Package service 业务逻辑接口定义(对齐 Luna-CY 模板 internal/interface/service)
package service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// LanguagePatternProvider 语言模式合成提供方(由 language_pattern_service 实现)
// 对话服务依赖它获取/重建道人的系统提示词缓存
type LanguagePatternProvider interface {
	// GetOrBuildPattern 获取道人语言模式: 缓存命中(is_valid 且指纹一致)直接返回,否则调用合成引擎重建并写回
	GetOrBuildPattern(ctx context.Context, agentID uint) (*model.LanguagePattern, errors.Error)
}

// Chat 对话域业务逻辑接口(会话/消息/SSE 流式对话)
// 对外以 UUID 标识会话;内部联结仍用自增 ID。SSE 入口按 session UUID 解析
type Chat interface {
	// CreateSession 创建 1v1 会话;agentUID 为道人对外 UUID
	// 标题一律留空,由首个问答自动命名;group 会话走 Service 扩展入口
	CreateSession(ctx context.Context, agentUID uuid.UUID) (*model.ChatSession, errors.Error)

	// ListSessions 分页查询会话列表(agentUID 非零时按道人过滤),按更新时间倒序
	ListSessions(ctx context.Context, agentUID uuid.UUID, page int, size int) (int64, []*model.ChatSession, errors.Error)

	// GetMessages 从最新消息向前分页，每页内部按时间正序呈现(page=1 为最新一页)
	GetMessages(ctx context.Context, sessionUID uuid.UUID, page int, size int) (int64, []*model.ChatMessage, errors.Error)

	// TakeLatestUserMessage 查询会话最新用户消息，不受历史列表分页影响。
	TakeLatestUserMessage(ctx context.Context, sessionID uint) (*model.ChatMessage, errors.Error)

	// GetSessionAgentInfo 按会话 UUID 取会话(预加载道人),供 SSE 构建对话请求(session.ID/AgentID/Agent.ModelName)
	GetSessionAgentInfo(ctx context.Context, sessionUID uuid.UUID) (*model.ChatSession, errors.Error)

	// AuthorizeSessionForStream 校验单聊会话当前道人仍可用，并返回本轮已校验凭证。
	// 历史读取不调用此方法，停用道人历史仍保持可读。
	AuthorizeSessionForStream(ctx context.Context, session *model.ChatSession) (*credential.ModelCredentials, errors.Error)

	// GetOrBuildPattern 获取道人语言模式(委托 LanguagePatternProvider)
	GetOrBuildPattern(ctx context.Context, agentID uint) (*model.LanguagePattern, errors.Error)

	// ResolveCredentials 解析模型调用凭证(每轮解析,模型停用/换钥即时生效)
	ResolveCredentials(ctx context.Context, modelName string) (*credential.ModelCredentials, errors.Error)

	// StreamChat 调用语言引擎流式对话并逐块回调,返回完整内容与取消标记
	// ctx 取消时返回已累积的部分内容与 canceled=true,err 为 nil;引擎错误映射为可读中文描述
	StreamChat(ctx context.Context, messages []map[string]string, creds *credential.ModelCredentials, onChunk func(string)) (fullContent string, canceled bool, err error)

	// SaveMessage 写入消息并刷新所属会话 updated_at(sources 字段已废弃,不再写入)
	SaveMessage(ctx context.Context, sessionID uint, role string, content string) (*model.ChatMessage, errors.Error)

	// DeleteSession 删除会话(消息由 FK CASCADE 清理)
	DeleteSession(ctx context.Context, sessionUID uuid.UUID) errors.Error

	// UpdateSessionTitle 更新会话标题(trim 后空或 >30 字返 InvalidRequest)
	UpdateSessionTitle(ctx context.Context, sessionUID uuid.UUID, title string) errors.Error

	// CreateGroupSession 建群:成员≥2、去重、全部 active;title 置空待自动命名
	CreateGroupSession(ctx context.Context, agentUIDs []uuid.UUID) (*model.ChatSession, errors.Error)

	// ListMembers 列群成员(按发言顺序,预加载道人)
	ListMembers(ctx context.Context, sessionUID uuid.UUID) ([]*model.SessionMember, errors.Error)

	// AddMembers 邀请入群(已在群的静默跳过),落系统通知消息
	AddMembers(ctx context.Context, sessionUID uuid.UUID, agentUIDs []uuid.UUID) errors.Error

	// RemoveMember 踢出群,落系统通知消息;不在群返回 ErrorTypeRecordNotFound
	RemoveMember(ctx context.Context, sessionUID uuid.UUID, agentUID uuid.UUID) errors.Error

	// SaveAgentMessage 写带道人归属与提及的消息(群聊编排器用)
	SaveAgentMessage(ctx context.Context, sessionID uint, agentID uint, role string, content string, mentions model.JSONMap) (*model.ChatMessage, errors.Error)

	// GenerateSessionTitle 单聊自动命名入口:title 已非空(用户手改)放弃;失败返回 ""
	GenerateSessionTitle(ctx context.Context, sessionUID uuid.UUID, userContent string, firstReply string) string

	// RunGroupTurn 群聊回合编排:落用户消息→≤3轮逐道人发言→自动命名→turn_done
	// emit 由 handler 提供(带锁 + 心跳)
	RunGroupTurn(ctx context.Context, sessionUID uuid.UUID, content string, emit func(event string, payload any))

	// RetryGroupTurn 重试最近一个同内容用户回合，不重复保存用户消息。
	RetryGroupTurn(ctx context.Context, sessionUID uuid.UUID, content string, emit func(event string, payload any))
}
