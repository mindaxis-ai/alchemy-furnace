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
	if err := db.AutoMigrate(&model.DaoAgent{}, &model.ChatSession{}, &model.ChatMessage{}, &model.ChatRun{}); err != nil {
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

func TestChatDaoFindMembersBySessionIDsGroupsOrdersAndSkipsEmptyInput(t *testing.T) {
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
	sessions := []*model.ChatSession{
		{UUID: uuid.New(), Type: model.SessionTypeGroup},
		{UUID: uuid.New(), Type: model.SessionTypeGroup},
	}
	for _, session := range sessions {
		if err := DB.Create(session).Error; err != nil {
			t.Fatalf("create session: %v", err)
		}
	}
	// 故意逆序写入,证明结果按 sort_order 排序而非插入顺序
	seed := []*model.SessionMember{
		{SessionID: sessions[0].ID, AgentID: agents[1].ID, SortOrder: 1},
		{SessionID: sessions[0].ID, AgentID: agents[0].ID, SortOrder: 0},
		{SessionID: sessions[1].ID, AgentID: agents[1].ID, SortOrder: 0},
	}
	for _, member := range seed {
		if err := DB.Create(member).Error; err != nil {
			t.Fatalf("create member: %v", err)
		}
	}

	grouped, err := dao.FindMembersBySessionIDs(context.Background(), []uint{sessions[0].ID, sessions[1].ID})
	if err != nil {
		t.Fatalf("FindMembersBySessionIDs error = %v", err)
	}
	if len(grouped) != 2 {
		t.Fatalf("grouped sessions = %d, want 2", len(grouped))
	}
	first := grouped[sessions[0].ID]
	if len(first) != 2 || first[0].AgentID != agents[0].ID || first[1].AgentID != agents[1].ID {
		t.Fatalf("session[0] members = %+v, want sorted by sort_order", first)
	}
	if first[0].Agent.Name != "太上老君" {
		t.Fatalf("Agent 未预加载: %+v", first[0].Agent)
	}
	second := grouped[sessions[1].ID]
	if len(second) != 1 || second[0].AgentID != agents[1].ID {
		t.Fatalf("session[1] members = %+v, want single member", second)
	}

	// 空输入直接返回空 map,不访问数据库
	empty, err := dao.FindMembersBySessionIDs(context.Background(), nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty input = %v, %v; want empty map, nil error", empty, err)
	}
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

// ---- 编排 run 持久化（Task 9）----

func TestChatRunLifecyclePersistsAndTransitionsLegally(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	ctx := context.Background()

	run := &model.ChatRun{
		UUID:      uuid.New(),
		SessionID: session.ID,
		Status:    model.ChatRunStatusPending,
		Engine:    "langgraph",
	}
	if err := dao.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	if run.ID == 0 {
		t.Fatal("CreateRun did not assign run ID")
	}

	taken, err := dao.TakeRunByUUID(ctx, run.UUID)
	if err != nil {
		t.Fatalf("TakeRunByUUID error = %v", err)
	}
	if taken.UUID != run.UUID || taken.SessionID != session.ID {
		t.Fatalf("TakeRunByUUID identity = %+v, want run %s session %d", taken, run.UUID, session.ID)
	}
	if taken.Status != model.ChatRunStatusPending || taken.Engine != "langgraph" {
		t.Fatalf("TakeRunByUUID status/engine = %s/%s, want pending/langgraph", taken.Status, taken.Engine)
	}

	// 合法链：pending -> running -> interrupted -> running -> completed
	for _, status := range []string{
		model.ChatRunStatusRunning,
		model.ChatRunStatusInterrupted,
		model.ChatRunStatusRunning,
		model.ChatRunStatusCompleted,
	} {
		if err := dao.UpdateRunStatus(ctx, taken, status); err != nil {
			t.Fatalf("UpdateRunStatus(%s) error = %v", status, err)
		}
		if taken.Status != status {
			t.Fatalf("UpdateRunStatus did not backfill run.Status = %s, want %s", taken.Status, status)
		}
	}

	// 同状态重复更新 = 幂等 no-op
	if err := dao.UpdateRunStatus(ctx, taken, model.ChatRunStatusCompleted); err != nil {
		t.Fatalf("idempotent UpdateRunStatus(completed) error = %v", err)
	}
	if taken.Status != model.ChatRunStatusCompleted {
		t.Fatalf("idempotent update changed status to %s, want completed", taken.Status)
	}

	var stored model.ChatRun
	if err := DB.Where("uuid = ?", run.UUID.String()).First(&stored).Error; err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if stored.Status != model.ChatRunStatusCompleted {
		t.Fatalf("stored status = %s, want completed", stored.Status)
	}
}

func TestChatRunStatusRejectsIllegalTransitions(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	ctx := context.Background()

	newRun := func() *model.ChatRun {
		run := &model.ChatRun{
			UUID:      uuid.New(),
			SessionID: session.ID,
			Status:    model.ChatRunStatusPending,
			Engine:    "langgraph",
		}
		if err := dao.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun error = %v", err)
		}
		return run
	}

	// pending 只能出 running；直接 completed 拒绝
	pending := newRun()
	err := dao.UpdateRunStatus(ctx, pending, model.ChatRunStatusCompleted)
	if err == nil || err.GetCode() != "dao.chat.update_run_status" {
		t.Fatalf("pending->completed error = %#v, want dao.chat.update_run_status", err)
	}

	// running 可出三终态；终态无出路（先 pending->running，状态机不允许 pending 直达终态）
	for _, terminal := range []string{model.ChatRunStatusCompleted, model.ChatRunStatusFailed, model.ChatRunStatusCancelled} {
		run := newRun()
		if err := dao.UpdateRunStatus(ctx, run, model.ChatRunStatusRunning); err != nil {
			t.Fatalf("UpdateRunStatus(running) error = %v", err)
		}
		if err := dao.UpdateRunStatus(ctx, run, terminal); err != nil {
			t.Fatalf("UpdateRunStatus(%s) error = %v", terminal, err)
		}
		err := dao.UpdateRunStatus(ctx, run, model.ChatRunStatusRunning)
		if err == nil || err.GetCode() != "dao.chat.update_run_status" {
			t.Fatalf("%s->running error = %#v, want dao.chat.update_run_status", terminal, err)
		}
	}

	// 拒绝必须落库为未变更：pending->completed 被拒后 DB 状态仍是 pending
	var stored model.ChatRun
	if err := DB.Where("uuid = ?", pending.UUID.String()).First(&stored).Error; err != nil {
		t.Fatalf("reload rejected run: %v", err)
	}
	if stored.Status != model.ChatRunStatusPending {
		t.Fatalf("rejected transition persisted status = %s, want pending", stored.Status)
	}
}

func TestSaveFinalReplyOnceIsIdempotent(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	ctx := context.Background()

	run := &model.ChatRun{UUID: uuid.New(), SessionID: session.ID, Status: model.ChatRunStatusRunning, Engine: "langgraph"}
	if err := dao.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	message := &model.ChatMessage{
		UUID: uuid.New(), SessionID: session.ID, Role: "assistant", Content: "第一份终稿",
	}
	first, err := dao.SaveFinalReplyOnce(ctx, run.UUID, "reply-1", message)
	if err != nil {
		t.Fatalf("SaveFinalReplyOnce error = %v", err)
	}
	if first.ID == 0 || first.RunID == nil || *first.RunID != run.UUID || first.ReplyID == nil || *first.ReplyID != "reply-1" {
		t.Fatalf("first reply identity = %+v, want backfilled run/reply ids", first)
	}

	// 重复投递（重试）：不同内容也必须返回已有行，不产生第二条
	duplicate := &model.ChatMessage{
		UUID: uuid.New(), SessionID: session.ID, Role: "assistant", Content: "重试的第二份终稿",
	}
	second, err := dao.SaveFinalReplyOnce(ctx, run.UUID, "reply-1", duplicate)
	if err != nil {
		t.Fatalf("duplicate SaveFinalReplyOnce error = %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate returned ID = %d, want original %d", second.ID, first.ID)
	}
	if second.Content != "第一份终稿" {
		t.Fatalf("duplicate returned content = %q, want original %q", second.Content, "第一份终稿")
	}

	var count int64
	if err := DB.Model(&model.ChatMessage{}).Where("session_id = ?", session.ID).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted messages = %d, want 1 after duplicate delivery", count)
	}
}

