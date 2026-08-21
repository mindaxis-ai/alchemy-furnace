package chat

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	chatservice "github.com/alchemy-furnace/server/internal/service/chat_service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type sseChatStub struct {
	service.Chat
	session        *model.ChatSession
	sessionErr     errors.Error
	engineCalls    int
	patternCalls   int
	savedRoles     []string
	recentMessages []*model.ChatMessage
	streamChunks   []string
	streamFull     string
	streamErr      error
	members        []*model.SessionMember
}

func (s *sseChatStub) GetSessionAgentInfo(context.Context, uuid.UUID) (*model.ChatSession, errors.Error) {
	return s.session, s.sessionErr
}

func (s *sseChatStub) GetOrBuildPattern(context.Context, uint) (*model.LanguagePattern, errors.Error) {
	s.patternCalls++
	return &model.LanguagePattern{SystemPrompt: "test system prompt"}, nil
}

func (s *sseChatStub) AuthorizeSessionForStream(_ context.Context, session *model.ChatSession) (*credential.ModelCredentials, errors.Error) {
	if session.Agent.Status != "active" {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.chat.agent_inactive", "道人已停用")
	}
	return &credential.ModelCredentials{Model: session.Agent.ModelName, APIKey: "must-not-leak"}, nil
}

func (s *sseChatStub) SaveMessage(_ context.Context, sessionID uint, role, content string) (*model.ChatMessage, errors.Error) {
	s.savedRoles = append(s.savedRoles, role)
	return &model.ChatMessage{SessionID: sessionID, Role: role, Content: content}, nil
}

func (s *sseChatStub) GetMessages(context.Context, uuid.UUID, int, int) (int64, []*model.ChatMessage, errors.Error) {
	return int64(len(s.recentMessages)), s.recentMessages, nil
}

func (s *sseChatStub) ResolveCredentials(context.Context, string) (*credential.ModelCredentials, errors.Error) {
	return &credential.ModelCredentials{Model: "test-model", APIKey: "must-not-leak"}, nil
}

func (s *sseChatStub) StreamChat(_ context.Context, _ []map[string]string, _ *credential.ModelCredentials, onChunk func(string)) (string, bool, error) {
	s.engineCalls++
	for _, chunk := range s.streamChunks {
		onChunk(chunk)
	}
	return s.streamFull, false, s.streamErr
}

func (s *sseChatStub) GenerateSessionTitle(context.Context, uuid.UUID, string, string) string {
	return ""
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
	if stub.engineCalls != 0 {
		t.Fatalf("StreamChat calls = %d, want 0", stub.engineCalls)
	}
	if stub.patternCalls != 0 {
		t.Fatalf("GetOrBuildPattern calls = %d, want 0 before inactive rejection", stub.patternCalls)
	}
	if len(stub.savedRoles) != 0 {
		t.Fatalf("SaveMessage roles = %v, want none before inactive rejection", stub.savedRoles)
	}
}

func TestSSEChatInterruptedUpstreamRetainsPartialAndNeverEmitsDone(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
		streamChunks: []string{"partial answer"},
		streamFull:   "partial answer",
		streamErr:    &chatservice.StreamInterruptedError{},
	}

	w := performSSEChat(t, stub, sessionUID)
	body := w.Body.String()

	if !strings.Contains(body, `event: chunk`) || !strings.Contains(body, `partial answer`) {
		t.Fatalf("SSE body = %q, want retained partial chunk", body)
	}
	if !strings.Contains(body, `"error_code":"service.chat.stream_interrupted"`) || !strings.Contains(body, `"terminal":true`) {
		t.Fatalf("SSE body = %q, want safe terminal interruption", body)
	}
	if strings.Contains(body, "event: done") {
		t.Fatalf("SSE body = %q, interrupted stream must not emit done", body)
	}
	if strings.Join(stub.savedRoles, ",") != "user" {
		t.Fatalf("SaveMessage roles = %v, want user only (partial assistant stays client-side)", stub.savedRoles)
	}
}

