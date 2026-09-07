// Package dao 数据访问接口定义(对齐 Luna-CY 模板 internal/interface/dao)
package dao

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// Chat 对话域数据访问接口(会话/消息)
type Chat interface {
	// TakeSessionByUUID 按对外 UUID 查询会话(预加载道人),不存在返回 ErrorTypeRecordNotFound;
	// 续跑入口经 run.SessionID(UUID 文本)复用本方法反查
	TakeSessionByUUID(ctx context.Context, uid uuid.UUID) (*model.ChatSession, errors.Error)

	// FindSessions 分页查询会话列表(agentID 非空时按道人 UUID 过滤),按更新时间倒序
	FindSessions(ctx context.Context, agentID string, page int, size int) (int64, []*model.ChatSession, errors.Error)

	// SaveSession 新建会话
	SaveSession(ctx context.Context, session *model.ChatSession) errors.Error

	// SaveGroupSession 原子创建群聊会话与成员(单事务,任一步失败整体回滚);
	// 成员 SessionID 由实现按新建会话 ID 回填
	SaveGroupSession(ctx context.Context, session *model.ChatSession, members []*model.SessionMember) errors.Error

	// UpdateSession 按字段 map 部分更新会话(如标题)
	UpdateSession(ctx context.Context, session *model.ChatSession, updates map[string]any) errors.Error

	// DeleteSession 删除会话(消息由 FK CASCADE 清理)
	DeleteSession(ctx context.Context, session *model.ChatSession) errors.Error

	// FindMessages 从最新消息向前分页，每页内部按时间正序呈现(page=1 为最新一页)
	FindMessages(ctx context.Context, sessionID string, page int, size int) (int64, []*model.ChatMessage, errors.Error)

	// TakeLatestUserMessage 查询会话最新用户消息(created_at/主键 倒序)，不受历史分页影响。
	// sessionID 为会话 UUID 文本(011 业务键)。
	TakeLatestUserMessage(ctx context.Context, sessionID string) (*model.ChatMessage, errors.Error)

	// SaveMessage 写入消息并刷新所属会话 updated_at
	SaveMessage(ctx context.Context, message *model.ChatMessage) errors.Error

	// SaveMembers 批量写入群成员(调用方保证不重复;sort_order 由调用方赋值)
	SaveMembers(ctx context.Context, members []*model.SessionMember) errors.Error

	// FindMembers 按发言顺序(SortOrder ASC)查询群成员,预加载 Agent
	// sessionID 为会话 UUID 文本(011 业务键)。
	FindMembers(ctx context.Context, sessionID string) ([]*model.SessionMember, errors.Error)

	// FindMembersBySessionIDs 批量查询多会话成员(WHERE session_id IN),预加载 Agent,
	// 按 session_id/sort_order/session_member_id 排序后按会话分组;空输入直接返回空 map 不访问数据库
	FindMembersBySessionIDs(ctx context.Context, sessionIDs []string) (map[string][]*model.SessionMember, errors.Error)

	// DeleteMember 移出群成员;不存在返回 ErrorTypeRecordNotFound
	DeleteMember(ctx context.Context, sessionUUID string, agentUUID string) errors.Error

	// CreateRun 写入编排 run 初始行(status=pending)
	CreateRun(ctx context.Context, run *model.ChatRun) errors.Error

	// UpdateRunStatus 按状态机校验并迁移 run 状态;合法则落库并回填 run.Status;
	// 同状态重复更新为幂等 no-op;非法迁移返回 ErrorInvalidRequest
	UpdateRunStatus(ctx context.Context, run *model.ChatRun, status string) errors.Error

	// TakeRunByUUID 按对外 UUID 查询 run,不存在返回 ErrorTypeRecordNotFound
	TakeRunByUUID(ctx context.Context, uid uuid.UUID) (*model.ChatRun, errors.Error)

	// SaveFinalReplyOnce 幂等落库最终回复消息:以 (run_id, reply_id) 为幂等键,
	// 已存在则直接返回已有行(不比较内容);消息 RunID/ReplyID 由实现回填;
	// 未知 run 返回 ErrorTypeRecordNotFound
	SaveFinalReplyOnce(ctx context.Context, runUUID uuid.UUID, replyID string, message *model.ChatMessage) (*model.ChatMessage, errors.Error)
}
