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

	session := &model.ChatSession{ChatSessionID: uuid.New().String(), Type: model.SessionTypeGroup, Title: "transaction test"}
	if err := db.Create(session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	return NewChatDao(), session
}

// mustParseUUID 测试内把主键 uuid 文本转回 uuid.UUID(Take*ByUUID 仍以 uuid.UUID 为入参)
func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	uid, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return uid
}

func TestChatDaoSaveMessageRollsBackInsertWhenSessionTouchFails(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	trigger := fmt.Sprintf(`
CREATE TRIGGER fail_chat_session_touch
BEFORE UPDATE OF updated_at ON chat_sessions
WHEN OLD.chat_session_id = '%s'
BEGIN
  SELECT RAISE(ABORT, 'forced session touch failure');
END`, session.ChatSessionID)
	if err := DB.Exec(trigger).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	message := &model.ChatMessage{
		ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "user", Content: "must roll back",
	}
	err := dao.SaveMessage(context.Background(), message)
	if err == nil || err.GetCode() != "dao.chat.save_message_touch" {
		t.Fatalf("SaveMessage error = %#v, want safe touch failure", err)
	}

	var count int64
	if queryErr := DB.Model(&model.ChatMessage{}).
		Where("session_id = ? AND content = ?", session.ChatSessionID, message.Content).
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
		Where("chat_session_id = ?", session.ChatSessionID).
		UpdateColumn("updated_at", oldUpdatedAt).Error; err != nil {
		t.Fatalf("backdate session: %v", err)
	}

	message := &model.ChatMessage{
		ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "user", Content: "persist atomically",
	}
	if err := dao.SaveMessage(context.Background(), message); err != nil {
		t.Fatalf("SaveMessage error = %v", err)
	}

	var count int64
	if err := DB.Model(&model.ChatMessage{}).
		Where("session_id = ? AND content = ?", session.ChatSessionID, message.Content).
		Count(&count).Error; err != nil {
		t.Fatalf("count persisted messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted messages = %d, want 1", count)
	}
	var storedSession model.ChatSession
	if err := DB.Where("chat_session_id = ?", session.ChatSessionID).First(&storedSession).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if !storedSession.UpdatedAt.After(oldUpdatedAt) {
		t.Fatalf("updated_at = %s, want after %s", storedSession.UpdatedAt, oldUpdatedAt)
	}
}

