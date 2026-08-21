package chat_service

import (
	"context"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// ---------- fakes (只实现用到的;其他方法 panic 表示意外调用) ----------

type fakeAgentDao struct{ agents map[string]*model.DaoAgent }

func (f *fakeAgentDao) TakeAgentByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error) {
	if a, ok := f.agents[uid.String()]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, errors.ErrorRecordNotFound("test.fake.take_agent")
}
func (f *fakeAgentDao) TakeAgentDetailByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error) {
	panic("unused")
}
func (f *fakeAgentDao) TakeAgentDetailByID(ctx context.Context, id uint) (*model.DaoAgent, errors.Error) {
	panic("unused")
}
func (f *fakeAgentDao) FindAgents(ctx context.Context, page, size int, status string) (int64, []*model.DaoAgent, errors.Error) {
	panic("unused")
}
func (f *fakeAgentDao) SaveAgent(ctx context.Context, a *model.DaoAgent) errors.Error {
	panic("unused")
}
func (f *fakeAgentDao) UpdateAgent(ctx context.Context, a *model.DaoAgent, updates map[string]any) errors.Error {
	panic("unused")
}
func (f *fakeAgentDao) DeleteAgent(ctx context.Context, a *model.DaoAgent) errors.Error {
	panic("unused")
}
func (f *fakeAgentDao) TakeAgentPill(ctx context.Context, agentID, pillID uint) (*model.AgentPill, errors.Error) {
	panic("unused")
}
func (f *fakeAgentDao) SaveAgentPill(ctx context.Context, ap *model.AgentPill) errors.Error {
	panic("unused")
}
func (f *fakeAgentDao) UpdateAgentPill(ctx context.Context, ap *model.AgentPill, updates map[string]any) errors.Error {
	panic("unused")
}
func (f *fakeAgentDao) DeleteAgentPill(ctx context.Context, agentID, pillID uint) (int64, errors.Error) {
	panic("unused")
}
func (f *fakeAgentDao) MaxAgentPillSortOrder(ctx context.Context, agentID uint) (int, errors.Error) {
	panic("unused")
}
func (f *fakeAgentDao) FindPillsByAgentID(ctx context.Context, agentID uint) ([]*model.ElixirPill, errors.Error) {
	panic("unused")
}
func (f *fakeAgentDao) InvalidateLanguagePattern(ctx context.Context, agentID uint) errors.Error {
	panic("unused")
}
func (f *fakeAgentDao) SaveLanguagePattern(ctx context.Context, p *model.LanguagePattern) errors.Error {
	panic("unused")
}

type fakeChatDao struct {
	sessions  map[string]*model.ChatSession
	members   map[uint][]*model.SessionMember
	messages  []*model.ChatMessage
	nextID    uint
	agentByID map[uint]*model.DaoAgent // 模拟 GORM Preload("Agent")
	saveErr   errors.Error
}

func (f *fakeChatDao) TakeSessionByUUID(ctx context.Context, uid uuid.UUID) (*model.ChatSession, errors.Error) {
	if s, ok := f.sessions[uid.String()]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, errors.ErrorRecordNotFound("test.fake.take_session")
}
func (f *fakeChatDao) FindSessions(ctx context.Context, agentID uint, page, size int) (int64, []*model.ChatSession, errors.Error) {
	out := make([]*model.ChatSession, 0, len(f.sessions))
	for _, session := range f.sessions {
		if agentID != 0 && (session.AgentID == nil || *session.AgentID != agentID) {
			continue
		}
		copy := *session
		out = append(out, &copy)
	}
	return int64(len(out)), out, nil
}
func (f *fakeChatDao) SaveSession(ctx context.Context, s *model.ChatSession) errors.Error {
	f.nextID++
	s.ID = f.nextID
	if s.UUID == (uuid.UUID{}) {
		s.UUID = uuid.New()
	}
	f.sessions[s.UUID.String()] = s
	return nil
}
func (f *fakeChatDao) UpdateSession(ctx context.Context, s *model.ChatSession, updates map[string]any) errors.Error {
	if stored, ok := f.sessions[s.UUID.String()]; ok {
		for k, v := range updates {
			switch k {
			case "title":
				if str, ok := v.(string); ok {
					stored.Title = str
				}
			}
		}
	}
	return nil
}

