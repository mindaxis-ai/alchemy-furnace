// Package dao 对话域数据访问实现(新架构 internal 分层;UUID 边界在此解析,内部联结仍用自增 ID)
package dao

import (
	"context"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ChatDao dao.Chat 接口实现
type ChatDao struct{}

// NewChatDao 构造对话域 DAO
func NewChatDao() *ChatDao {
	return &ChatDao{}
}

// TakeSessionByUUID 按对外 UUID 查询会话(预加载道人),不存在返回 ErrorTypeRecordNotFound
func (d *ChatDao) TakeSessionByUUID(ctx context.Context, uid uuid.UUID) (*model.ChatSession, errors.Error) {
	var session model.ChatSession
	if err := GetDB().WithContext(ctx).
		Preload("Agent").
		Where("uuid = ?", uid.String()).
		First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.chat.take_session_by_uuid")
		}
		return nil, errors.ErrorServerInternalError("dao.chat.take_session_by_uuid")
	}
	return &session, nil
}

// FindSessions 分页查询会话列表(agentID>0 时按道人过滤),按更新时间倒序
func (d *ChatDao) FindSessions(ctx context.Context, agentID uint, page int, size int) (int64, []*model.ChatSession, errors.Error) {
	db := GetDB().WithContext(ctx).Model(&model.ChatSession{})
	if agentID > 0 {
		db = db.Where("agent_id = ?", agentID)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.chat.find_sessions_count")
	}
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}

	var sessions []*model.ChatSession
	if err := db.Preload("Agent").
		Order("updated_at DESC").
		Offset((page - 1) * size).Limit(size).
		Find(&sessions).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.chat.find_sessions")
	}
	return total, sessions, nil
}

// SaveSession 新建会话
func (d *ChatDao) SaveSession(ctx context.Context, session *model.ChatSession) errors.Error {
	if err := GetDB().WithContext(ctx).Create(session).Error; err != nil {
		return errors.ErrorServerInternalError("dao.chat.save_session")
	}
	return nil
}

// SaveGroupSession 原子创建群聊会话与成员(单事务;失败整体回滚,不在事务外补偿删除)
func (d *ChatDao) SaveGroupSession(ctx context.Context, session *model.ChatSession, members []*model.SessionMember) errors.Error {
	err := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(session).Error; err != nil {
			return err
		}
		for _, member := range members {
			member.SessionID = session.ID
		}
		if len(members) == 0 {
			return nil
		}
		return tx.Create(members).Error
	})
	if err != nil {
		return errors.ErrorServerInternalError("dao.chat.save_group_session")
	}
	return nil
}

// UpdateSession 按字段 map 部分更新会话(如标题)
func (d *ChatDao) UpdateSession(ctx context.Context, session *model.ChatSession, updates map[string]any) errors.Error {
	if err := GetDB().WithContext(ctx).Model(session).Updates(updates).Error; err != nil {
		return errors.ErrorServerInternalError("dao.chat.update_session")
	}
	return nil
}

// DeleteSession 删除会话(消息由 FK CASCADE 清理)
func (d *ChatDao) DeleteSession(ctx context.Context, session *model.ChatSession) errors.Error {
	if err := GetDB().WithContext(ctx).Delete(session).Error; err != nil {
		return errors.ErrorServerInternalError("dao.chat.delete_session")
	}
	return nil
}

// FindMessages 从最新消息向前分页，每页内部按时间正序呈现(page=1 为最新一页)
func (d *ChatDao) FindMessages(ctx context.Context, sessionID uint, page int, size int) (int64, []*model.ChatMessage, errors.Error) {
	db := GetDB().WithContext(ctx).Model(&model.ChatMessage{}).Where("session_id = ?", sessionID)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.chat.find_messages_count")
	}
	if page < 1 {
		page = 1
	}
	if total == 0 || size <= 0 {
		return total, nil, nil
	}
	pageEnd := total - int64((page-1)*size)
	if pageEnd <= 0 {
		return total, nil, nil
	}
	pageStart := pageEnd - int64(size)
	if pageStart < 0 {
		pageStart = 0
	}

	var messages []*model.ChatMessage
	if err := db.Preload("Agent").
		Order("created_at ASC").Order("id ASC").
		Offset(int(pageStart)).Limit(int(pageEnd - pageStart)).
		Find(&messages).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.chat.find_messages")
	}
	return total, messages, nil
}

