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

func TestChatDaoFindMessagesPagesBackwardFromNewestAndPresentsAscending(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	createdAt := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	messages := make([]model.ChatMessage, 0, 25)
	for i := 1; i <= 25; i++ {
		messages = append(messages, model.ChatMessage{
			UUID:      uuid.New(),
			SessionID: session.ID,
			Role:      "assistant",
			Content:   fmt.Sprintf("message-%02d", i),
			CreatedAt: createdAt,
		})
	}
	if err := DB.Create(&messages).Error; err != nil {
		t.Fatalf("create message history: %v", err)
	}

	tests := []struct {
		page int
		want []string
	}{
		{page: 1, want: []string{"message-16", "message-17", "message-18", "message-19", "message-20", "message-21", "message-22", "message-23", "message-24", "message-25"}},
		{page: 2, want: []string{"message-06", "message-07", "message-08", "message-09", "message-10", "message-11", "message-12", "message-13", "message-14", "message-15"}},
		{page: 3, want: []string{"message-01", "message-02", "message-03", "message-04", "message-05"}},
		{page: 4, want: nil},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("page_%d", tt.page), func(t *testing.T) {
			total, got, err := dao.FindMessages(context.Background(), session.ID, tt.page, 10)
			if err != nil {
				t.Fatalf("FindMessages() error = %v", err)
			}
			if total != 25 {
				t.Fatalf("FindMessages() total = %d, want 25", total)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("FindMessages() len = %d, want %d", len(got), len(tt.want))
			}
			for i, message := range got {
				if message.Content != tt.want[i] {
					t.Fatalf("FindMessages()[%d] = %q, want %q", i, message.Content, tt.want[i])
				}
			}
		})
	}
}
