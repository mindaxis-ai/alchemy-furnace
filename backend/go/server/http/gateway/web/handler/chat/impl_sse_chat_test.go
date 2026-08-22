package chat

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
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
	patternErr     errors.Error
	saveErr        errors.Error
	saveErrors     map[string]errors.Error
	engineCalls    int
	patternCalls   int
	titleCalls     int
	generatedTitle string
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
	if s.patternErr != nil {
		return nil, s.patternErr
	}
	return &model.LanguagePattern{SystemPrompt: "test system prompt"}, nil
}

func (s *sseChatStub) AuthorizeSessionForStream(_ context.Context, session *model.ChatSession) (*credential.ModelCredentials, errors.Error) {
	if session.Agent.Status != "active" {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.chat.agent_inactive", "道人已停用")
	}
	return &credential.ModelCredentials{Model: session.Agent.ModelName, APIKey: "must-not-leak"}, nil
}

func (s *sseChatStub) SaveMessage(_ context.Context, sessionID uint, role, content string) (*model.ChatMessage, errors.Error) {
	if err := s.saveErrors[role]; err != nil {
		return nil, err
	}
	if s.saveErr != nil {
		return nil, s.saveErr
	}
	s.savedRoles = append(s.savedRoles, role)
	return &model.ChatMessage{SessionID: sessionID, Role: role, Content: content}, nil
}

func (s *sseChatStub) GetMessages(_ context.Context, _ uuid.UUID, page, size int) (int64, []*model.ChatMessage, errors.Error) {
	start := (page - 1) * size
	if start >= len(s.recentMessages) {
		return int64(len(s.recentMessages)), nil, nil
	}
	end := min(start+size, len(s.recentMessages))
	return int64(len(s.recentMessages)), s.recentMessages[start:end], nil
}

func (s *sseChatStub) TakeLatestUserMessage(context.Context, uint) (*model.ChatMessage, errors.Error) {
	for i := len(s.recentMessages) - 1; i >= 0; i-- {
		if s.recentMessages[i].Role == "user" {
			return s.recentMessages[i], nil
		}
	}
	return nil, errors.ErrorRecordNotFound("test.latest_user")
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
	if !strings.Contains(body, `event: accepted`) || !strings.Contains(body, `"recovery":"persisted_retry"`) {
		t.Fatalf("SSE body = %q, persisted user must be acknowledged before recoverable interruption", body)
	}
	if strings.Contains(body, "event: done") {
		t.Fatalf("SSE body = %q, interrupted stream must not emit done", body)
	}
	if strings.Join(stub.savedRoles, ",") != "user" {
		t.Fatalf("SaveMessage roles = %v, want user only (partial assistant stays client-side)", stub.savedRoles)
	}
}

func TestSSEChatAssistantSaveFailureTerminatesWithSanitizedPersistedRetry(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
		streamChunks:   []string{"complete answer"},
		streamFull:     "complete answer",
		generatedTitle: "must not be emitted",
		saveErrors: map[string]errors.Error{
			"assistant": errors.ErrorServerInternalError("secret.database.assistant_write"),
		},
	}

	w := performSSEChat(t, stub, sessionUID)
	body := w.Body.String()

	if !strings.Contains(body, "event: accepted") {
		t.Fatalf("SSE body = %q, want accepted after persisted user message", body)
	}
	if !strings.Contains(body, "event: error") ||
		!strings.Contains(body, `"error_code":"service.chat.stream_unavailable"`) ||
		!strings.Contains(body, `"terminal":true`) ||
		!strings.Contains(body, `"recovery":"persisted_retry"`) {
		t.Fatalf("SSE body = %q, want terminal persisted_retry persistence error", body)
	}
	if strings.Contains(body, "secret.database") || strings.Contains(body, "must not be emitted") {
		t.Fatalf("SSE body leaked persistence details or generated a title: %q", body)
	}
	if strings.Contains(body, "event: done") || strings.Contains(body, "event: title") {
		t.Fatalf("SSE body = %q, failed assistant persistence must not emit title/done", body)
	}
	if stub.titleCalls != 0 {
		t.Fatalf("GenerateSessionTitle calls = %d, want 0 after assistant save failure", stub.titleCalls)
	}
	if len(stub.savedRoles) != 1 || stub.savedRoles[0] != "user" {
		t.Fatalf("persisted roles = %v, want only the successful user row", stub.savedRoles)
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

func TestSSEChatPatternFailureOffersNormalResendBeforePersistence(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
		patternErr: errors.ErrorServerInternalError("secret.pattern.failure"),
	}

	w := performSSEChat(t, stub, sessionUID)
	body := w.Body.String()

	if !strings.Contains(body, `"recovery":"resend"`) {
		t.Fatalf("SSE body = %q, pre-persist pattern failure must offer normal resend", body)
	}
	if len(stub.savedRoles) != 0 || stub.engineCalls != 0 {
		t.Fatalf("pattern failure saved roles %v or called engine %d", stub.savedRoles, stub.engineCalls)
	}
}

func TestSSEChatUserSaveFailureOffersNormalResend(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
		saveErr: errors.ErrorServerInternalError("secret.user.save.failure"),
	}

	w := performSSEChat(t, stub, sessionUID)
	body := w.Body.String()

	if !strings.Contains(body, `"recovery":"resend"`) {
		t.Fatalf("SSE body = %q, failed user persistence must offer normal resend", body)
	}
	if strings.Contains(body, "secret.user.save.failure") || stub.engineCalls != 0 {
		t.Fatalf("save failure leaked details or called engine: body=%q engine=%d", body, stub.engineCalls)
	}
}

func TestSSEChatPersistedRetryPatternFailureNeverOffersNormalResend(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
		patternErr:     errors.ErrorServerInternalError("secret.pattern.failure"),
		recentMessages: []*model.ChatMessage{{SessionID: 3, Role: "user", Content: "persisted question"}},
	}

	w := performSSEChatBody(t, stub, sessionUID, `{"content":"persisted question","retry":true}`)
	body := w.Body.String()

	if !strings.Contains(body, `"recovery":"persisted_retry"`) || strings.Contains(body, `"recovery":"resend"`) {
		t.Fatalf("SSE body = %q, failure during persisted retry must keep persisted recovery", body)
	}
	if len(stub.savedRoles) != 0 {
		t.Fatalf("SaveMessage roles = %v, persisted retry must not save another user", stub.savedRoles)
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

func TestSSEChatRetryUsesLatestUserBeyondFirstHistoryPage(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	history := []*model.ChatMessage{{SessionID: 3, Role: "user", Content: "stale old question"}}
	for i := 0; i < 20; i++ {
		history = append(history, &model.ChatMessage{SessionID: 3, Role: "assistant", Content: fmt.Sprintf("old reply %d", i)})
	}
	history = append(history, &model.ChatMessage{SessionID: 3, Role: "user", Content: "latest question"})
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
		recentMessages: history,
		streamFull:     "completed retry",
	}

	w := performSSEChatBody(t, stub, sessionUID, `{"content":"latest question","retry":true}`)

	if !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("SSE body = %q, retry should validate the latest user beyond page 1", w.Body.String())
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