func TestChatDaoFindMessagesPagesBackwardFromNewestAndPresentsAscending(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	// 每条消息错开 1 秒:主键(uuid 文本)不再随插入递增,同时间戳的次序无业务含义,
	// 测试断言以 created_at 为权威序
	base := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	messages := make([]model.ChatMessage, 0, 25)
	for i := 1; i <= 25; i++ {
		messages = append(messages, model.ChatMessage{
			ChatMessageID: uuid.New().String(),
			SessionID:     session.ChatSessionID,
			Role:          "assistant",
			Content:       fmt.Sprintf("message-%02d", i),
			CreatedAt:     base.Add(time.Duration(i) * time.Second),
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
			total, got, err := dao.FindMessages(context.Background(), session.ChatSessionID, tt.page, 10)
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
		{DaoAgentID: uuid.New().String(), Name: "太上老君", Status: "active", ModelName: "test-model"},
		{DaoAgentID: uuid.New().String(), Name: "孙悟空", Status: "active", ModelName: "test-model"},
	}
	for _, agent := range agents {
		if err := DB.Create(agent).Error; err != nil {
			t.Fatalf("create agent: %v", err)
		}
	}
	sessions := []*model.ChatSession{
		{ChatSessionID: uuid.New().String(), Type: model.SessionTypeGroup},
		{ChatSessionID: uuid.New().String(), Type: model.SessionTypeGroup},
	}
	for _, session := range sessions {
		if err := DB.Create(session).Error; err != nil {
			t.Fatalf("create session: %v", err)
		}
	}
	// 故意逆序写入,证明结果按 sort_order 排序而非插入顺序
	seed := []*model.SessionMember{
		{SessionID: sessions[0].ChatSessionID, AgentID: agents[1].DaoAgentID, SortOrder: 1},
		{SessionID: sessions[0].ChatSessionID, AgentID: agents[0].DaoAgentID, SortOrder: 0},
		{SessionID: sessions[1].ChatSessionID, AgentID: agents[1].DaoAgentID, SortOrder: 0},
	}
	for _, member := range seed {
		if err := DB.Create(member).Error; err != nil {
			t.Fatalf("create member: %v", err)
		}
	}

	grouped, err := dao.FindMembersBySessionIDs(context.Background(), []string{sessions[0].ChatSessionID, sessions[1].ChatSessionID})
	if err != nil {
		t.Fatalf("FindMembersBySessionIDs error = %v", err)
	}
	if len(grouped) != 2 {
		t.Fatalf("grouped sessions = %d, want 2", len(grouped))
	}
	first := grouped[sessions[0].ChatSessionID]
	if len(first) != 2 || first[0].AgentID != agents[0].DaoAgentID || first[1].AgentID != agents[1].DaoAgentID {
		t.Fatalf("session[0] members = %+v, want sorted by sort_order", first)
	}
	if first[0].Agent.Name != "太上老君" {
		t.Fatalf("Agent 未预加载: %+v", first[0].Agent)
	}
	second := grouped[sessions[1].ChatSessionID]
	if len(second) != 1 || second[0].AgentID != agents[1].DaoAgentID {
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

	session := &model.ChatSession{ChatSessionID: uuid.New().String(), Type: model.SessionTypeGroup, Title: ""}
	members := []*model.SessionMember{
		{AgentID: uuid.NewString(), SortOrder: 0},
		{AgentID: uuid.NewString(), SortOrder: 1},
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
		{DaoAgentID: uuid.New().String(), Name: "太上老君", Status: "active", ModelName: "test-model"},
		{DaoAgentID: uuid.New().String(), Name: "孙悟空", Status: "active", ModelName: "test-model"},
	}
	for _, agent := range agents {
		if err := DB.Create(agent).Error; err != nil {
			t.Fatalf("create agent: %v", err)
		}
	}

	session := &model.ChatSession{ChatSessionID: uuid.New().String(), Type: model.SessionTypeGroup, Title: ""}
	members := []*model.SessionMember{
		{AgentID: agents[0].DaoAgentID, SortOrder: 0},
		{AgentID: agents[1].DaoAgentID, SortOrder: 1},
	}
	if err := dao.SaveGroupSession(context.Background(), session, members); err != nil {
		t.Fatalf("SaveGroupSession error = %v", err)
	}

	if session.ChatSessionID == "" {
		t.Fatal("SaveGroupSession did not assign session ID")
	}
	for i, member := range members {
		if member.SessionID != session.ChatSessionID {
			t.Fatalf("members[%d].SessionID = %s, want session UUID %s", i, member.SessionID, session.ChatSessionID)
		}
	}

	var persisted model.ChatSession
	if err := DB.Where("chat_session_id = ?", session.ChatSessionID).First(&persisted).Error; err != nil {
		t.Fatalf("session not persisted: %v", err)
	}
	var persistedMembers []*model.SessionMember
	if err := DB.Where("session_id = ?", session.ChatSessionID).
		Order("sort_order ASC, session_member_id ASC").
		Find(&persistedMembers).Error; err != nil {
		t.Fatalf("query members: %v", err)
	}
	if len(persistedMembers) != 2 {
		t.Fatalf("persisted members = %d, want 2", len(persistedMembers))
	}
	if persistedMembers[0].AgentID != agents[0].DaoAgentID || persistedMembers[1].AgentID != agents[1].DaoAgentID {
		t.Fatalf("member order/association wrong: %+v", persistedMembers)
	}
}

// ---- 编排 run 持久化（Task 9）----

func TestChatRunLifecyclePersistsAndTransitionsLegally(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	ctx := context.Background()

	run := &model.ChatRun{
		ChatRunID: uuid.New().String(),
		SessionID: session.ChatSessionID,
		Status:    model.ChatRunStatusPending,
		Engine:    "langgraph",
	}
	if err := dao.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}
	if run.ChatRunID == "" {
		t.Fatal("CreateRun did not assign run ID")
	}

	taken, err := dao.TakeRunByUUID(ctx, mustParseUUID(t, run.ChatRunID))
	if err != nil {
		t.Fatalf("TakeRunByUUID error = %v", err)
	}
	if taken.ChatRunID != run.ChatRunID || taken.SessionID != session.ChatSessionID {
		t.Fatalf("TakeRunByUUID identity = %+v, want run %s session %s", taken, run.ChatRunID, session.ChatSessionID)
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
	if err := DB.Where("chat_run_id = ?", run.ChatRunID).First(&stored).Error; err != nil {
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
			ChatRunID: uuid.New().String(),
			SessionID: session.ChatSessionID,
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
	if err := DB.Where("chat_run_id = ?", pending.ChatRunID).First(&stored).Error; err != nil {
		t.Fatalf("reload rejected run: %v", err)
	}
	if stored.Status != model.ChatRunStatusPending {
		t.Fatalf("rejected transition persisted status = %s, want pending", stored.Status)
	}
}

func TestSaveFinalReplyOnceIsIdempotent(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	ctx := context.Background()

	run := &model.ChatRun{ChatRunID: uuid.New().String(), SessionID: session.ChatSessionID, Status: model.ChatRunStatusRunning, Engine: "langgraph"}
	if err := dao.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error = %v", err)
	}

	message := &model.ChatMessage{
		ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "assistant", Content: "第一份终稿",
	}
	first, err := dao.SaveFinalReplyOnce(ctx, mustParseUUID(t, run.ChatRunID), "reply-1", message)
	if err != nil {
		t.Fatalf("SaveFinalReplyOnce error = %v", err)
	}
	if first.ChatMessageID == "" || first.RunID == nil || *first.RunID != run.ChatRunID || first.ReplyID == nil || *first.ReplyID != "reply-1" {
		t.Fatalf("first reply identity = %+v, want backfilled run/reply ids", first)
	}

	// 重复投递（重试）：不同内容也必须返回已有行，不产生第二条
	duplicate := &model.ChatMessage{
		ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "assistant", Content: "重试的第二份终稿",
	}
	second, err := dao.SaveFinalReplyOnce(ctx, mustParseUUID(t, run.ChatRunID), "reply-1", duplicate)
	if err != nil {
		t.Fatalf("duplicate SaveFinalReplyOnce error = %v", err)
	}
	if first.ChatMessageID != second.ChatMessageID {
		t.Fatalf("duplicate returned ID = %s, want original %s", second.ChatMessageID, first.ChatMessageID)
	}
	if second.Content != "第一份终稿" {
		t.Fatalf("duplicate returned content = %q, want original %q", second.Content, "第一份终稿")
	}

	var count int64
	if err := DB.Model(&model.ChatMessage{}).Where("session_id = ?", session.ChatSessionID).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted messages = %d, want 1 after duplicate delivery", count)
	}
}

func TestSaveFinalReplyOnceUniqueKeyIsRunAndReply(t *testing.T) {
	dao, session := newChatDAOTestSession(t)
	ctx := context.Background()

	runA := &model.ChatRun{ChatRunID: uuid.New().String(), SessionID: session.ChatSessionID, Status: model.ChatRunStatusRunning, Engine: "langgraph"}
	runB := &model.ChatRun{ChatRunID: uuid.New().String(), SessionID: session.ChatSessionID, Status: model.ChatRunStatusRunning, Engine: "langgraph"}
	for _, run := range []*model.ChatRun{runA, runB} {
		if err := dao.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun(%s) error = %v", run.ChatRunID, err)
		}
	}

	// 同 run 不同 reply -> 两行
	first, err := dao.SaveFinalReplyOnce(ctx, mustParseUUID(t, runA.ChatRunID), "reply-1", &model.ChatMessage{ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "assistant", Content: "runA-reply1"})
	if err != nil {
		t.Fatalf("SaveFinalReplyOnce runA/reply-1 error = %v", err)
	}
	second, err := dao.SaveFinalReplyOnce(ctx, mustParseUUID(t, runA.ChatRunID), "reply-2", &model.ChatMessage{ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "assistant", Content: "runA-reply2"})
	if err != nil {
		t.Fatalf("SaveFinalReplyOnce runA/reply-2 error = %v", err)
	}
	if first.ChatMessageID == second.ChatMessageID {
		t.Fatalf("distinct reply ids collided on message ID %s, want two rows", first.ChatMessageID)
	}

	// 不同 run 同 reply -> 两行（唯一键是 (run_id, reply_id) 组合，不是 reply_id 单列）
	third, err := dao.SaveFinalReplyOnce(ctx, mustParseUUID(t, runB.ChatRunID), "reply-1", &model.ChatMessage{ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "assistant", Content: "runB-reply1"})
	if err != nil {
		t.Fatalf("SaveFinalReplyOnce runB/reply-1 error = %v", err)
	}
	if third.ChatMessageID == first.ChatMessageID || third.ChatMessageID == second.ChatMessageID {
		t.Fatalf("different run same reply collided on message ID %s, want a third row", third.ChatMessageID)
	}

	var count int64
	if err := DB.Model(&model.ChatMessage{}).Where("session_id = ?", session.ChatSessionID).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 3 {
		t.Fatalf("persisted messages = %d, want 3", count)
	}

	// 未知 run -> 稳定错误码
	_, err = dao.SaveFinalReplyOnce(ctx, uuid.New(), "reply-x", &model.ChatMessage{ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "assistant", Content: "orphan"})
	if err == nil || err.GetCode() != "dao.chat.save_final_reply_once" {
		t.Fatalf("unknown run error = %#v, want dao.chat.save_final_reply_once", err)
	}
}

// ---- 续跑:run.SessionID 存会话 UUID 文本(011 业务键),经 TakeSessionByUUID 定位会话 ----

// TestChatDaoTakeSessionByUUIDPreloadsAgentAndReportsMissing 供续跑入口以
// run.SessionID(UUID 文本)定位会话(预加载道人);未知 UUID 返回
// take_session_by_uuid 记录不存在。
func TestChatDaoTakeSessionByUUIDPreloadsAgentAndReportsMissing(t *testing.T) {
	dao, groupSession := newChatDAOTestSession(t)
	ctx := context.Background()

	got, err := dao.TakeSessionByUUID(ctx, mustParseUUID(t, groupSession.ChatSessionID))
	if err != nil {
		t.Fatalf("TakeSessionByUUID error = %v", err)
	}
	if got.ChatSessionID != groupSession.ChatSessionID || got.Title != "transaction test" {
		t.Fatalf("TakeSessionByUUID = %+v, want session %s(transaction test)", got, groupSession.ChatSessionID)
	}

	agent := &model.DaoAgent{DaoAgentID: uuid.New().String(), Name: "单聊道人", Status: "active", ModelName: "m"}
	if err := DB.Create(agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agentUUID := agent.DaoAgentID
	single := &model.ChatSession{ChatSessionID: uuid.New().String(), Type: model.SessionTypeSingle, AgentID: &agentUUID}
	if err := DB.Create(single).Error; err != nil {
		t.Fatalf("create single session: %v", err)
	}
	preloaded, err := dao.TakeSessionByUUID(ctx, mustParseUUID(t, single.ChatSessionID))
	if err != nil {
		t.Fatalf("TakeSessionByUUID(single) error = %v", err)
	}
	if preloaded.Agent.Name != "单聊道人" {
		t.Fatalf("preloaded Agent = %+v, want 单聊道人", preloaded.Agent)
	}

	if _, err := dao.TakeSessionByUUID(ctx, uuid.New()); err == nil || err.GetCode() != "dao.chat.take_session_by_uuid" {
		t.Fatalf("missing error = %#v, want dao.chat.take_session_by_uuid", err)
	}
}
