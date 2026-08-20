package chat

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type sseChatStub struct {
	service.Chat
	session     *model.ChatSession
	sessionErr  errors.Error
	engineCalls int
}

func (s *sseChatStub) GetSessionAgentInfo(context.Context, uuid.UUID) (*model.ChatSession, errors.Error) {
	return s.session, s.sessionErr
}

func (s *sseChatStub) GetOrBuildPattern(context.Context, uint) (*model.LanguagePattern, errors.Error) {
	return &model.LanguagePattern{SystemPrompt: "test system prompt"}, nil
}

func (s *sseChatStub) AuthorizeSessionForStream(_ context.Context, session *model.ChatSession) (*credential.ModelCredentials, errors.Error) {
	if session.Agent.Status != "active" {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.chat.agent_inactive", "道人已停用")
	}
	return &credential.ModelCredentials{Model: session.Agent.ModelName, APIKey: "must-not-leak"}, nil
}

func (s *sseChatStub) SaveMessage(_ context.Context, sessionID uint, role, content string) (*model.ChatMessage, errors.Error) {
	return &model.ChatMessage{SessionID: sessionID, Role: role, Content: content}, nil
}

func (s *sseChatStub) GetMessages(context.Context, uuid.UUID, int, int) (int64, []*model.ChatMessage, errors.Error) {
	return 0, nil, nil
}

func (s *sseChatStub) ResolveCredentials(context.Context, string) (*credential.ModelCredentials, errors.Error) {
	return &credential.ModelCredentials{Model: "test-model", APIKey: "must-not-leak"}, nil
}

func (s *sseChatStub) StreamChat(context.Context, []map[string]string, *credential.ModelCredentials, func(string)) (string, bool, error) {
	s.engineCalls++
	return "engine should not be invoked", false, nil
}

func (s *sseChatStub) GenerateSessionTitle(context.Context, uuid.UUID, string, string) string {
	return ""
}

func performSSEChat(t *testing.T, stub *sseChatStub, sessionUID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/chat/sse/"+sessionUID.String(), strings.NewReader(`{"content":"hello"}`))
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
