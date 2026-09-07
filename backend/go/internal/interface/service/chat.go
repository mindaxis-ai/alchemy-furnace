// Package service 业务逻辑接口定义(对齐 Luna-CY 模板 internal/interface/service)
package service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// LanguagePatternProvider 语言模式合成提供方(由 language_pattern_service 实现)
// 对话服务依赖它获取/重建道人的系统提示词缓存
type LanguagePatternProvider interface {
	// GetOrBuildPattern 获取道人语言模式: 缓存命中(is_valid 且指纹一致)直接返回,否则调用合成引擎重建并写回
	// agentUID 为道人 UUID 文本(011 业务键)
	GetOrBuildPattern(ctx context.Context, agentUID string) (*model.LanguagePattern, errors.Error)
}

// ChatReadiness 后端权威的可对话就绪状态:active 道人总数与通过正式凭证校验的道人 UUID 名单。
// 不携带任何凭证内容;单个道人不可用只使其缺席名单,不导致整体失败。
type ChatReadiness struct {
	ActiveAgentCount int
	ReadyAgentIDs    []uuid.UUID
}

// GenerationOptions 流式对话的显式生成选项(spec §7.2;禁止可变参数/隐藏默认值)
type GenerationOptions struct {
	MaxTokens    int
	MaxSentences int // Task 10:句数硬限制(完整句边界停止);<=0 表示不限制
}

// ConversationCommand 一次对话轮的输入(LangGraph 权威编排入口 RunConversation)。
// handler 只做参数校验与透传:编排、持久化、事件语义全部由服务层定夺。
type ConversationCommand struct {
	SessionUID  uuid.UUID // 会话 UUID(公共标识)
	Content     string    // 用户消息原文;Retry=true 时须与最近一条用户消息一致
	Retry       bool      // 重试:复用最近同内容用户消息,不重复落库
	DebugPrompt bool      // 显式开启模型输入调试(prompt_debug 事件)
}

// PromptDebugPayload 是仅在用户显式开启调试时通过 SSE 返回的实际模型输入。
// 凭证与 API 地址不属于模型消息，禁止加入该结构。
type PromptDebugPayload struct {
	AgentID    string              `json:"agent_id,omitempty"`
	AgentName  string              `json:"agent_name,omitempty"`
	Model      string              `json:"model"`
	Messages   []map[string]string `json:"messages"`
	Generation struct {
		MaxTokens    int `json:"max_tokens"`
		MaxSentences int `json:"max_sentences"`
	} `json:"generation"`
}

// NewPromptDebugPayload 从最终模型请求参数创建安全的调试快照。
func NewPromptDebugPayload(agentID, agentName, modelName string, messages []map[string]string, options GenerationOptions) PromptDebugPayload {
	payload := PromptDebugPayload{
		AgentID: agentID, AgentName: agentName, Model: modelName, Messages: messages,
	}
	payload.Generation.MaxTokens = options.MaxTokens
	payload.Generation.MaxSentences = options.MaxSentences
	return payload
}