// TakeLatestUserMessage 查询会话最新用户消息；ID 作为同时间戳下的稳定次序。
func (d *ChatDao) TakeLatestUserMessage(ctx context.Context, sessionID uint) (*model.ChatMessage, errors.Error) {
	var message model.ChatMessage
	if err := GetDB().WithContext(ctx).
		Where("session_id = ? AND role = ?", sessionID, "user").
		Order("created_at DESC").
		Order("id DESC").
		First(&message).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.chat.take_latest_user_message")
		}
		return nil, errors.ErrorServerInternalError("dao.chat.take_latest_user_message")
	}
	return &message, nil
}

// SaveMessage 写入消息并刷新所属会话 updated_at
func (d *ChatDao) SaveMessage(ctx context.Context, message *model.ChatMessage) errors.Error {
	failureCode := "dao.chat.save_message"
	if err := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		failureCode = "dao.chat.save_message_touch"
		return tx.Model(&model.ChatSession{}).
			Where("id = ?", message.SessionID).
			Update("updated_at", time.Now()).Error
	}); err != nil {
		return errors.ErrorServerInternalError(failureCode)
	}
	return nil
}

// SaveMembers 批量写入群成员
func (d *ChatDao) SaveMembers(ctx context.Context, members []*model.SessionMember) errors.Error {
	if len(members) == 0 {
		return nil
	}
	if err := GetDB().WithContext(ctx).Create(members).Error; err != nil {
		return errors.ErrorServerInternalError("dao.chat.save_members")
	}
	return nil
}

// FindMembers 按发言顺序查询群成员(预加载道人)
func (d *ChatDao) FindMembers(ctx context.Context, sessionID uint) ([]*model.SessionMember, errors.Error) {
	var members []*model.SessionMember
	if err := GetDB().WithContext(ctx).
		Preload("Agent").
		Where("session_id = ?", sessionID).
		Order("sort_order ASC").
		Find(&members).Error; err != nil {
		return nil, errors.ErrorServerInternalError("dao.chat.find_members")
	}
	return members, nil
}

// FindMembersBySessionIDs 批量查询多会话成员,预加载道人,按会话分组(消除列表 N+1)
func (d *ChatDao) FindMembersBySessionIDs(ctx context.Context, sessionIDs []uint) (map[uint][]*model.SessionMember, errors.Error) {
	grouped := map[uint][]*model.SessionMember{}
	if len(sessionIDs) == 0 {
		return grouped, nil
	}
	var members []*model.SessionMember
	if err := GetDB().WithContext(ctx).
		Preload("Agent").
		Where("session_id IN ?", sessionIDs).
		Order("session_id ASC, sort_order ASC, id ASC").
		Find(&members).Error; err != nil {
		return nil, errors.ErrorServerInternalError("dao.chat.find_members_by_session_ids")
	}
	for _, member := range members {
		grouped[member.SessionID] = append(grouped[member.SessionID], member)
	}
	return grouped, nil
}

// DeleteMember 移出群成员
func (d *ChatDao) DeleteMember(ctx context.Context, sessionID uint, agentID uint) errors.Error {
	res := GetDB().WithContext(ctx).
		Where("session_id = ? AND agent_id = ?", sessionID, agentID).
		Delete(&model.SessionMember{})
	if res.Error != nil {
		return errors.ErrorServerInternalError("dao.chat.delete_member")
	}
	if res.RowsAffected == 0 {
		return errors.ErrorRecordNotFound("dao.chat.delete_member")
	}
	return nil
}