func (f *fakeChatDao) FindMessages(ctx context.Context, sessionID uint, page, size int) (int64, []*model.ChatMessage, errors.Error) {
	// 简易实现:按写入顺序返回,fake 不分页(测试单轮历史 < 20 条)
	out := make([]*model.ChatMessage, 0, len(f.messages))
	for _, m := range f.messages {
		if m.SessionID == sessionID {
			cp := *m
			if cp.AgentID != nil {
				if a, ok := f.agentByID[*cp.AgentID]; ok {
					agent := *a
					cp.Agent = &agent
				}
			}
			out = append(out, &cp)
		}
	}
	return int64(len(out)), out, nil
}
func (f *fakeChatDao) TakeLatestUserMessage(ctx context.Context, sessionID uint) (*model.ChatMessage, errors.Error) {
	for i := len(f.messages) - 1; i >= 0; i-- {
		if f.messages[i].SessionID == sessionID && f.messages[i].Role == "user" {
			cp := *f.messages[i]
			return &cp, nil
		}
	}
	return nil, errors.ErrorRecordNotFound("test.fake.latest_user")
}
func (f *fakeChatDao) DeleteSession(ctx context.Context, s *model.ChatSession) errors.Error {
	delete(f.sessions, s.UUID.String())
	return nil
}
func (f *fakeChatDao) SaveMessage(ctx context.Context, msg *model.ChatMessage) errors.Error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.messages = append(f.messages, msg)
	return nil
}
func (f *fakeChatDao) SaveMembers(ctx context.Context, ms []*model.SessionMember) errors.Error {
	for _, m := range ms {
		f.members[m.SessionID] = append(f.members[m.SessionID], m)
	}
	return nil
}
func (f *fakeChatDao) FindMembers(ctx context.Context, sessionID uint) ([]*model.SessionMember, errors.Error) {
	// 填充 Agent(模拟 GORM Preload)
	src := f.members[sessionID]
	out := make([]*model.SessionMember, 0, len(src))
	for _, m := range src {
		cp := *m
		if a, ok := f.agentByID[cp.AgentID]; ok {
			agent := *a
			cp.Agent = agent
		}
		out = append(out, &cp)
	}
	return out, nil
}
func (f *fakeChatDao) DeleteMember(ctx context.Context, sessionID uint, agentID uint) errors.Error {
	src := f.members[sessionID]
	for i, m := range src {
		if m.AgentID == agentID {
			f.members[sessionID] = append(src[:i], src[i+1:]...)
			return nil
		}
	}
	return errors.ErrorRecordNotFound("test.fake.delete_member")
}

func newGroupTestSvc() (*Chat, *fakeChatDao, uuid.UUID, uuid.UUID, uuid.UUID) {
	u1, u2, u3 := uuid.New(), uuid.New(), uuid.New()
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{
		u1.String(): {ID: 1, UUID: u1, Name: "太上老君", Status: "active", ModelName: "test-model"},
		u2.String(): {ID: 2, UUID: u2, Name: "孙悟空", Status: "active", ModelName: "test-model"},
		u3.String(): {ID: 3, UUID: u3, Name: "睡道人", Status: "inactive", ModelName: "test-model"},
	}}
	agentByID := map[uint]*model.DaoAgent{
		1: agents.agents[u1.String()],
		2: agents.agents[u2.String()],
	}
	chats := &fakeChatDao{
		sessions:  map[string]*model.ChatSession{},
		members:   map[uint][]*model.SessionMember{},
		agentByID: agentByID,
	}
	svc := New(chats, agents, nil, availableCredentialResolver("test-model"), "http://unused")
	return svc, chats, u1, u2, u3
}

func TestCreateGroupSession(t *testing.T) {
	svc, chats, u1, u2, u3 := newGroupTestSvc()
	ctx := context.Background()

	if _, err := svc.CreateGroupSession(ctx, []uuid.UUID{u1}); err == nil {
		t.Fatal("成员不足2人应报错")
	}
	if _, err := svc.CreateGroupSession(ctx, []uuid.UUID{u1, u3}); err == nil {
		t.Fatal("含 inactive 成员应报错")
	}
	// 正常建群(重复 uuid 去重)
	s, err := svc.CreateGroupSession(ctx, []uuid.UUID{u1, u2, u1})
	if err != nil {
		t.Fatalf("CreateGroupSession: %v", err)
	}
	if s.Type != model.SessionTypeGroup || s.Title != "" || s.AgentID != nil {
		t.Fatalf("群会话字段不对: %+v", s)
	}
	if len(chats.members[s.ID]) != 2 {
		t.Fatalf("成员未去重: %d", len(chats.members[s.ID]))
	}
	if chats.members[s.ID][0].SortOrder != 0 || chats.members[s.ID][1].SortOrder != 1 {
		t.Fatalf("SortOrder 未按邀请顺序赋值: %+v", chats.members[s.ID])
	}
}

