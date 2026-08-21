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

// FindMessages 分页查询会话消息(按时间正序)
func (d *ChatDao) FindMessages(ctx context.Context, sessionID uint, page int, size int) (int64, []*model.ChatMessage, errors.Error) {
	db := GetDB().WithContext(ctx).Model(&model.ChatMessage{}).Where("session_id = ?", sessionID)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return 0, nil, errors.ErrorServerInternalError("dao.chat.find_messages_count")
	}
	if total == 0 || size <= 0 || int64((page-1)*size) >= total {
		return total, nil, nil
	}

	var messages []*model.ChatMessage
	if err := db.Preload("Agent").
		Order("created_at ASC").
		Offset((page - 1) * size).Limit(size).
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
