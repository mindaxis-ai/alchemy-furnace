package dao

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

func newChatDAOTestSession(t *testing.T) (*ChatDao, *model.ChatSession) {
	t.Helper()
	db := newSQLiteTestDB(t, filepath.Join(t.TempDir(), "chat-dao.db"))
	if err := db.AutoMigrate(&model.DaoAgent{}, &model.ChatSession{}, &model.ChatMessage{}); err != nil {
		t.Fatalf("AutoMigrate chat models: %v", err)
	}
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	session := &model.ChatSession{UUID: uuid.New(), Type: model.SessionTypeGroup, Title: "transaction test"}
	if err := db.Create(session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	return NewChatDao(), session
}

func TestChatDaoSaveMessageRollsBackInsertWhenSessionTouchFails(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	trigger := fmt.Sprintf(`
CREATE TRIGGER fail_chat_session_touch
BEFORE UPDATE OF updated_at ON chat_sessions
WHEN OLD.id = %d
BEGIN
  SELECT RAISE(ABORT, 'forced session touch failure');
END`, session.ID)
	if err := DB.Exec(trigger).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	message := &model.ChatMessage{
		UUID: uuid.New(), SessionID: session.ID, Role: "user", Content: "must roll back",
	}
	err := dao.SaveMessage(context.Background(), message)
	if err == nil || err.GetCode() != "dao.chat.save_message_touch" {
		t.Fatalf("SaveMessage error = %#v, want safe touch failure", err)
	}

	var count int64
	if queryErr := DB.Model(&model.ChatMessage{}).
		Where("session_id = ? AND content = ?", session.ID, message.Content).
		Count(&count).Error; queryErr != nil {
		t.Fatalf("count rolled-back messages: %v", queryErr)
	}
	if count != 0 {
		t.Fatalf("persisted messages = %d, want 0 when session touch fails", count)
	}
}

func TestChatDaoSaveMessagePersistsAndTouchesSession(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	oldUpdatedAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	if err := DB.Model(&model.ChatSession{}).
		Where("id = ?", session.ID).
		UpdateColumn("updated_at", oldUpdatedAt).Error; err != nil {
		t.Fatalf("backdate session: %v", err)
	}

	message := &model.ChatMessage{
		UUID: uuid.New(), SessionID: session.ID, Role: "user", Content: "persist atomically",
	}
	if err := dao.SaveMessage(context.Background(), message); err != nil {
		t.Fatalf("SaveMessage error = %v", err)
	}

	var count int64
	if err := DB.Model(&model.ChatMessage{}).
		Where("session_id = ? AND content = ?", session.ID, message.Content).
		Count(&count).Error; err != nil {
		t.Fatalf("count persisted messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted messages = %d, want 1", count)
	}
	var storedSession model.ChatSession
	if err := DB.First(&storedSession, session.ID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if !storedSession.UpdatedAt.After(oldUpdatedAt) {
		t.Fatalf("updated_at = %s, want after %s", storedSession.UpdatedAt, oldUpdatedAt)
	}
}
