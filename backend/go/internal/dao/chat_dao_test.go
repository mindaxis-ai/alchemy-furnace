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

// newChatDAOTestGroupDB 群聊原子创建测试库(含 session_members 表)
func newChatDAOTestGroupDB(t *testing.T) *ChatDao {
	t.Helper()
	db := newSQLiteTestDB(t, filepath.Join(t.TempDir(), "chat-group-dao.db"))
	if err := db.AutoMigrate(&model.DaoAgent{}, &model.ChatSession{}, &model.SessionMember{}); err != nil {
		t.Fatalf("AutoMigrate group models: %v", err)
	}
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	return NewChatDao()
}

func TestChatDaoSaveGroupSessionRollsBackSessionWhenMemberInsertFails(t *testing.T) {
	dao := newChatDAOTestGroupDB(t)
	// 触发器强制成员写入失败,验证会话与成员整体回滚
	trigger := `
CREATE TRIGGER fail_session_member_insert
BEFORE INSERT ON session_members
BEGIN
  SELECT RAISE(ABORT, 'forced member insert failure');
END`
	if err := DB.Exec(trigger).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	session := &model.ChatSession{UUID: uuid.New(), Type: model.SessionTypeGroup, Title: ""}
	members := []*model.SessionMember{
		{AgentID: 1, SortOrder: 0},
		{AgentID: 2, SortOrder: 1},
	}
	err := dao.SaveGroupSession(context.Background(), session, members)
	if err == nil || err.GetCode() != "dao.chat.save_group_session" {
		t.Fatalf("SaveGroupSession error = %#v, want safe dao.chat.save_group_session", err)
	}

	var sessionCount, memberCount int64
	if queryErr := DB.Model(&model.ChatSession{}).Count(&sessionCount).Error; queryErr != nil {
		t.Fatalf("count sessions: %v", queryErr)
	}
	if queryErr := DB.Model(&model.SessionMember{}).Count(&memberCount).Error; queryErr != nil {
		t.Fatalf("count members: %v", queryErr)
	}
	if sessionCount != 0 || memberCount != 0 {
		t.Fatalf("persisted sessions=%d members=%d, want both 0 after member failure", sessionCount, memberCount)
	}
}

func TestChatDaoSaveGroupSessionPersistsSessionAndMembersInOrder(t *testing.T) {
	dao := newChatDAOTestGroupDB(t)
	agents := []*model.DaoAgent{
		{UUID: uuid.New(), Name: "太上老君", Status: "active", ModelName: "test-model"},
		{UUID: uuid.New(), Name: "孙悟空", Status: "active", ModelName: "test-model"},
	}
	for _, agent := range agents {
		if err := DB.Create(agent).Error; err != nil {
			t.Fatalf("create agent: %v", err)
		}
	}

	session := &model.ChatSession{UUID: uuid.New(), Type: model.SessionTypeGroup, Title: ""}
	members := []*model.SessionMember{
		{AgentID: agents[0].ID, SortOrder: 0},
		{AgentID: agents[1].ID, SortOrder: 1},
	}
	if err := dao.SaveGroupSession(context.Background(), session, members); err != nil {
		t.Fatalf("SaveGroupSession error = %v", err)
	}

	if session.ID == 0 {
		t.Fatal("SaveGroupSession did not assign session ID")
	}
	for i, member := range members {
		if member.SessionID != session.ID {
			t.Fatalf("members[%d].SessionID = %d, want session FK %d", i, member.SessionID, session.ID)
		}
	}

	var persisted model.ChatSession
	if err := DB.Where("uuid = ?", session.UUID.String()).First(&persisted).Error; err != nil {
		t.Fatalf("session not persisted: %v", err)
	}
	var persistedMembers []*model.SessionMember
	if err := DB.Where("session_id = ?", session.ID).
		Order("sort_order ASC, id ASC").
		Find(&persistedMembers).Error; err != nil {
		t.Fatalf("query members: %v", err)
	}
	if len(persistedMembers) != 2 {
		t.Fatalf("persisted members = %d, want 2", len(persistedMembers))
	}
	if persistedMembers[0].AgentID != agents[0].ID || persistedMembers[1].AgentID != agents[1].ID {
		t.Fatalf("member order/association wrong: %+v", persistedMembers)
	}
}