func TestListSessionsLoadsCurrentGroupMembers(t *testing.T) {
	svc, chats, u1, u2, _ := newGroupTestSvc()
	session, err := svc.CreateGroupSession(context.Background(), []uuid.UUID{u1, u2})
	if err != nil {
		t.Fatalf("CreateGroupSession() error = %v", err)
	}
	chats.agentByID[2].Status = "inactive"

	_, sessions, listErr := svc.ListSessions(context.Background(), uuid.Nil, 1, 100)
	if listErr != nil {
		t.Fatalf("ListSessions() error = %v", listErr)
	}
	if len(sessions) != 1 || sessions[0].UUID != session.UUID {
		t.Fatalf("ListSessions() = %+v, want created group", sessions)
	}
	if len(sessions[0].Members) != 2 {
		t.Fatalf("ListSessions() members = %+v, want 2 current members", sessions[0].Members)
	}
	if sessions[0].Members[1].Agent.Status != "inactive" {
		t.Fatalf("second member status = %q, want inactive", sessions[0].Members[1].Agent.Status)
	}
}

func TestCreateGroupSessionRejectsInvalidMemberBeforePersistence(t *testing.T) {
	activeUID := uuid.New()
	inactiveUID := uuid.New()
	unavailableUID := uuid.New()
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{
		activeUID.String(): {
			ID: 1, UUID: activeUID, Name: "太上老君", Status: "active", ModelName: "available-model",
		},
		inactiveUID.String(): {
			ID: 2, UUID: inactiveUID, Name: "睡道人", Status: "inactive", ModelName: "available-model",
		},
		unavailableUID.String(): {
			ID: 3, UUID: unavailableUID, Name: "无凭道人", Status: "active", ModelName: "unavailable-model",
		},
	}}

	tests := []struct {
		name     string
		uids     []uuid.UUID
		wantCode string
	}{
		{
			name:     "inactive member",
			uids:     []uuid.UUID{activeUID, inactiveUID},
			wantCode: "service.chat.agent_inactive",
		},
		{
			name:     "member model unavailable",
			uids:     []uuid.UUID{activeUID, unavailableUID},
			wantCode: "service.chat.model_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chats := &fakeChatDao{
				sessions: map[string]*model.ChatSession{},
				members:  map[uint][]*model.SessionMember{},
			}
			resolver := fakeCredentialResolver{credentials: map[string]*credential.ModelCredentials{
				"available-model":   {Model: "available-model", APIKey: "test-api-key"},
				"unavailable-model": {Model: "unavailable-model"},
			}}
			svc := New(chats, agents, nil, resolver, "http://unused")

			session, err := svc.CreateGroupSession(context.Background(), tt.uids)

			if err == nil {
				t.Fatalf("CreateGroupSession() error = nil, want %s", tt.wantCode)
			}
			if session != nil {
				t.Fatalf("CreateGroupSession() session = %+v, want nil", session)
			}
			if err.GetCode() != tt.wantCode {
				t.Fatalf("CreateGroupSession() error code = %q, want %q", err.GetCode(), tt.wantCode)
			}
			if len(chats.sessions) != 0 {
				t.Fatalf("CreateGroupSession() persisted %d sessions after validation failure", len(chats.sessions))
			}
			if len(chats.members) != 0 {
				t.Fatalf("CreateGroupSession() persisted members after validation failure: %+v", chats.members)
			}
		})
	}
}

func TestAddAndRemoveMember(t *testing.T) {
	svc, chats, u1, u2, _ := newGroupTestSvc()
	ctx := context.Background()
	s, _ := svc.CreateGroupSession(ctx, []uuid.UUID{u1, u2})

	// 重复邀请静默跳过
	if err := svc.AddMembers(ctx, s.UUID, []uuid.UUID{u1}); err != nil {
		t.Fatalf("AddMembers 重复邀请: %v", err)
	}
	if len(chats.members[s.ID]) != 2 {
		t.Fatal("重复邀请产生了重复成员")
	}

	// 踢人 + 通知消息
	if err := svc.RemoveMember(ctx, s.UUID, u1); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if len(chats.members[s.ID]) != 1 {
		t.Fatal("踢人失败")
	}
	var notice *model.ChatMessage
	for _, m := range chats.messages {
		if m.Role == "system" && strings.Contains(m.Content, "移出") {
			notice = m
		}
	}
	if notice == nil {
		t.Fatal("踢人未落系统通知消息")
	}

	// 踢不存在成员 → 错误
	if err := svc.RemoveMember(ctx, s.UUID, u1); err == nil {
		t.Fatal("踢不存在成员应报错")
	}
}

func TestUpdateSessionTitleValidation(t *testing.T) {
	svc, _, u1, _, _ := newGroupTestSvc()
	ctx := context.Background()
	s, err := svc.CreateSession(ctx, u1)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := svc.UpdateSessionTitle(ctx, s.UUID, "   "); err == nil {
		t.Fatal("空白标题应报错")
	}
	if err := svc.UpdateSessionTitle(ctx, s.UUID, strings.Repeat("长", 31)); err == nil {
		t.Fatal("超30字标题应报错")
	}
	if err := svc.UpdateSessionTitle(ctx, s.UUID, "丹道夜话"); err != nil {
		t.Fatalf("合法标题: %v", err)
	}
}