// Chat 对话域业务逻辑接口(会话/消息/SSE 流式对话)
// 对外以 UUID 文本标识会话/道人;主键统一 uuid 文本。SSE 入口按 session UUID 解析
type Chat interface {
	// GetReadiness 汇总可发起正式对话的道人就绪状态;仅道人列表读取失败才返回错误
	GetReadiness(ctx context.Context) (*ChatReadiness, errors.Error)

	// CreateSession 创建 1v1 会话;agentUID 为道人对外 UUID
	// 标题一律留空,由首个问答自动命名;group 会话走 Service 扩展入口
	CreateSession(ctx context.Context, agentUID uuid.UUID) (*model.ChatSession, errors.Error)

	// ListSessions 分页查询会话列表(agentUID 非零时按道人过滤),按更新时间倒序
	ListSessions(ctx context.Context, agentUID uuid.UUID, page int, size int) (int64, []*model.ChatSession, errors.Error)

	// GetMessages 从最新消息向前分页，每页内部按时间正序呈现(page=1 为最新一页)
	GetMessages(ctx context.Context, sessionUID uuid.UUID, page int, size int) (int64, []*model.ChatMessage, errors.Error)

	// TakeLatestUserMessage 查询会话最新用户消息，不受历史列表分页影响;sessionUID 为会话 UUID 文本。
	TakeLatestUserMessage(ctx context.Context, sessionUID string) (*model.ChatMessage, errors.Error)

	// GetSessionAgentInfo 按会话 UUID 取会话(预加载道人),供 SSE 构建对话请求(ChatSessionID/AgentID/Agent.ModelName)
	GetSessionAgentInfo(ctx context.Context, sessionUID uuid.UUID) (*model.ChatSession, errors.Error)

	// SaveMessage 写入消息并刷新所属会话 updated_at(sources 字段已废弃,不再写入);sessionUID 为会话 UUID 文本
	SaveMessage(ctx context.Context, sessionUID string, role string, content string) (*model.ChatMessage, errors.Error)

	// DeleteSession 删除会话(消息由 FK CASCADE 清理)
	DeleteSession(ctx context.Context, sessionUID uuid.UUID) errors.Error

	// UpdateSessionTitle 更新会话标题(trim 后空或 >200 字返 InvalidRequest)
	UpdateSessionTitle(ctx context.Context, sessionUID uuid.UUID, title string) errors.Error

	// UpdateGroupAvatar 更新或清空群聊头像，单聊不接受此字段
	UpdateGroupAvatar(ctx context.Context, sessionUID uuid.UUID, avatar string) errors.Error

	// CreateGroupSession 建群:成员≥2、去重、全部 active;title 可选,trim 后为空则待自动命名
	CreateGroupSession(ctx context.Context, agentUIDs []uuid.UUID, title, avatar string) (*model.ChatSession, errors.Error)

	// ListMembers 列群成员(按发言顺序,预加载道人)
	ListMembers(ctx context.Context, sessionUID uuid.UUID) ([]*model.SessionMember, errors.Error)

	// AddMembers 邀请入群(已在群的静默跳过),落系统通知消息
	AddMembers(ctx context.Context, sessionUID uuid.UUID, agentUIDs []uuid.UUID) errors.Error

	// RemoveMember 踢出群,落系统通知消息;不在群返回 ErrorTypeRecordNotFound
	RemoveMember(ctx context.Context, sessionUID uuid.UUID, agentUID uuid.UUID) errors.Error

	// SaveAgentMessage 写带道人归属与提及的消息(群聊编排器用);sessionUID/agentUID 均为 UUID 文本
	SaveAgentMessage(ctx context.Context, sessionUID string, agentUID string, role string, content string, mentions model.JSONMap) (*model.ChatMessage, errors.Error)

	// GenerateSessionTitle 单聊自动命名入口:title 已非空(用户手改)放弃;失败返回 ""
	GenerateSessionTitle(ctx context.Context, sessionUID uuid.UUID, userContent string, firstReply string) string

	// RunConversation LangGraph 权威编排的统一对话轮入口(迁移期:单聊先行,群聊 Task 13 接线)。
	// 事件契约:accepted/chunk/prompt_debug/title/done/error/stopped;
	// 持久化语义:只落 assistant_final(幂等),取消/中断不保留部分回复。
	RunConversation(ctx context.Context, cmd ConversationCommand, emit func(event string, payload any))

	// RunConversationResume 续跑 interrupted run(Task 14):按 run 定位会话,以同一事件契约
	// 消费 Python Resume 流;不落用户消息、不新建 run、不发 accepted。
	RunConversationResume(ctx context.Context, runUID uuid.UUID, emit func(event string, payload any))

	// P3 记忆挂载:检索结果注入编排快照;蒸馏异步触发(实现为空实现=不启用);agentUID 为道人 UUID 文本
	RetrieveMemories(ctx context.Context, agentUID string, userMessage string) []MemorySnippet
	EnqueueMemoryDistillation(ctx context.Context, spec DistillationSpec) bool
}
