package chat

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/behavior"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	chatservice "github.com/alchemy-furnace/server/internal/service/chat_service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/turnpolicy"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type sseChatStub struct {
	service.Chat
	session        *model.ChatSession
	sessionErr     errors.Error
	pattern        *model.LanguagePattern
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
	lastMessages   []map[string]string
	lastOptions    service.GenerationOptions
	members        []*model.SessionMember

	retrievedSnippets   []turnpolicy.MemorySnippet
	retrieveCalls       int
	lastRetrieveAgentID uint
	lastRetrieveMessage string
	distillSpecs        []service.DistillationSpec
}

// P3 记忆挂载:检索委托 + 蒸馏入队(handler 经 service.Chat 接口调用)
func (s *sseChatStub) RetrieveMemories(_ context.Context, agentID uint, userMessage string) []turnpolicy.MemorySnippet {
	s.retrieveCalls++
	s.lastRetrieveAgentID = agentID
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

func (s *sseChatStub) GetOrBuildPattern(context.Context, uint) (*model.LanguagePattern, errors.Error) {
	s.patternCalls++
	if s.patternErr != nil {
		return nil, s.patternErr
	}
	if s.pattern != nil {
		return s.pattern, nil
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

func (s *sseChatStub) StreamChat(_ context.Context, messages []map[string]string, _ *credential.ModelCredentials, options service.GenerationOptions, onChunk func(string)) (string, bool, error) {
	s.engineCalls++
	s.lastMessages = messages
	s.lastOptions = options
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

// 普通单聊:系统消息必须含四个动态分区(§11),且带行为档案时激活丹性规则
func TestSSEChatSystemMessageIncludesDynamicSections(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	profileJSON, err := behavior.ProfileToJSONMap(behavior.CompileProfile("沉稳", []synthesis.PillInput{{
		ID: "p1", Name: "古琴丹", Weight: 2.0, SortOrder: 0,
		SkillSchema: model.JSONMap{
			"description": "以古琴之道应答",
			"mental_models": []any{map[string]any{
				"name": "知音", "one_liner": "先问对方所好再谈琴",
			}},
		},
	}}))
	if err != nil {
		t.Fatalf("ProfileToJSONMap() error = %v", err)
	}
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model", Proactivity: 60},
		},
		pattern:    &model.LanguagePattern{SystemPrompt: "cached static prompt", BehaviorProfile: profileJSON},
		streamFull: "ok",
	}

	w := performSSEChatBody(t, stub, sessionUID, `{"content":"聊点音乐"} `)
	body := w.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Fatalf("SSE body = %q, want done", body)
	}
	if stub.engineCalls != 1 || len(stub.lastMessages) == 0 {
		t.Fatalf("engine calls = %d, messages = %d", stub.engineCalls, len(stub.lastMessages))
	}
	sys := stub.lastMessages[0]
	if sys["role"] != "system" {
		t.Fatalf("首条应为 system: %+v", sys)
	}
	for _, title := range []string{"本轮激活丹性", "本地记忆事实", "用户当轮要求", "回答与群聊预算"} {
		if !strings.Contains(sys["content"], "【"+title+"】") {
			t.Fatalf("系统消息缺少动态分区 %q:\n%s", title, sys["content"])
		}
	}
	if !strings.Contains(sys["content"], "古琴丹") {
		t.Fatalf("系统消息应含激活丹性规则:\n%s", sys["content"])
	}
}

// 用户明确停止:引擎 0 调用,直接 done
func TestSSEChatStopRequestSkipsEngine(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
	}

	w := performSSEChatBody(t, stub, sessionUID, `{"content":"够了，别说了"}`)
	body := w.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Fatalf("SSE body = %q, want done for stop request", body)
	}
	if strings.Contains(body, "event: chunk") {
		t.Fatalf("SSE body = %q, stop must not stream chunks", body)
	}
	if stub.engineCalls != 0 {
		t.Fatalf("StreamChat calls = %d, stop must not call engine", stub.engineCalls)
	}
	if stub.titleCalls != 0 {
		t.Fatalf("GenerateSessionTitle calls = %d, stop must not generate title", stub.titleCalls)
	}
	if strings.Join(stub.savedRoles, ",") != "user" {
		t.Fatalf("saved roles = %v, want user only", stub.savedRoles)
	}
}

// 空的模型输出不是成功回复：不能发送 done 或触发自动命名。
func TestSSEChatEmptyModelOutputReturnsError(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 1, UUID: sessionUID, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Name: "测试道人", Status: "active", ModelName: "test-model", Proactivity: 50},
		},
		streamFull: "   ",
	}
	w := performSSEChat(t, stub, sessionUID)
	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("empty model output must emit error: %q", body)
	}
	if strings.Contains(body, "event: done") {
		t.Fatalf("empty model output must not emit done: %q", body)
	}
}