func TestSSEChatUnknownStreamErrorDoesNotLeakRawDetails(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
		streamErr: stderrors.New("raw upstream error containing api_key=must-not-leak"),
	}

	w := performSSEChat(t, stub, sessionUID)
	body := w.Body.String()

	if strings.Contains(body, "raw upstream") || strings.Contains(body, "must-not-leak") {
		t.Fatalf("SSE body leaked raw stream error: %q", body)
	}
	if !strings.Contains(body, `"error_code":"service.chat.stream_unavailable"`) || !strings.Contains(body, `"terminal":true`) {
		t.Fatalf("SSE body = %q, want stable terminal stream_unavailable error", body)
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

func TestSSEChatRetryDoesNotPersistDuplicateUserMessage(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
		recentMessages: []*model.ChatMessage{{SessionID: 3, Role: "user", Content: "hello"}},
		streamFull:     "completed retry",
	}

	w := performSSEChatBody(t, stub, sessionUID, `{"content":"hello","retry":true}`)
	if !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("SSE body = %q, want successful retry", w.Body.String())
	}
	for _, role := range stub.savedRoles {
		if role == "user" {
			t.Fatalf("SaveMessage roles = %v, retry duplicated persisted user", stub.savedRoles)
		}
	}
}

func TestStreamInterruptedErrorSupportsErrorsAs(t *testing.T) {
	var target *chatservice.StreamInterruptedError
	if !stderrors.As(&chatservice.StreamInterruptedError{}, &target) {
		t.Fatal("typed interruption must support errors.As at the handler boundary")
	}
}

func TestSSEChatInactiveAgentReturnsStableErrorBeforeEngine(t *testing.T) {
	sessionUID := uuid.New()
	agentUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{session: &model.ChatSession{
		ID:      3,
		UUID:    sessionUID,
		Type:    model.SessionTypeSingle,
		AgentID: &agentID,
		Agent: model.DaoAgent{
			ID: agentID, UUID: agentUID, Name: "Dormant", Status: "inactive", ModelName: "test-model",
		},
	}}

	w := performSSEChat(t, stub, sessionUID)
	body := w.Body.String()

	if !strings.Contains(body, "event: error") || !strings.Contains(body, `"error_code":"service.chat.agent_inactive"`) {
		t.Fatalf("SSE body = %q, want stable agent_inactive error", body)
	}
	if stub.engineCalls != 0 {
		t.Fatalf("StreamChat calls = %d, want 0", stub.engineCalls)
	}
	if stub.patternCalls != 0 {
		t.Fatalf("GetOrBuildPattern calls = %d, want 0 before inactive authorization failure", stub.patternCalls)
	}
	if len(stub.savedRoles) != 0 {
		t.Fatalf("SaveMessage roles = %v, want no persistence before inactive authorization failure", stub.savedRoles)
	}
}

func TestSessionResponseIncludesStatusesAndCurrentMembers(t *testing.T) {
	agentID := uint(1)
	session := &model.ChatSession{
		UUID: uuid.New(), Type: model.SessionTypeGroup, AgentID: &agentID,
		Agent: model.DaoAgent{UUID: uuid.New(), Status: "inactive"},
		Members: []model.SessionMember{
			{AgentID: 2, Agent: model.DaoAgent{UUID: uuid.New(), Name: "Alpha", Status: "active"}},
			{AgentID: 3, Agent: model.DaoAgent{UUID: uuid.New(), Name: "Beta", Status: "inactive"}},
		},
	}

	data, err := json.Marshal(toSessionResponse(session))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var response struct {
		AgentStatus string `json:"agent_status"`
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
	if len(response.Members) != 2 || response.Members[1].Status != "inactive" {
		t.Fatalf("members = %+v, want current members with statuses", response.Members)
	}
}

func TestGetSessionReturnsDirectGroupMetadata(t *testing.T) {
	sessionUID := uuid.New()
	stub := &sseChatStub{
		session: &model.ChatSession{ID: 7, UUID: sessionUID, Type: model.SessionTypeGroup, Title: "Deep link"},
		members: []*model.SessionMember{{
			AgentID: 4,
			Agent:   model.DaoAgent{UUID: uuid.New(), Name: "Current member", Avatar: "/member.png", Status: "inactive"},
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
