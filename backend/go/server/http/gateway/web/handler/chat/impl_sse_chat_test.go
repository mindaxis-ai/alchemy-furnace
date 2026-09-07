package chat

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	chatservice "github.com/alchemy-furnace/server/internal/service/chat_service"
	"github.com/alchemy-furnace/server/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// sseChatStub handler 层测试桩(Task 15 LangGraph 权威化后 handler 只做传输适配):
// 只覆盖 handler 传播路径触达的方法;legacy 组装链相关桩(StreamChat/
// AuthorizeSessionForStream/RunGroupTurn/RetryGroupTurn)已随接口收缩删除。
type sseChatStub struct {
	service.Chat
	session        *model.ChatSession
	sessionErr     errors.Error
	saveErr        errors.Error
	saveErrors     map[string]errors.Error
	titleCalls     int
	generatedTitle string
	savedRoles     []string
	recentMessages []*model.ChatMessage
	members        []*model.SessionMember

	retrievedSnippets   []service.MemorySnippet
	retrieveCalls       int
	lastRetrieveAgentID string
	lastRetrieveMessage string
	distillSpecs        []service.DistillationSpec

	runConversationCalls int
	lastCommand          service.ConversationCommand

	resumeCalls      int
	lastResumeRunUID uuid.UUID
}

// P3 记忆挂载:检索委托 + 蒸馏入队(handler 经 service.Chat 接口调用)
func (s *sseChatStub) RetrieveMemories(_ context.Context, agentUID string, userMessage string) []service.MemorySnippet {
	s.retrieveCalls++
	s.lastRetrieveAgentID = agentUID
	s.lastRetrieveMessage = userMessage
	return s.retrievedSnippets
}

func (s *sseChatStub) EnqueueMemoryDistillation(_ context.Context, spec service.DistillationSpec) bool {
	s.distillSpecs = append(s.distillSpecs, spec)
	return true
}

func (s *sseChatStub) GetSessionAgentInfo(context.Context, uuid.UUID) (*model.ChatSession, errors.Error) {
	return s.session, s.sessionErr
}

func (s *sseChatStub) SaveMessage(_ context.Context, sessionUID string, role, content string) (*model.ChatMessage, errors.Error) {
	if err := s.saveErrors[role]; err != nil {
		return nil, err
	}
	if s.saveErr != nil {
		return nil, s.saveErr
	}
	s.savedRoles = append(s.savedRoles, role)
	return &model.ChatMessage{SessionID: sessionUID, Role: role, Content: content}, nil
}

func (s *sseChatStub) GetMessages(_ context.Context, _ uuid.UUID, page, size int) (int64, []*model.ChatMessage, errors.Error) {
	start := (page - 1) * size
	if start >= len(s.recentMessages) {
		return int64(len(s.recentMessages)), nil, nil
	}
	end := min(start+size, len(s.recentMessages))
	return int64(len(s.recentMessages)), s.recentMessages[start:end], nil
}

func (s *sseChatStub) TakeLatestUserMessage(context.Context, string) (*model.ChatMessage, errors.Error) {
	for i := len(s.recentMessages) - 1; i >= 0; i-- {
		if s.recentMessages[i].Role == "user" {
			return s.recentMessages[i], nil
		}
	}
	return nil, errors.ErrorRecordNotFound("test.latest_user")
}

func (s *sseChatStub) GenerateSessionTitle(context.Context, uuid.UUID, string, string) string {
	s.titleCalls++
	return s.generatedTitle
}

func (s *sseChatStub) ListMembers(context.Context, uuid.UUID) ([]*model.SessionMember, errors.Error) {
	return s.members, nil
}

func performSSEChat(t *testing.T, stub *sseChatStub, sessionUID uuid.UUID) *httptest.ResponseRecorder {
	return performSSEChatBody(t, stub, sessionUID, `{"content":"hello"}`)
}

func performSSEChatBody(t *testing.T, stub *sseChatStub, sessionUID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/chat/sse/"+sessionUID.String(), strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "uuid", Value: sessionUID.String()}}

	New(stub).SSEChat(c)
	return w
}

func TestSSEChatMissingSessionReturnsStableSafeError(t *testing.T) {
	sessionUID := uuid.New()
	stub := &sseChatStub{sessionErr: errors.ErrorRecordNotFound("dao.secret.session_lookup")}

	w := performSSEChat(t, stub, sessionUID)
	body := w.Body.String()

	if !strings.Contains(body, "event: error") {
		t.Fatalf("SSE body = %q, want error event", body)
	}
	if !strings.Contains(body, `"error_code":"service.chat.session_not_found"`) {
		t.Fatalf("SSE body = %q, want stable session_not_found code", body)
	}
	if strings.Contains(body, "dao.secret") {
		t.Fatalf("SSE body leaked internal error: %q", body)
	}
	if len(stub.savedRoles) != 0 {
		t.Fatalf("SaveMessage roles = %v, want none before session resolution", stub.savedRoles)
	}
}