// chatRunStatusTransitions 编排 run 状态机合法迁移表(设计 §10):
// pending 仅出 running;interrupted 可回 running(续跑);三终态无出路
var chatRunStatusTransitions = map[string]map[string]bool{
	model.ChatRunStatusPending:     {model.ChatRunStatusRunning: true},
	model.ChatRunStatusRunning:     {model.ChatRunStatusCompleted: true, model.ChatRunStatusFailed: true, model.ChatRunStatusInterrupted: true, model.ChatRunStatusCancelled: true},
	model.ChatRunStatusInterrupted: {model.ChatRunStatusRunning: true},
	model.ChatRunStatusCompleted:   {},
	model.ChatRunStatusFailed:      {},
	model.ChatRunStatusCancelled:   {},
}

// CreateRun 写入编排 run 初始行(status=pending)
func (d *ChatDao) CreateRun(ctx context.Context, run *model.ChatRun) errors.Error {
	if err := GetDB().WithContext(ctx).Create(run).Error; err != nil {
		return errors.ErrorServerInternalError("dao.chat.create_run")
	}
	return nil
}

// UpdateRunStatus 按状态机校验并迁移 run 状态;合法则落库并回填 run.Status;
// 同状态重复更新为幂等 no-op;非法迁移返回 ErrorInvalidRequest 且不落库
func (d *ChatDao) UpdateRunStatus(ctx context.Context, run *model.ChatRun, status string) errors.Error {
	if run.Status == status {
		return nil
	}
	if !chatRunStatusTransitions[run.Status][status] {
		return errors.ErrorInvalidRequest("dao.chat.update_run_status")
	}
	if err := GetDB().WithContext(ctx).Model(&model.ChatRun{}).
		Where("id = ?", run.ID).
		Update("status", status).Error; err != nil {
		return errors.ErrorServerInternalError("dao.chat.update_run_status")
	}
	run.Status = status
	return nil
}

// TakeRunByUUID 按对外 UUID 查询 run,不存在返回 ErrorTypeRecordNotFound
func (d *ChatDao) TakeRunByUUID(ctx context.Context, uid uuid.UUID) (*model.ChatRun, errors.Error) {
	var run model.ChatRun
	if err := GetDB().WithContext(ctx).
		Where("uuid = ?", uid.String()).
		First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound("dao.chat.take_run_by_uuid")
		}
		return nil, errors.ErrorServerInternalError("dao.chat.take_run_by_uuid")
	}
	return &run, nil
}

// SaveFinalReplyOnce 幂等落库最终回复消息:以 (run_id, reply_id) 为幂等键。
// 先查已有行(重试投递直接返回,不比较内容);并发窗口内的唯一冲突经复查收敛,
// 不依赖 gorm TranslateError(跨 PG/MySQL/SQLite 行为一致)
func (d *ChatDao) SaveFinalReplyOnce(ctx context.Context, runUUID uuid.UUID, replyID string, message *model.ChatMessage) (*model.ChatMessage, errors.Error) {
	code := "dao.chat.save_final_reply_once"
	var existing model.ChatMessage
	err := GetDB().WithContext(ctx).
		Where("run_id = ? AND reply_id = ?", runUUID.String(), replyID).
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, errors.ErrorServerInternalError(code)
	}

	var run model.ChatRun
	if err := GetDB().WithContext(ctx).
		Where("uuid = ?", runUUID.String()).
		First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.ErrorRecordNotFound(code)
		}
		return nil, errors.ErrorServerInternalError(code)
	}

	message.RunID = &run.UUID
	message.ReplyID = &replyID
	if err := GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return tx.Model(&model.ChatSession{}).
			Where("id = ?", message.SessionID).
			Update("updated_at", time.Now()).Error
	}); err != nil {
		// 并发下另一请求已落同一 (run_id, reply_id) → 返回已有行
		var winner model.ChatMessage
		if qerr := GetDB().WithContext(ctx).
			Where("run_id = ? AND reply_id = ?", runUUID.String(), replyID).
			First(&winner).Error; qerr == nil {
			return &winner, nil
		}
		return nil, errors.ErrorServerInternalError(code)
	}
	return message, nil
}