// TurnPlan.MaxTokens 必须经 GenerationOptions 传向引擎
func TestSSEChatPassesMaxTokensFromPlan(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
		streamFull: "详细回答",
	}

	w := performSSEChatBody(t, stub, sessionUID, `{"content":"详细讲讲这个方案"}`)
	if !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("SSE body = %q, want done", w.Body.String())
	}
	// 单聊详细:TurnPlan.MaxTokens = deep_dive 档 896(不再提升到 2048)
	if stub.lastOptions.MaxTokens != 896 {
		t.Fatalf("lastOptions = %+v, want MaxTokens=896", stub.lastOptions)
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

// 单聊响应必须携带道人真实身份(名称/头像/状态),不能只有 UUID
func TestSessionResponseIncludesSingleAgentIdentity(t *testing.T) {
	agentID := uint(7)
	agentUID := uuid.New()
	session := &model.ChatSession{
		UUID: uuid.New(), Type: model.SessionTypeSingle, AgentID: &agentID,
		Agent: model.DaoAgent{UUID: agentUID, Name: "太上老君", Avatar: "https://example.com/laojun.png", Status: "inactive"},
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
		UUID: uuid.New(), Type: model.SessionTypeGroup,
		Agent: model.DaoAgent{UUID: uuid.New(), Name: "太上老君", Avatar: "https://example.com/laojun.png", Status: "inactive"},
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

// ---------- P3 本地记忆挂载(§10.3/§10.4) ----------

// memoryProfile 最小档案:仅保证 profile 非 nil,动态四分区正常渲染
func memoryProfile(t *testing.T) model.JSONMap {
	t.Helper()
	m, err := behavior.ProfileToJSONMap(&behavior.DaoistBehaviorProfile{BasePersonality: "以丹道应世"})
	if err != nil {
		t.Fatalf("ProfileToJSONMap() error = %v", err)
	}
	return m
}

// P3:memory_enabled 道人 → 检索结果注入 system 记忆分区,以 agentID+用户内容检索
func TestSSEChatMemoryInjectedIntoPrompt(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model", MemoryEnabled: true},
		},
		pattern:           &model.LanguagePattern{SystemPrompt: "cached static prompt", BehaviorProfile: memoryProfile(t)},
		streamFull:        "道友请讲",
		retrievedSnippets: []turnpolicy.MemorySnippet{{Kind: "fact", Content: "用户喜欢围棋"}},
	}

	w := performSSEChat(t, stub, sessionUID)
	if !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("SSE body = %q, want done", w.Body.String())
	}
	if stub.retrieveCalls != 1 {
		t.Fatalf("RetrieveMemories calls = %d, want 1", stub.retrieveCalls)
	}
	if stub.lastRetrieveAgentID != agentID || stub.lastRetrieveMessage != "hello" {
		t.Fatalf("检索入参 agentID=%d message=%q, want %d/hello", stub.lastRetrieveAgentID, stub.lastRetrieveMessage, agentID)
	}
	sys := stub.lastMessages[0]["content"]
	if !strings.Contains(sys, "【本地记忆事实】") || !strings.Contains(sys, "用户喜欢围棋") {
		t.Fatalf("system 消息应含检索记忆:\n%s", sys)
	}
}

// P3:MemoryEnabled=false → 不检索不注入不蒸馏(门控)
func TestSSEChatMemoryDisabledNoInjection(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model"},
		},
		pattern:    &model.LanguagePattern{SystemPrompt: "cached static prompt", BehaviorProfile: memoryProfile(t)},
		streamFull: "道友请讲",
	}

	w := performSSEChat(t, stub, sessionUID)
	if !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("SSE body = %q, want done", w.Body.String())
	}
	if stub.retrieveCalls != 0 {
		t.Fatalf("MemoryEnabled=false 不得检索: calls = %d", stub.retrieveCalls)
	}
	if len(stub.distillSpecs) != 0 {
		t.Fatalf("MemoryEnabled=false 不得蒸馏: %+v", stub.distillSpecs)
	}
}

// P3:回复成功后异步入队蒸馏,spec 携带 session/model/用户消息与单目标双消息
func TestSSEChatEnqueueDistillationAfterReply(t *testing.T) {
	sessionUID := uuid.New()
	agentID := uint(7)
	stub := &sseChatStub{
		session: &model.ChatSession{
			ID: 3, UUID: sessionUID, Type: model.SessionTypeSingle, AgentID: &agentID,
			Agent: model.DaoAgent{ID: agentID, UUID: uuid.New(), Status: "active", ModelName: "test-model", MemoryEnabled: true},
		},
		streamFull: "金丹妙不可言",
	}

	w := performSSEChat(t, stub, sessionUID)
	if !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("SSE body = %q, want done", w.Body.String())
	}
	if len(stub.distillSpecs) != 1 {
		t.Fatalf("蒸馏 spec 数 = %d, want 1", len(stub.distillSpecs))
	}
	spec := stub.distillSpecs[0]
	if spec.SessionUUID != sessionUID.String() || spec.Model != "test-model" || spec.UserMessage != "hello" {
		t.Fatalf("spec = %+v, want session/model/userMessage 匹配", spec)
	}
	if len(spec.Targets) != 1 {
		t.Fatalf("Targets = %d, want 1", len(spec.Targets))
	}
	tgt := spec.Targets[0]
	if tgt.AgentID != agentID {
		t.Fatalf("Target.AgentID = %d, want %d", tgt.AgentID, agentID)
	}
	if len(tgt.Messages) != 2 || tgt.Messages[0].Role != "user" || tgt.Messages[0].Content != "hello" ||
		tgt.Messages[1].Role != "assistant" || tgt.Messages[1].Content != "金丹妙不可言" {
		t.Fatalf("Target.Messages = %+v, want [user/hello, assistant/金丹妙不可言]", tgt.Messages)
	}
}