func TestGroupMemberErrorWirePayloadExplicitlyMarksNonterminal(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &sseWriter{w: w, flusher: w}

	sw.event("error", chatservice.GroupSpeakerPayload{
		AgentID: "agent-a", AgentName: "Alpha", Content: "member failed",
	})

	if !strings.Contains(w.Body.String(), `"terminal":false`) {
		t.Fatalf("SSE body = %q, nonterminal member error must serialize terminal=false", w.Body.String())
	}
}

func TestSessionResponseIncludesStatusesAndCurrentMembers(t *testing.T) {
	agentID := uuid.NewString()
	session := &model.ChatSession{
		ChatSessionID: uuid.New().String(), Type: model.SessionTypeGroup, AgentID: &agentID,
		Avatar: "https://example.com/group.png",
		Agent:  model.DaoAgent{DaoAgentID: uuid.New().String(), Status: "inactive"},
		Members: []model.SessionMember{
			{AgentID: uuid.NewString(), Agent: model.DaoAgent{DaoAgentID: uuid.New().String(), Name: "Alpha", Status: "active"}},
			{AgentID: uuid.NewString(), Agent: model.DaoAgent{DaoAgentID: uuid.New().String(), Name: "Beta", Status: "inactive"}},
		},
	}

	data, err := json.Marshal(toSessionResponse(session))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var response struct {
		AgentStatus string `json:"agent_status"`
		Avatar      string `json:"avatar"`
		Members     []struct {
			Status string `json:"status"`
		} `json:"members"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if response.AgentStatus != "inactive" {
		t.Fatalf("agent_status = %q, want inactive", response.AgentStatus)
	}
	if response.Avatar != "https://example.com/group.png" {
		t.Fatalf("avatar = %q, want group avatar", response.Avatar)
	}
	if len(response.Members) != 2 || response.Members[1].Status != "inactive" {
		t.Fatalf("members = %+v, want current members with statuses", response.Members)
	}
}

// 单聊响应必须携带道人真实身份(名称/头像/状态),不能只有 UUID
func TestSessionResponseIncludesSingleAgentIdentity(t *testing.T) {
	agentUID := uuid.New()
	agentID := agentUID.String()
	session := &model.ChatSession{
		ChatSessionID: uuid.New().String(), Type: model.SessionTypeSingle, AgentID: &agentID,
		Agent: model.DaoAgent{DaoAgentID: agentUID.String(), Name: "太上老君", Avatar: "https://example.com/laojun.png", Status: "inactive"},
	}
	response := toSessionResponse(session)
	if response.AgentID != agentUID.String() || response.AgentName != "太上老君" {
		t.Fatalf("identity = %+v", response)
	}
	if response.AgentAvatar != "https://example.com/laojun.png" || response.AgentStatus != "inactive" {
		t.Fatalf("avatar/status = %+v", response)
	}
}

// 群聊三个单聊身份字段必须为空或省略,成员身份只来自 members
// 真实群聊的 AgentID 为 NULL(单聊外键),带残留预加载也不得输出身份
func TestSessionResponseOmitsSingleAgentIdentityForGroup(t *testing.T) {
	session := &model.ChatSession{
		ChatSessionID: uuid.New().String(), Type: model.SessionTypeGroup,
		Agent: model.DaoAgent{DaoAgentID: uuid.New().String(), Name: "太上老君", Avatar: "https://example.com/laojun.png", Status: "inactive"},
	}
	data, err := json.Marshal(toSessionResponse(session))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var response struct {
		AgentID     string `json:"agent_id"`
		AgentName   string `json:"agent_name"`
		AgentAvatar string `json:"agent_avatar"`
		AgentStatus string `json:"agent_status"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if response.AgentID != "" || response.AgentName != "" || response.AgentAvatar != "" || response.AgentStatus != "" {
		t.Fatalf("group identity fields must be empty: %+v", response)
	}
}

func TestGetSessionReturnsDirectGroupMetadata(t *testing.T) {
	sessionUID := uuid.New()
	stub := &sseChatStub{
		session: &model.ChatSession{ChatSessionID: sessionUID.String(), Type: model.SessionTypeGroup, Title: "Deep link"},
		members: []*model.SessionMember{{
			AgentID: uuid.NewString(),
			Agent:   model.DaoAgent{DaoAgentID: uuid.New().String(), Name: "Current member", Avatar: "/member.png", Status: "inactive"},
		}},
	}
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/chat/sessions/"+sessionUID.String(), nil)
	c.Params = gin.Params{{Key: "uuid", Value: sessionUID.String()}}

	_, data, err := New(stub).GetSession(c)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	response, ok := data.(*SessionResponse)
	if !ok {
		t.Fatalf("GetSession() data = %T, want *SessionResponse", data)
	}
	if response.ID != sessionUID.String() || len(response.Members) != 1 || response.Members[0].Status != "inactive" {
		t.Fatalf("GetSession() response = %+v, want direct group metadata with current member status", response)
	}
}

// RunConversation 覆写:记录调用与命令,并发两枚事件验证 handler 的 emit 透传。
func (s *sseChatStub) RunConversation(_ context.Context, cmd service.ConversationCommand, emit func(string, any)) {
	s.runConversationCalls++
	s.lastCommand = cmd
	emit("accepted", struct{}{})
	emit("done", struct{}{})
}

// Task 15:LangGraph 权威路径——handler 只做输入校验与委托,不存在任何 legacy 组装链。
func TestSSEChatLangGraphDelegatesWithoutLegacyComposition(t *testing.T) {
	sessionUID := uuid.New()
	agentIDText := uuid.NewString()
	stub := &sseChatStub{
		session: &model.ChatSession{
			ChatSessionID: sessionUID.String(), Type: model.SessionTypeSingle, AgentID: &agentIDText,
			Agent: model.DaoAgent{DaoAgentID: uuid.New().String(), Status: "active", ModelName: "test-model"},
		},
	}
	w := performSSEChatBody(t, stub, sessionUID, `{"content":"hello","retry":true,"debug_prompt":true}`)

	if stub.runConversationCalls != 1 {
		t.Fatalf("RunConversation calls = %d, want 1", stub.runConversationCalls)
	}
	if !strings.Contains(w.Body.String(), "event: accepted") || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("SSE body = %q, want emit 透传 accepted/done", w.Body.String())
	}
	cmd := stub.lastCommand
	if cmd.SessionUID != sessionUID || cmd.Content != "hello" || !cmd.Retry || !cmd.DebugPrompt {
		t.Fatalf("command = %+v, want session/content/retry/debug 完整透传", cmd)
	}
}