func TestSaveFinalReplyOnceUniqueKeyIsRunAndReply(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	ctx := context.Background()

	runA := &model.ChatRun{UUID: uuid.New(), SessionID: session.ID, Status: model.ChatRunStatusRunning, Engine: "langgraph"}
	runB := &model.ChatRun{UUID: uuid.New(), SessionID: session.ID, Status: model.ChatRunStatusRunning, Engine: "langgraph"}
	for _, run := range []*model.ChatRun{runA, runB} {
		if err := dao.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun(%s) error = %v", run.UUID, err)
		}
	}

	// 同 run 不同 reply -> 两行
	first, err := dao.SaveFinalReplyOnce(ctx, runA.UUID, "reply-1", &model.ChatMessage{UUID: uuid.New(), SessionID: session.ID, Role: "assistant", Content: "runA-reply1"})
	if err != nil {
		t.Fatalf("SaveFinalReplyOnce runA/reply-1 error = %v", err)
	}
	second, err := dao.SaveFinalReplyOnce(ctx, runA.UUID, "reply-2", &model.ChatMessage{UUID: uuid.New(), SessionID: session.ID, Role: "assistant", Content: "runA-reply2"})
	if err != nil {
		t.Fatalf("SaveFinalReplyOnce runA/reply-2 error = %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("distinct reply ids collided on message ID %d, want two rows", first.ID)
	}

	// 不同 run 同 reply -> 两行（唯一键是 (run_id, reply_id) 组合，不是 reply_id 单列）
	third, err := dao.SaveFinalReplyOnce(ctx, runB.UUID, "reply-1", &model.ChatMessage{UUID: uuid.New(), SessionID: session.ID, Role: "assistant", Content: "runB-reply1"})
	if err != nil {
		t.Fatalf("SaveFinalReplyOnce runB/reply-1 error = %v", err)
	}
	if third.ID == first.ID || third.ID == second.ID {
		t.Fatalf("different run same reply collided on message ID %d, want a third row", third.ID)
	}

	var count int64
	if err := DB.Model(&model.ChatMessage{}).Where("session_id = ?", session.ID).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 3 {
		t.Fatalf("persisted messages = %d, want 3", count)
	}

	// 未知 run -> 稳定错误码
	_, err = dao.SaveFinalReplyOnce(ctx, uuid.New(), "reply-x", &model.ChatMessage{UUID: uuid.New(), SessionID: session.ID, Role: "assistant", Content: "orphan"})
	if err == nil || err.GetCode() != "dao.chat.save_final_reply_once" {
		t.Fatalf("unknown run error = %#v, want dao.chat.save_final_reply_once", err)
	}
}
