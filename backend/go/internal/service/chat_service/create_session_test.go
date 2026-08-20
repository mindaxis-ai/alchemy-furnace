package chat_service

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

type fakeCredentialResolver struct {
	credentials map[string]*credential.ModelCredentials
	errors      map[string]error
}

func (f fakeCredentialResolver) ResolveCredentials(_ context.Context, name string) (*credential.ModelCredentials, error) {
	if err := f.errors[name]; err != nil {
		return nil, err
	}
	creds := f.credentials[name]
	if creds == nil {
		return nil, nil
	}
	copy := *creds
	return &copy, nil
}

func (f fakeCredentialResolver) ResolveSynthesisCredentials(ctx context.Context) (*credential.ModelCredentials, error) {
	return f.ResolveCredentials(ctx, "")
}

func (f fakeCredentialResolver) ResolveFusionCredentials(ctx context.Context) (*credential.ModelCredentials, error) {
	return f.ResolveCredentials(ctx, "")
}

func availableCredentialResolver(modelNames ...string) fakeCredentialResolver {
	credentials := make(map[string]*credential.ModelCredentials, len(modelNames))
	for _, name := range modelNames {
		credentials[name] = &credential.ModelCredentials{Model: name, APIKey: "test-api-key"}
	}
	return fakeCredentialResolver{credentials: credentials}
}

func TestCreateSessionRejectsInvalidAgentBeforePersistence(t *testing.T) {
	activeUID := uuid.New()
	inactiveUID := uuid.New()
	missingUID := uuid.New()
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{
		activeUID.String(): {
			ID: 1, UUID: activeUID, Name: "太上老君", Status: "active", ModelName: "formal-model",
		},
		inactiveUID.String(): {
			ID: 2, UUID: inactiveUID, Name: "睡道人", Status: "inactive", ModelName: "formal-model",
		},
	}}

	tests := []struct {
		name     string
		agentUID uuid.UUID
		resolver fakeCredentialResolver
		wantCode string
	}{
		{
			name:     "agent not found",
			agentUID: missingUID,
			resolver: availableCredentialResolver("formal-model"),
			wantCode: "service.chat.agent_not_found",
		},
		{
			name:     "agent inactive",
			agentUID: inactiveUID,
			resolver: availableCredentialResolver("formal-model"),
			wantCode: "service.chat.agent_inactive",
		},
		{
			name:     "credential resolution fails",
			agentUID: activeUID,
			resolver: fakeCredentialResolver{errors: map[string]error{"formal-model": stderrors.New("resolver failed")}},
			wantCode: "service.chat.model_unavailable",
		},
		{
			name:     "credential resolution has no api key",
			agentUID: activeUID,
			resolver: fakeCredentialResolver{credentials: map[string]*credential.ModelCredentials{
				"formal-model": {Model: "formal-model", BaseURL: "https://example.invalid"},
			}},
			wantCode: "service.chat.model_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chats := &fakeChatDao{
				sessions: map[string]*model.ChatSession{},
				members:  map[uint][]*model.SessionMember{},
			}
			svc := New(chats, agents, nil, tt.resolver, "http://unused")

			session, err := svc.CreateSession(context.Background(), tt.agentUID)

			if err == nil {
				t.Fatalf("CreateSession() error = nil, want %s", tt.wantCode)
			}
			if session != nil {
				t.Fatalf("CreateSession() session = %+v, want nil", session)
			}
			if err.GetCode() != tt.wantCode {
				t.Fatalf("CreateSession() error code = %q, want %q", err.GetCode(), tt.wantCode)
			}
			if len(chats.sessions) != 0 {
				t.Fatalf("CreateSession() persisted %d sessions after validation failure", len(chats.sessions))
			}
		})
	}
}

func TestCreateSessionCreatesUUIDForAvailableFormalModel(t *testing.T) {
	agentUID := uuid.New()
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{
		agentUID.String(): {
			ID: 7, UUID: agentUID, Name: "太上老君", Status: "active", ModelName: "formal-model",
		},
	}}
	chats := &fakeChatDao{
		sessions: map[string]*model.ChatSession{},
		members:  map[uint][]*model.SessionMember{},
	}
	svc := New(chats, agents, nil, availableCredentialResolver("formal-model"), "http://unused")

	session, err := svc.CreateSession(context.Background(), agentUID)

	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session.UUID == uuid.Nil {
		t.Fatal("CreateSession() UUID is nil")
	}
	if session.Type != model.SessionTypeSingle || session.AgentID == nil || *session.AgentID != 7 {
		t.Fatalf("CreateSession() = %+v, want single session for agent 7", session)
	}
	if _, ok := chats.sessions[session.UUID.String()]; !ok {
		t.Fatalf("CreateSession() UUID %s was not persisted", session.UUID)
	}
}

func TestGetMessagesKeepsInactiveAgentHistoryReadable(t *testing.T) {
	agentUID := uuid.New()
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{
		agentUID.String(): {
			ID: 9, UUID: agentUID, Name: "旧友", Status: "active", ModelName: "formal-model",
		},
	}}
	chats := &fakeChatDao{
		sessions: map[string]*model.ChatSession{},
		members:  map[uint][]*model.SessionMember{},
	}
	svc := New(chats, agents, nil, availableCredentialResolver("formal-model"), "http://unused")
	session, err := svc.CreateSession(context.Background(), agentUID)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := svc.SaveMessage(context.Background(), session.ID, "assistant", "旧日答复"); err != nil {
		t.Fatalf("SaveMessage() error = %v", err)
	}
	agents.agents[agentUID.String()].Status = "inactive"

	_, messages, historyErr := svc.GetMessages(context.Background(), session.UUID, 1, 20)
	if historyErr != nil {
		t.Fatalf("GetMessages() error = %v", historyErr)
	}
	if len(messages) != 1 || messages[0].Content != "旧日答复" {
		t.Fatalf("GetMessages() = %+v, want preserved history", messages)
	}
}