// Task 15:群聊同样唯一经 RunConversation(事件透传,不存在 legacy 群编排分支)。
func TestSSEGroupLangGraphDelegatesToRunConversation(t *testing.T) {
	sessionUID := uuid.New()
	stub := &sseChatStub{
		session: &model.ChatSession{ChatSessionID: sessionUID.String(), Type: model.SessionTypeGroup},
	}
	w := performSSEChatBody(t, stub, sessionUID, `{"content":"报数","retry":true,"debug_prompt":true}`)

	if stub.runConversationCalls != 1 {
		t.Fatalf("RunConversation calls = %d, want 1", stub.runConversationCalls)
	}
	if !strings.Contains(w.Body.String(), "event: accepted") || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("SSE body = %q, want emit 透传 accepted/done", w.Body.String())
	}
	cmd := stub.lastCommand
	if cmd.SessionUID != sessionUID || cmd.Content != "报数" || !cmd.Retry || !cmd.DebugPrompt {
		t.Fatalf("command = %+v, want 完整透传", cmd)
	}
}

// ---- Task 14:resume 端点委托(RAW SSE POST /chat/runs/:run_id/resume)----

// RunConversationResume 覆写:记录 run UUID 并透传两枚事件验证 handler 透传。
func (s *sseChatStub) RunConversationResume(_ context.Context, runUID uuid.UUID, emit func(string, any)) {
	s.resumeCalls++
	s.lastResumeRunUID = runUID
	emit("chunk", chatservice.ConversationEventPayload{Content: "续"})
	emit("done", chatservice.ConversationEventPayload{})
}

// performSSEResume 直接以 gin 测试上下文调用 RAW resume 端点。
func performSSEResume(t *testing.T, stub *sseChatStub, runID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/chat/runs/"+runID+"/resume", nil)
	c.Params = gin.Params{{Key: "run_id", Value: runID}}
	New(stub).ResumeRunSSE(c)
	return w
}

// TestSSEResumeRunDelegatesToService resume 端点解析 run_id 并全权委托服务层,事件原样透传。
func TestSSEResumeRunDelegatesToService(t *testing.T) {
	runUID := uuid.New()
	stub := &sseChatStub{}
	w := performSSEResume(t, stub, runUID.String())

	if stub.resumeCalls != 1 || stub.lastResumeRunUID != runUID {
		t.Fatalf("RunConversationResume calls/runUID = %d/%s, want 1/%s", stub.resumeCalls, stub.lastResumeRunUID, runUID)
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: chunk") || !strings.Contains(body, "event: done") {
		t.Fatalf("SSE body = %q, want emit 透传 chunk/done", body)
	}
}

// TestSSEResumeRunInvalidUUIDRejected 非法 run_id 400,不触达服务层。
func TestSSEResumeRunInvalidUUIDRejected(t *testing.T) {
	stub := &sseChatStub{}
	w := performSSEResume(t, stub, "not-a-uuid")
	if w.Code != 400 {
		t.Fatalf("code = %d, want 400", w.Code)
	}
	if stub.resumeCalls != 0 {
		t.Fatalf("resume calls = %d, want 0", stub.resumeCalls)
	}
}
