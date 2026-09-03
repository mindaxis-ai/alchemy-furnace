package chat_service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/behavior"
	"github.com/alchemy-furnace/server/internal/configuration"
	concretedao "github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/engineendpoint"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/turnpolicy"
	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// scriptEngine 按调用次序回放预设 SSE 响应;"|" 分隔 chunk
// completionReply 为非流式 /completions 端点返回的 content(为空时该端点返回 500)
type scriptEngine struct {
	server          *httptest.Server
	replies         []string
	calls           int
	completionReply string
	streamMessages  [][]map[string]string
}

func newScriptEngine(replies []string) *scriptEngine {
	e := &scriptEngine{replies: replies}
	e.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/chat/completions/stream") {
			var request struct {
				Messages []map[string]string `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err == nil {
				e.streamMessages = append(e.streamMessages, request.Messages)
			}
			reply := ""
			if e.calls < len(e.replies) {
				reply = e.replies[e.calls]
			}
			e.calls++
			w.Header().Set("Content-Type", "text/event-stream")
			if reply != "" {
				for _, ch := range strings.Split(reply, "|") {
					fmt.Fprintf(w, "data: {\"content\": %q}\n\n", ch)
				}
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		// 非流式 /chat/completions(模拟 Python 端 BaseResponse 包络)
		if e.completionReply == "" {
			http.Error(w, `{"error":"no completion reply configured"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":0,"message":"ok","data":{"content": %q, "model": "test", "usage": {}}}`, e.completionReply)
	}))
	return e
}

type fakePattern struct{}

func (fakePattern) GetOrBuildPattern(ctx context.Context, agentID uint) (*model.LanguagePattern, errors.Error) {
	return &model.LanguagePattern{SystemPrompt: "你是道人。"}, nil
}

func newGroupSvc(t *testing.T, replies []string) (*Chat, *fakeChatDao, *scriptEngine, *model.ChatSession) {
	return newGroupSvcWithCompletion(t, replies, "")
}

func newGroupSvcWithCompletion(t *testing.T, replies []string, completionReply string) (*Chat, *fakeChatDao, *scriptEngine, *model.ChatSession) {
	t.Helper()
	engine := newScriptEngine(replies)
	engine.completionReply = completionReply
	t.Cleanup(engine.server.Close)
	u1, u2 := uuid.New(), uuid.New()
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{
		u1.String(): {ID: 1, UUID: u1, Name: "太上老君", Avatar: "/laojun.png", Status: "active", Proactivity: 50, ModelName: "test-model"},
		u2.String(): {ID: 2, UUID: u2, Name: "孙悟空", Avatar: "/wukong.png", Status: "active", Proactivity: 90, ModelName: "test-model"},
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
	svc := New(chats, agents, fakePattern{}, availableCredentialResolver("test-model"), engine.server.URL)
	s, err := svc.CreateGroupSession(context.Background(), []uuid.UUID{u1, u2}, "")
	if err != nil {
		t.Fatalf("建群: %v", err)
	}
	return svc, chats, engine, s
}

type eventLog struct{ events []string }

func (l *eventLog) emit(event string, payload any) { l.events = append(l.events, event) }

func countEvent(l *eventLog, name string) int {
	n := 0
	for _, e := range l.events {
		if e == name {
			n++
		}
	}
	return n
}

func TestGroupPromptDebugEmitsEachSpeakersActualModelRequest(t *testing.T) {
	svc, _, engine, session := newGroupSvc(t, []string{"俺老孙收到，给出一段完整回答。"})
	ctx := WithPromptDebug(context.Background(), true)
	var debugPayloads []service.PromptDebugPayload

	svc.RunGroupTurn(ctx, session.UUID, "@孙悟空 请回答", func(event string, payload any) {
		if event != "prompt_debug" {
			return
		}
		debugPayloads = append(debugPayloads, payload.(service.PromptDebugPayload))
	})

	if len(debugPayloads) != 1 {
		t.Fatalf("prompt_debug events = %d, want 1", len(debugPayloads))
	}
	debug := debugPayloads[0]
	if debug.AgentName != "孙悟空" || debug.Model != "test-model" {
		t.Fatalf("prompt debug identity = %+v", debug)
	}
	if len(engine.streamMessages) != 1 || !equalPromptMessages(debug.Messages, engine.streamMessages[0]) {
		t.Fatalf("debug messages = %#v, actual engine messages = %#v", debug.Messages, engine.streamMessages)
	}
	if debug.Generation.MaxTokens <= 0 || debug.Generation.MaxSentences <= 0 {
		t.Fatalf("debug generation budget = %+v, want positive limits", debug.Generation)
	}
}

func equalPromptMessages(a, b []map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i]["role"] != b[i]["role"] || a[i]["content"] != b[i]["content"] {
			return false
		}
	}
	return true
}

func TestInactiveGroupMemberStopsTurnBeforeEngine(t *testing.T) {
	svc, chats, engine, s := newGroupSvc(t, []string{"engine should not run"})
	for _, member := range chats.members[s.ID] {
		if member.AgentID == 2 {
			chats.agentByID[member.AgentID].Status = "inactive"
			for _, agent := range svc.agent.(*fakeAgentDao).agents {
				if agent.ID == member.AgentID {
					agent.Status = "inactive"
				}
			}
		}
	}
	var errorCode string
	var terminal bool
	var recovery string
	svc.RunGroupTurn(context.Background(), s.UUID, "诸位怎么看?", func(event string, payload any) {
		if event == "error" {
			data, _ := json.Marshal(payload)
			var p struct {
				ErrorCode string `json:"error_code"`
				Terminal  bool   `json:"terminal"`
				Recovery  string `json:"recovery"`
			}
			_ = json.Unmarshal(data, &p)
			errorCode = p.ErrorCode
			terminal = p.Terminal
			recovery = p.Recovery
		}
	})

	if errorCode != "service.chat.agent_inactive" {
		t.Fatalf("error code = %q, want service.chat.agent_inactive", errorCode)
	}
	if !terminal {
		t.Fatal("inactive preflight error must terminate a turn that cannot emit turn_done")
	}
	if recovery != "resend" {
		t.Fatalf("recovery = %q, pre-persist group failure must offer normal resend", recovery)
	}
	if engine.calls != 0 {
		t.Fatalf("engine calls = %d, want 0", engine.calls)
	}
	if len(chats.messages) != 0 {
		t.Fatalf("messages saved before authorization: %+v", chats.messages)
	}
}

func TestGroupUserSaveFailureOffersNormalResend(t *testing.T) {
	svc, chats, engine, session := newGroupSvc(t, nil)
	chats.saveErr = errors.ErrorServerInternalError("secret.group.user.save.failure")

	var errorPayload GroupSpeakerPayload
	svc.RunGroupTurn(context.Background(), session.UUID, "question that cannot be saved", func(event string, payload any) {
		if event != "error" {
			return
		}
		data, _ := json.Marshal(payload)
		_ = json.Unmarshal(data, &errorPayload)
	})

	if errorPayload.Recovery != StreamRecoveryResend {
		t.Fatalf("recovery = %q, failed group user persistence must offer normal resend", errorPayload.Recovery)
	}
	if strings.Contains(errorPayload.Content, "secret.group.user.save.failure") || engine.calls != 0 {
		t.Fatalf("save failure leaked details or called engine: payload=%+v engine=%d", errorPayload, engine.calls)
	}
	if len(chats.messages) != 0 {
		t.Fatalf("messages = %+v, failed user persistence must not leave a row", chats.messages)
	}
}

func TestInactiveGroupMemberDuringPersistedRetryNeverOffersNormalResend(t *testing.T) {
	svc, chats, _, session := newGroupSvc(t, nil)
	chats.messages = append(chats.messages, &model.ChatMessage{SessionID: session.ID, Role: "user", Content: "persisted group question"})
	for _, member := range chats.members[session.ID] {
		if member.AgentID == 2 {
			chats.agentByID[member.AgentID].Status = "inactive"
			for _, agent := range svc.agent.(*fakeAgentDao).agents {
				if agent.ID == member.AgentID {
					agent.Status = "inactive"
				}
			}
		}
	}

	var recovery string
	svc.RetryGroupTurn(context.Background(), session.UUID, "persisted group question", func(event string, payload any) {
		if event != "error" {
			return
		}
		data, _ := json.Marshal(payload)
		var decoded struct {
			Recovery string `json:"recovery"`
		}
		_ = json.Unmarshal(data, &decoded)
		recovery = decoded.Recovery
	})

	if recovery != "persisted_retry" {
		t.Fatalf("recovery = %q, persisted group retry must never downgrade to normal resend", recovery)
	}
}

func TestGroupSpeakerLifecycleEventsCarryExplicitIdentity(t *testing.T) {
	svc, chats, _, s := newGroupSvc(t, []string{"太上老君给出一段足够长且完整的回答", "[PASS]", "[PASS]", "[PASS]"})
	type identity struct {
		AgentID     string `json:"agent_id"`
		AgentName   string `json:"agent_name"`
		AgentAvatar string `json:"agent_avatar"`
	}
	seen := map[string][]identity{}
	// task 档(2 发言人,无表达欲桶过滤):首位老君发言,身份事件确定归属
	svc.RunGroupTurn(context.Background(), s.UUID, "帮我排查一个问题", func(event string, payload any) {
		if event != "speaker_start" && event != "chunk" && event != "speaker_done" {
			return
		}
		data, _ := json.Marshal(payload)
		var got identity
		_ = json.Unmarshal(data, &got)
		seen[event] = append(seen[event], got)
	})

	wantAgent := chats.agentByID[1]
	for _, event := range []string{"speaker_start", "chunk", "speaker_done"} {
		if len(seen[event]) == 0 {
			t.Fatalf("missing %s event", event)
		}
		for _, got := range seen[event] {
			if got.AgentID != wantAgent.UUID.String() || got.AgentName != wantAgent.Name || got.AgentAvatar != wantAgent.Avatar {
				t.Fatalf("%s identity = %+v, want id/name/avatar for current speaker", event, got)
			}
		}
	}
}

func TestGroupTransportInterruptionTerminatesTurnWithoutCorruptingCompletedSpeakers(t *testing.T) {
	svc, chats, _, session := newGroupSvc(t, nil)
	engineCalls := 0
	abruptEngine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		engineCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		switch engineCalls {
		case 1:
			fmt.Fprint(w, "data: {\"content\":\"first speaker completed reply\"}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
		case 2:
			fmt.Fprint(w, "data: {\"content\":\"second speaker partial reply\"}\n\n")
			// Close without [DONE] to reproduce an upstream transport interruption.
		default:
			fmt.Fprint(w, "data: {\"content\":\"unexpected later speaker\"}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
		}
	}))
	t.Cleanup(abruptEngine.Close)
	svc.engineBaseURL = engineendpoint.Static(abruptEngine.URL)

	type recordedEvent struct {
		name    string
		payload GroupSpeakerPayload
	}
	var events []recordedEvent
	// task 档(2 发言人):第 2 位发言时传输中断
	svc.RunGroupTurn(context.Background(), session.UUID, "帮我排查一个问题", func(event string, payload any) {
		data, _ := json.Marshal(payload)
		var speakerPayload GroupSpeakerPayload
		_ = json.Unmarshal(data, &speakerPayload)
		events = append(events, recordedEvent{name: event, payload: speakerPayload})
	})

	if engineCalls != 2 {
		t.Fatalf("engine calls = %d, want 2 after terminal transport interruption", engineCalls)
	}
	for _, event := range events {
		if event.name == "turn_done" {
			t.Fatalf("events = %+v, terminal transport interruption must not emit turn_done", events)
		}
	}
	wantInterruptedAgent := chats.agentByID[2]
	foundTerminalInterruption := false
	for _, event := range events {
		if event.name == "error" && event.payload.ErrorCode == "service.chat.stream_interrupted" {
			foundTerminalInterruption = event.payload.Terminal &&
				event.payload.Recovery == StreamRecoveryPersistedRetry &&
				event.payload.AgentID == wantInterruptedAgent.UUID.String() &&
				event.payload.AgentName == wantInterruptedAgent.Name &&
				event.payload.AgentAvatar == wantInterruptedAgent.Avatar
		}
	}
	if !foundTerminalInterruption {
		t.Fatalf("events = %+v, want identity-bearing terminal stream_interrupted error", events)
	}

	assistantMessages := 0
	for _, message := range chats.messages {
		if message.Role == "assistant" {
			assistantMessages++
			if message.AgentID == nil || *message.AgentID != 1 || message.Content != "first speaker completed reply" {
				t.Fatalf("persisted assistant = %+v, want only completed first speaker", message)
			}
		}
	}
	if assistantMessages != 1 {
		t.Fatalf("assistant messages = %d, want only the completed speaker persisted", assistantMessages)
	}
}

func TestGroupMemberStreamErrorIsNonterminalAndSanitized(t *testing.T) {
	svc, chats, _, session := newGroupSvc(t, nil)
	engineCalls := 0
	memberErrorEngine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		engineCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		if engineCalls == 1 {
			fmt.Fprint(w, "data: {\"error\":\"raw provider error api_key=must-not-leak\"}\n\n")
		} else {
			fmt.Fprint(w, "data: {\"content\":\"[PASS]\"}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(memberErrorEngine.Close)
	svc.engineBaseURL = engineendpoint.Static(memberErrorEngine.URL)

	var memberError GroupSpeakerPayload
	turnDone := false
	// task 档(2 发言人,无表达欲桶过滤):首位老君报错,次位孙悟空仍发言收束
	svc.RunGroupTurn(context.Background(), session.UUID, "帮我排查一个问题", func(event string, payload any) {
		if event == "turn_done" {
			turnDone = true
		}
		if event != "error" {
			return
		}
		data, _ := json.Marshal(payload)
		_ = json.Unmarshal(data, &memberError)
	})

	wantAgent := chats.agentByID[1]
	if memberError.Terminal {
		t.Fatalf("member error = %+v, ordinary member failures must remain nonterminal", memberError)
	}
	if memberError.ErrorCode != "service.chat.stream_unavailable" {
		t.Fatalf("member error code = %q, want stable stream_unavailable", memberError.ErrorCode)
	}
	if strings.Contains(memberError.Content, "raw provider") || strings.Contains(memberError.Content, "must-not-leak") {
		t.Fatalf("member error leaked raw upstream details: %+v", memberError)
	}
	if memberError.AgentID != wantAgent.UUID.String() || memberError.AgentName != wantAgent.Name || memberError.AgentAvatar != wantAgent.Avatar {
		t.Fatalf("member error identity = %+v, want first speaker identity", memberError)
	}
	if !turnDone {
		t.Fatal("nonterminal member error must allow the group turn to finish")
	}
}

func TestRetryGroupTurnReusesPersistedUserMessage(t *testing.T) {
	svc, chats, _, s := newGroupSvc(t, []string{"[PASS]", "[PASS]"})
	chats.messages = append(chats.messages, &model.ChatMessage{SessionID: s.ID, Role: "user", Content: "same question"})
	usersBefore := 1

	svc.RetryGroupTurn(context.Background(), s.UUID, "same question", func(string, any) {})

	usersAfter := 0
	for _, message := range chats.messages {
		if message.Role == "user" {
			usersAfter++
		}
	}
	if usersAfter != usersBefore {
		t.Fatalf("user message count = %d, want %d after group retry", usersAfter, usersBefore)
	}
}

func TestRetryGroupTurnUsesLatestUserBeyondFirstHistoryPage(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:"+filepath.Join(t.TempDir(), "latest-user.db")+"?_loc=Local&_fk=1"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.DaoAgent{}, &model.ChatSession{}, &model.ChatMessage{}, &model.SessionMember{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	previousDB := concretedao.DB
	concretedao.DB = db
	t.Cleanup(func() { concretedao.DB = previousDB })

	agentOne := model.DaoAgent{UUID: uuid.New(), Name: "Alpha", Status: "active", ModelName: "test-model", Proactivity: 50}
	agentTwo := model.DaoAgent{UUID: uuid.New(), Name: "Beta", Status: "active", ModelName: "test-model", Proactivity: 50}
	if err := db.Create(&agentOne).Error; err != nil {
		t.Fatalf("create first agent: %v", err)
	}
	if err := db.Create(&agentTwo).Error; err != nil {
		t.Fatalf("create second agent: %v", err)
	}
	session := model.ChatSession{UUID: uuid.New(), Type: model.SessionTypeGroup}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	members := []model.SessionMember{
		{SessionID: session.ID, AgentID: agentOne.ID, SortOrder: 0},
		{SessionID: session.ID, AgentID: agentTwo.ID, SortOrder: 1},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create members: %v", err)
	}

	createdAt := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	history := []model.ChatMessage{{
		SessionID: session.ID, Role: "user", Content: "stale old question", CreatedAt: createdAt,
	}}
	for i := 0; i < 20; i++ {
		agentID := agentOne.ID
		history = append(history, model.ChatMessage{
			SessionID: session.ID, Role: "assistant", Content: fmt.Sprintf("old reply %d", i),
			AgentID: &agentID, CreatedAt: createdAt.Add(time.Duration(i+1) * time.Second),
		})
	}
	history = append(history, model.ChatMessage{
		SessionID: session.ID, Role: "user", Content: "latest question", CreatedAt: createdAt.Add(21 * time.Second),
	})
	if err := db.Create(&history).Error; err != nil {
		t.Fatalf("create history: %v", err)
	}

	engine := newScriptEngine([]string{"[PASS]", "[PASS]"})
	t.Cleanup(engine.server.Close)
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{
		agentOne.UUID.String(): &agentOne,
		agentTwo.UUID.String(): &agentTwo,
	}}
	svc := New(concretedao.NewChatDao(), agents, fakePattern{}, availableCredentialResolver("test-model"), engine.server.URL)
	var retryError string
	turnDone := false
	svc.RetryGroupTurn(context.Background(), session.UUID, "latest question", func(event string, payload any) {
		if event == "turn_done" {
			turnDone = true
		}
		if event != "error" {
			return
		}
		data, _ := json.Marshal(payload)
		var decoded struct {
			ErrorCode string `json:"error_code"`
		}
		_ = json.Unmarshal(data, &decoded)
		retryError = decoded.ErrorCode
	})

	if retryError == "service.chat.retry_unavailable" || !turnDone {
		t.Fatalf("latest retry beyond page 1 failed: error=%q turn_done=%v", retryError, turnDone)
	}
	if len(engine.streamMessages) == 0 {
		t.Fatal("group retry did not send a model request")
	}
	for requestIndex, requestMessages := range engine.streamMessages {
		var contents []string
		for _, message := range requestMessages {
			contents = append(contents, message["content"])
		}
		joined := strings.Join(contents, "\n")
		if !strings.Contains(joined, "latest question") || strings.Contains(joined, "stale old question") {
			t.Fatalf("model request %d history = %q, want latest question without stale oldest page", requestIndex, joined)
		}
	}
	var userCount int64
	if err := db.Model(&model.ChatMessage{}).
		Where("session_id = ? AND role = ?", session.ID, "user").
		Count(&userCount).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 2 {
		t.Fatalf("user messages = %d, want 2 without retry duplicate", userCount)
	}

	staleRetryError := ""
	svc.RetryGroupTurn(context.Background(), session.UUID, "stale old question", func(event string, payload any) {
		if event != "error" {
			return
		}
		data, _ := json.Marshal(payload)
		var decoded struct {
			ErrorCode string `json:"error_code"`
		}
		_ = json.Unmarshal(data, &decoded)
		staleRetryError = decoded.ErrorCode
	})
	if staleRetryError != "service.chat.retry_unavailable" {
		t.Fatalf("stale oldest-page retry error = %q, want retry_unavailable", staleRetryError)
	}
}

func TestGroupTurnAllPass(t *testing.T) {
	svc, chats, engine, s := newGroupSvc(t, []string{"[PASS]", "[PASS]"})
	log := &eventLog{}
	// task 档(2 发言人)全员沉默:整轮沉默提前收束
	svc.RunGroupTurn(context.Background(), s.UUID, "帮我排查一个问题", log.emit)

	if countEvent(log, "speaker_start") != 0 {
		t.Fatal("全员沉默不应有 speaker_start")
	}
	if countEvent(log, "turn_done") != 1 {
		t.Fatal("必须有 turn_done")
	}
	// 整轮沉默提前 break:引擎仅 1 轮 × 2 人 = 2 次调用,零 assistant 落库
	if engine.calls != 2 {
		t.Fatalf("引擎调用次数=%d, 应=2", engine.calls)
	}
	for _, m := range chats.messages {
		if m.Role == "assistant" {
			t.Fatal("沉默不应落库 assistant 消息")
		}
	}
}

func TestGroupTurnOrdinaryQuestionAlwaysSelectsPrimarySpeaker(t *testing.T) {
	svc, chats, _, s := newGroupSvc(t, []string{"主道人应当回答这个问题"})
	chats.agentByID[1].Proactivity = 0
	chats.agentByID[2].Proactivity = 0
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "请回答这个问题", log.emit)
	if countEvent(log, "speaker_done") != 1 {
		t.Fatalf("ordinary question must have one primary speaker: %v", log.events)
	}
}

func TestGroupTurnConciseEnforcesSpeakerLimit(t *testing.T) {
	svc, chats, _, s := newGroupSvc(t, []string{"第一位回答", "第二位回答"})
	chats.agentByID[1].Proactivity = 100
	chats.agentByID[2].Proactivity = 100
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "简短回答", log.emit)
	if countEvent(log, "speaker_done") > 1 {
		t.Fatalf("concise turn must not exceed one speaker: %v", log.events)
	}
}

func TestGroupTurnMentionedPassIsVisibleFailure(t *testing.T) {
	svc, _, _, s := newGroupSvc(t, []string{"[PASS]"})
	var sawError bool
	svc.RunGroupTurn(context.Background(), s.UUID, "@孙悟空 请回答", func(event string, payload any) {
		if event == "error" {
			sawError = true
		}
	})
	if !sawError {
		t.Fatal("a mentioned speaker returning PASS must produce a visible failure")
	}
}

func TestGroupTurnMustAnswer(t *testing.T) {
	svc, chats, _, s := newGroupSvc(t, []string{"俺老孙来也", "[PASS]"})
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "@孙悟空 你怎么看?", log.emit)

	if countEvent(log, "speaker_start") != 1 {
		t.Fatalf("应只有孙悟空发言: %v", log.events)
	}
	var reply *model.ChatMessage
	for _, m := range chats.messages {
		if m.Role == "assistant" {
			reply = m
		}
	}
	if reply == nil || reply.AgentID == nil || *reply.AgentID != 2 {
		t.Fatalf("发言归属应为孙悟空(ID=2): %+v", reply)
	}
}

func TestGroupTurnChainMention(t *testing.T) {
	// round1: 老君发言并@悟空,悟空 PASS; round2: 悟空必答
	svc, _, engine, s := newGroupSvc(t, []string{
		"此事应问@孙悟空", "[PASS]",
		"俺来了", "[PASS]",
	})
	log := &eventLog{}
	// deep_dive 档(2 轮):round1 老君@悟空,round2 悟空必答
	svc.RunGroupTurn(context.Background(), s.UUID, "详细分析一下", log.emit)

	if countEvent(log, "speaker_start") != 2 {
		t.Fatalf("应有2人次发言: %v", log.events)
	}
	if engine.calls != 4 {
		t.Fatalf("应调引擎4次(2轮×2人), 实际%d", engine.calls)
	}
}

// Task 4:deep_dive 档 MaxRounds=2(§8.2)→ 2 轮 × 2 人 = 4 次调用;更多 replies 受上限约束
func TestGroupTurnMaxRoundsPerTurnPlan(t *testing.T) {
	svc, _, engine, s := newGroupSvc(t, []string{"甲1", "乙1", "甲2", "乙2"})
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "详细分析一下", log.emit)
	if engine.calls != 4 {
		t.Fatalf("普通讨论 MaxRounds=2: 引擎应调4次(2轮×2人), 实际%d", engine.calls)
	}
	if countEvent(log, "speaker_done") != 4 {
		t.Fatalf("应有4条发言: %v", log.events)
	}

	svc2, _, engine2, s2 := newGroupSvc(t, []string{"甲1", "乙1", "甲2", "乙2", "甲3", "乙3"})
	log2 := &eventLog{}
	svc2.RunGroupTurn(context.Background(), s2.UUID, "详细分析一下", log2.emit)
	if engine2.calls != 4 {
		t.Fatalf("更多 replies 仍受 MaxRounds=2 上限约束: 引擎应只调4次, 实际%d", engine2.calls)
	}
	if countEvent(log2, "speaker_done") != 4 {
		t.Fatalf("上限内应只有4条发言: %v", log2.events)
	}
}

// Task 9:每人一句(OneEach)→ MaxSpeakers=memberCount=2、MaxRounds=1,发言人次不超名额
func TestGroupTurnSpeakerCap(t *testing.T) {
	svc, _, engine, s := newGroupSvc(t, []string{"老君一言", "悟空一言", "老君二言", "悟空二言"})
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "大家每人一句", log.emit)
	if n := countEvent(log, "speaker_start"); n > 2 {
		t.Fatalf("speaker_start=%d, 应 ≤ MaxSpeakers=2: %v", n, log.events)
	}
	if engine.calls != 2 {
		t.Fatalf("每人一句只应 1 轮 × 2 人 = 2 次调用, 实际%d", engine.calls)
	}
	if countEvent(log, "speaker_done") != 2 {
		t.Fatalf("应有2条发言: %v", log.events)
	}
}

// Task 9:§9.2 去重——回复 ≥8 字符且与既有回复 bigram Jaccard ≥0.85 即收敛,
// 丢弃该发言(不开气泡不落库)且整回合不再启动下一名发言人
func TestGroupTurnSimilarityStopsNextSpeaker(t *testing.T) {
	duplicate := "这是一个非常独特的回答内容"
	svc, chats, engine, s := newGroupSvc(t, []string{duplicate, duplicate})
	log := &eventLog{}
	// task 档(2 发言人):第二位回复重复 → 去重收敛
	svc.RunGroupTurn(context.Background(), s.UUID, "帮我排查一个问题", log.emit)

	if countEvent(log, "speaker_done") != 1 {
		t.Fatalf("重复回复应只保留首位: %v", log.events)
	}
	if countEvent(log, "speaker_start") != 1 {
		t.Fatalf("去重后不应再开新发言人: %v", log.events)
	}
	// 重复判定发生在引擎调用之后(需 fullContent),第二位仍会被调用但被丢弃
	if engine.calls != 2 {
		t.Fatalf("引擎调用=%d, 期望2(首位发言+第二位判定去重后收敛)", engine.calls)
	}
	assistants := 0
	for _, m := range chats.messages {
		if m.Role == "assistant" {
			assistants++
		}
	}
	if assistants != 1 {
		t.Fatalf("落库 assistant=%d, 应=1", assistants)
	}
	if countEvent(log, "turn_done") != 1 {
		t.Fatal("去重收敛后仍应有 turn_done")
	}
}

// Task 9:<8 字符不拦截(§9.2):两条短回复都正常开气泡
func TestGroupTurnShortReplyNoDedup(t *testing.T) {
	svc, _, engine, s := newGroupSvc(t, []string{"短回复", "短回复"})
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "大家每人一句", log.emit)
	if countEvent(log, "speaker_done") != 2 {
		t.Fatalf("短回复不应去重: %v", log.events)
	}
	if engine.calls != 2 {
		t.Fatalf("每人一句=1轮×2人=2次调用, 实际%d", engine.calls)
	}
}

// Task 9:§9.4 被点名者失败必须显示失败,不得静默换人补位
func TestGroupTurnNamedSpeakerFailureShowsError(t *testing.T) {
	svc, chats, _, s := newGroupSvc(t, nil)
	engineCalls := 0
	failEngine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		engineCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"error\":\"model overloaded\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(failEngine.Close)
	svc.engineBaseURL = engineendpoint.Static(failEngine.URL)

	var namedError GroupSpeakerPayload
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "@孙悟空 你怎么看?", func(event string, payload any) {
		log.emit(event, payload)
		if event == "error" {
			data, _ := json.Marshal(payload)
			_ = json.Unmarshal(data, &namedError)
		}
	})

	want := chats.agentByID[2]
	if namedError.AgentID != want.UUID.String() || namedError.AgentName != want.Name {
		t.Fatalf("失败应带被点名者身份: %+v, want %s", namedError, want.Name)
	}
	if countEvent(log, "speaker_start") != 0 {
		t.Fatalf("被点名者失败不得有其他成员补位: %v", log.events)
	}
	if engineCalls != 1 {
		t.Fatalf("点名失败应即收束(不调其他成员), 引擎调用=%d", engineCalls)
	}
	if countEvent(log, "turn_done") != 1 {
		t.Fatalf("非传输中断失败仍应 turn_done: %v", log.events)
	}
}

// Task 9:表达欲桶(§7.1)按 会话|道人|用户消息|轮次 稳定哈希——同一输入两次结果一致
func TestGroupTurnVolunteerBucketDeterministic(t *testing.T) {
	replies := []string{"老君第一", "悟空第一", "老君第二", "悟空第二"}
	run := func() (calls int, done int) {
		svc, _, engine, s := newGroupSvc(t, replies)
		log := &eventLog{}
		svc.RunGroupTurn(context.Background(), s.UUID, "热烈讨论", log.emit)
		return engine.calls, countEvent(log, "speaker_done")
	}
	firstCalls, firstDone := run()
	secondCalls, secondDone := run()
	if firstCalls != secondCalls || firstDone != secondDone {
		t.Fatalf("同一输入两次运行的引擎调用/发言数不一致: %d/%d vs %d/%d",
			firstCalls, firstDone, secondCalls, secondDone)
	}
}

func TestGroupTurnAutoTitle(t *testing.T) {
	svc, chats, _, s := newGroupSvcWithCompletion(t, []string{"金丹妙不可言", "[PASS]"}, "丹道夜话")
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "什么是金丹?", log.emit)

	if countEvent(log, "title") != 1 {
		t.Fatalf("应触发一次 title 事件: %v", log.events)
	}
	if chats.sessions[s.UUID.String()].Title != "丹道夜话" {
		t.Fatalf("标题未落库: %q", chats.sessions[s.UUID.String()].Title)
	}
}

func TestGroupTurnAutoTitleSkipsWhenRenamed(t *testing.T) {
	svc, chats, _, s := newGroupSvcWithCompletion(t, []string{"金丹妙不可言", "[PASS]"}, "丹道夜话")
	chats.sessions[s.UUID.String()].Title = "用户改的名"
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "什么是金丹?", log.emit)
	if countEvent(log, "title") != 0 || chats.sessions[s.UUID.String()].Title != "用户改的名" {
		t.Fatal("已手动改名不应被覆盖")
	}
}

// ---------- P3 本地记忆挂载(§10.3/§10.4) ----------

// memoryPattern 带行为档案的语言模式(动态分区渲染前提:profile 非 nil)
type memoryPattern struct{}

func (memoryPattern) GetOrBuildPattern(ctx context.Context, agentID uint) (*model.LanguagePattern, errors.Error) {
	profile, err := behavior.ProfileToJSONMap(&behavior.DaoistBehaviorProfile{BasePersonality: "以丹道应世"})
	if err != nil {
		return nil, errors.New(errors.ErrorTypeServerInternalError, "test.profile", err.Error())
	}
	return &model.LanguagePattern{SystemPrompt: "你是道人。", BehaviorProfile: profile}, nil
}

// memoryStub iservice.Memory 测试替身:按道人返回检索片段,记录蒸馏 spec
type memoryStub struct {
	snippetsByAgent map[uint][]turnpolicy.MemorySnippet
	retrieveCalls   int
	distillSpecs    []service.DistillationSpec
}

func (m *memoryStub) ListMemories(context.Context, uint, string, bool) ([]*model.AgentMemory, errors.Error) {
	return nil, nil
}

func (m *memoryStub) CreateMemory(context.Context, uint, service.MemoryInput) (*model.AgentMemory, errors.Error) {
	return nil, nil
}

func (m *memoryStub) UpdateMemory(context.Context, uint, uuid.UUID, service.MemoryInput) (*model.AgentMemory, errors.Error) {
	return nil, nil
}

func (m *memoryStub) DeleteMemory(context.Context, uint, uuid.UUID) errors.Error { return nil }

func (m *memoryStub) ClearMemories(context.Context, uint) (int64, errors.Error) { return 0, nil }

func (m *memoryStub) Retrieve(_ context.Context, agentID uint, _ string) ([]turnpolicy.MemorySnippet, errors.Error) {
	m.retrieveCalls++
	return m.snippetsByAgent[agentID], nil
}

func (m *memoryStub) EnqueueDistillation(_ context.Context, spec service.DistillationSpec) bool {
	m.distillSpecs = append(m.distillSpecs, spec)
	return true
}

func (m *memoryStub) Close() {}

// P3:两位 memory_enabled 道人的回合各自注入本人记忆;蒸馏一次、Targets=2 个成功发言人
func TestGroupTurnMemoryInjectedPerSpeaker(t *testing.T) {
	svc, chats, engine, s := newGroupSvc(t, []string{"老君记着你爱围棋", "悟空记着你爱炼丹"})
	chats.agentByID[1].MemoryEnabled = true
	chats.agentByID[2].MemoryEnabled = true
	mem := &memoryStub{snippetsByAgent: map[uint][]turnpolicy.MemorySnippet{
		1: {{Kind: "fact", Content: "老君记忆:用户爱围棋"}},
		2: {{Kind: "fact", Content: "悟空记忆:用户爱炼丹"}},
	}}
	svc.Memory = mem
	svc.pattern = memoryPattern{}
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "大家每人一句", log.emit)

	if len(engine.streamMessages) != 2 {
		t.Fatalf("引擎调用=%d, 期望2(1轮×2人)", len(engine.streamMessages))
	}
	wants := []string{"老君记忆:用户爱围棋", "悟空记忆:用户爱炼丹"}
	for i, want := range wants {
		sys := engine.streamMessages[i][0]["content"]
		if !strings.Contains(sys, "【本地记忆事实】") || !strings.Contains(sys, want) {
			t.Fatalf("第%d次引擎调用 system 应含本人记忆 %q:\n%s", i, want, sys)
		}
	}
	if countEvent(log, "speaker_done") != 2 {
		t.Fatalf("speaker_done=%d, 期望2: %v", countEvent(log, "speaker_done"), log.events)
	}
	if len(mem.distillSpecs) != 1 {
		t.Fatalf("蒸馏 spec 数=%d, 期望1", len(mem.distillSpecs))
	}
	spec := mem.distillSpecs[0]
	if spec.SessionUUID != s.UUID.String() || spec.Model != "test-model" || spec.UserMessage != "大家每人一句" {
		t.Fatalf("spec=%+v, 期望 session/model/userMessage 匹配", spec)
	}
	if len(spec.Targets) != 2 {
		t.Fatalf("Targets=%d, 期望2个发言道人: %+v", len(spec.Targets), spec.Targets)
	}
	if spec.Targets[0].AgentID != 1 || spec.Targets[1].AgentID != 2 {
		t.Fatalf("Targets 顺序=%d/%d, 期望 1/2(按发言顺序)",
			spec.Targets[0].AgentID, spec.Targets[1].AgentID)
	}
	if len(spec.Targets[0].Messages) != 2 || spec.Targets[0].Messages[0].Role != "user" ||
		spec.Targets[0].Messages[1].Role != "assistant" {
		t.Fatalf("Target[0].Messages=%+v, 期望 [user, assistant]", spec.Targets[0].Messages)
	}
}

// P3:MemoryEnabled=false 成员不注入记忆、蒸馏目标排除该成员(门控)
func TestGroupTurnMemoryDisabledMemberExcluded(t *testing.T) {
	svc, chats, engine, s := newGroupSvc(t, []string{"老君记着你爱围棋", "悟空记着你爱炼丹"})
	chats.agentByID[1].MemoryEnabled = true
	chats.agentByID[2].MemoryEnabled = false
	mem := &memoryStub{snippetsByAgent: map[uint][]turnpolicy.MemorySnippet{
		1: {{Kind: "fact", Content: "老君记忆:用户爱围棋"}},
	}}
	svc.Memory = mem
	svc.pattern = memoryPattern{}
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "大家每人一句", log.emit)

	if len(engine.streamMessages) != 2 {
		t.Fatalf("引擎调用=%d, 期望2", len(engine.streamMessages))
	}
	if !strings.Contains(engine.streamMessages[0][0]["content"], "老君记忆:用户爱围棋") {
		t.Fatalf("启用成员应注入记忆:\n%s", engine.streamMessages[0][0]["content"])
	}
	disabledSys := engine.streamMessages[1][0]["content"]
	if !strings.Contains(disabledSys, "(无)") {
		t.Fatalf("禁用成员记忆分区应为(无):\n%s", disabledSys)
	}
	if strings.Contains(disabledSys, "用户爱围棋") {
		t.Fatalf("禁用成员不得注入记忆:\n%s", disabledSys)
	}
	if len(mem.distillSpecs) != 1 {
		t.Fatalf("蒸馏 spec 数=%d, 期望1", len(mem.distillSpecs))
	}
	if len(mem.distillSpecs[0].Targets) != 1 || mem.distillSpecs[0].Targets[0].AgentID != 1 {
		t.Fatalf("Targets=%+v, 期望仅含启用成员1", mem.distillSpecs[0].Targets)
	}
	if countEvent(log, "turn_done") != 1 {
		t.Fatalf("仍应 turn_done: %v", log.events)
	}
}

// ==================== Task 12:导演预算接入群聊 ====================
// 5 名道人的群聊 fixture:全高表达欲(确定性 volunteer),便于锁定调用次数边界
func newGroupSvcFive(t *testing.T, replies []string) (*Chat, *fakeChatDao, *scriptEngine, *model.ChatSession) {
	t.Helper()
	engine := newScriptEngine(replies)
	t.Cleanup(engine.server.Close)
	names := []struct {
		id   uint
		name string
		pro  int
	}{
		{1, "太上老君", 100},
		{2, "孙悟空", 100},
		{3, "二郎神", 100},
		{4, "哪吒", 100},
		{5, "土地公", 100},
	}
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{}}
	agentByID := map[uint]*model.DaoAgent{}
	uids := make([]uuid.UUID, 0, len(names))
	for _, n := range names {
		u := uuid.New()
		a := &model.DaoAgent{ID: n.id, UUID: u, Name: n.name, Avatar: "/avatar.png", Status: "active", Proactivity: n.pro, ModelName: "test-model"}
		agents.agents[u.String()] = a
		agentByID[n.id] = a
		uids = append(uids, u)
	}
	chats := &fakeChatDao{sessions: map[string]*model.ChatSession{}, members: map[uint][]*model.SessionMember{}, agentByID: agentByID}
	svc := New(chats, agents, fakePattern{}, availableCredentialResolver("test-model"), engine.server.URL)
	s, err := svc.CreateGroupSession(context.Background(), uids, "")
	if err != nil {
		t.Fatalf("建群: %v", err)
	}
	return svc, chats, engine, s
}

// Task 12:普通闲聊(casual 1 名额/1 轮)——5 名道人最多 1 次模型调用、1 轮
func TestGroupDirectorCasualSingleCallOneRound(t *testing.T) {
	svc, _, engine, s := newGroupSvcFive(t, []string{"就聊聊天气吧。今天确实不错。"})
	ev := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "今天天气真不错", ev.emit)

	if engine.calls != 1 {
		t.Fatalf("engine calls = %d, want 1(闲聊 1 名额)", engine.calls)
	}
	if countEvent(ev, "speaker_start") != 1 {
		t.Fatalf("speaker_start = %d, want 1(1 轮 1 人)", countEvent(ev, "speaker_start"))
	}
}

// Task 12:情绪倾诉(vent 1 名额/1 轮/2 句)——引擎回放 3 句,句数预算截到 2 句
func TestGroupDirectorVentAppliesSentenceBudget(t *testing.T) {
	svc, _, engine, s := newGroupSvcFive(t, []string{"好烦啊。我懂。还有第三句。"})
	var joined strings.Builder
	svc.RunGroupTurn(context.Background(), s.UUID, "我今天好烦，只想吐槽一下", func(event string, payload any) {
		if event != "chunk" {
			return
		}
		data, _ := json.Marshal(payload)
		var got struct {
			Content string `json:"content"`
		}
		_ = json.Unmarshal(data, &got)
		joined.WriteString(got.Content)
	})

	if engine.calls != 1 {
		t.Fatalf("engine calls = %d, want 1(倾诉 1 名额)", engine.calls)
	}
	if joined.String() != "好烦啊。我懂。" {
		t.Fatalf("输出 = %q, want 句数预算截断为 %q", joined.String(), "好烦啊。我懂。")
	}
}

// Task 12:任务请求(task 2 名额/1 轮)——5 名道人最多 2 次模型调用、1 轮
func TestGroupDirectorTaskLimitsTwoCallsOneRound(t *testing.T) {
	svc, _, engine, s := newGroupSvcFive(t, []string{"问题在配置。", "再查一下日志。", "[PASS]"})
	ev := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "帮我排查构建失败", ev.emit)

	if engine.calls > 2 {
		t.Fatalf("engine calls = %d, want ≤2(任务 2 名额)", engine.calls)
	}
	if n := countEvent(ev, "speaker_start"); n > 2 {
		t.Fatalf("speaker_start = %d, want ≤2(1 轮)", n)
	}
}

// Task 12:明确「大家每人一句」(OneEach 全员/1 句)——每名成员最多 1 次、每次 1 句
func TestGroupDirectorOneEachEveryMemberOnce(t *testing.T) {
	svc, _, engine, s := newGroupSvcFive(t, []string{"好。就这。", "行。可以。", "没错。很好。", "可以。没问题。", "不错。就它。"})
	sentenceCount := map[string]int{} // 按发言人统计句末「。」数
	var agentIDs []string
	svc.RunGroupTurn(context.Background(), s.UUID, "大家每人一句", func(event string, payload any) {
		if event != "chunk" {
			return
		}
		data, _ := json.Marshal(payload)
		var got struct {
			AgentID string `json:"agent_id"`
			Content string `json:"content"`
		}
		_ = json.Unmarshal(data, &got)
		if got.AgentID == "" {
			return
		}
		if _, seen := sentenceCount[got.AgentID]; !seen {
			agentIDs = append(agentIDs, got.AgentID)
		}
		sentenceCount[got.AgentID] += strings.Count(got.Content, "。")
	})

	if engine.calls != 5 {
		t.Fatalf("engine calls = %d, want 每人恰好 1 次(5)", engine.calls)
	}
	if len(agentIDs) != 5 {
		t.Fatalf("发言人数 = %d, want 5", len(agentIDs))
	}
	for _, id := range agentIDs {
		if sentenceCount[id] != 1 {
			t.Fatalf("发言人 %s 输出 %d 句, want 每人一句", id, sentenceCount[id])
		}
	}
}

// ---- Task 13:LangGraph 群聊公共契约映射(计划锚点) ----

// groupMemoryCall 一次 CreateMemory 调用的观测。
type groupMemoryCall struct {
	agentID uint
	in      service.MemoryInput
}

// groupMemory 记忆服务测试替身:记录 CreateMemory/蒸馏入队,检索返回预置片段。
type groupMemory struct {
	service.Memory
	byAgent      map[uint][]turnpolicy.MemorySnippet
	memoryCalls  []groupMemoryCall
	distillCalls []service.DistillationSpec
}

func (m *groupMemory) Retrieve(_ context.Context, agentID uint, _ string) ([]turnpolicy.MemorySnippet, errors.Error) {
	return m.byAgent[agentID], nil
}

func (m *groupMemory) CreateMemory(_ context.Context, agentID uint, in service.MemoryInput) (*model.AgentMemory, errors.Error) {
	m.memoryCalls = append(m.memoryCalls, groupMemoryCall{agentID: agentID, in: in})
	return &model.AgentMemory{AgentID: agentID, Kind: in.Kind, Content: in.Content}, nil
}

func (m *groupMemory) EnqueueDistillation(_ context.Context, spec service.DistillationSpec) bool {
	m.distillCalls = append(m.distillCalls, spec)
	return true
}

// groupRun 一轮 LangGraph 群聊的观测结果。
type groupRun struct {
	Events                     []string
	SpeakerReplies             []string // "道人名:内容"(按落库序)
	SpeakerStarts              []string // 发言开始的道人名(按事件序)
	TurnDoneReason             string
	TurnDoneSpoke              int
	TurnDoneFailed             int
	SupervisorCalls            int // 非编排流请求次数(0=除编排流外零模型调用)
	Chunks                     []string
	PromptDebug                []string
	TitleEvents                []string
	MemoryCalls                []groupMemoryCall
	DistillCount               int
	PersistedAssistantContents []string
	UserMessageCount           int
	RunStatus                  string
	Payloads                   map[string][]string
}

// newGroupOrchestrationServer 假 Python 引擎(群聊):编排路径按序吐事件,
// /chat/completions 路径回统一包络 JSON(自动命名调用),记录全部请求路径。
func newGroupOrchestrationServer(t *testing.T, paths *[]string, events []turnEvent) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		if strings.Contains(r.URL.Path, "/chat/completions") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"content":"自动标题"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, e := range events {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.name, e.payload)
			flusher.Flush()
		}
	}))
}

// langGraphGroupAgentIDs 锚点群确定性道人 UUID(脚本可直接引用)。
var langGraphGroupAgentIDs = []string{
	"11111111-1111-1111-1111-111111111111", // 张雪峰
	"22222222-2222-2222-2222-222222222222", // 李雪琴
	"33333333-3333-3333-3333-333333333333", // 贾玲
	"44444444-4444-4444-4444-444444444444", // 沈腾
}

// newLangGraphGroupFixture 计划锚点群:张雪峰/李雪琴/贾玲/沈腾(贾玲记忆关闭),
// 标题预置"初始标题"(默认轮不触发自动命名,SupervisorCalls 计数纯净)。
func newLangGraphGroupFixture(t *testing.T) (*Chat, *fakeChatDao, *groupMemory, *model.ChatSession) {
	t.Helper()
	uids := make([]uuid.UUID, 0, len(langGraphGroupAgentIDs))
	for _, raw := range langGraphGroupAgentIDs {
		uids = append(uids, uuid.MustParse(raw))
	}
	names := []string{"张雪峰", "李雪琴", "贾玲", "沈腾"}
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{}}
	byID := map[uint]*model.DaoAgent{}
	for i, uid := range uids {
		agent := &model.DaoAgent{ID: uint(i + 1), UUID: uid, Name: names[i], Status: "active", ModelName: "test-model", MemoryEnabled: i != 2}
		agents.agents[uid.String()] = agent
		byID[agent.ID] = agent
	}
	chats := &fakeChatDao{
		sessions:  map[string]*model.ChatSession{},
		members:   map[uint][]*model.SessionMember{},
		agentByID: byID,
	}
	svc := New(chats, agents, fakePattern{}, availableCredentialResolver("test-model"), "unused")
	session, err := svc.CreateGroupSession(context.Background(), uids, "初始标题")
	if err != nil {
		t.Fatalf("建群: %v", err)
	}
	mem := &groupMemory{byAgent: map[uint][]turnpolicy.MemorySnippet{}}
	svc.Memory = mem
	return svc, chats, mem, session
}

// runGroupFixture 计划锚点入口:默认命令驱动一轮群聊。
func runGroupFixture(t *testing.T, stream []turnEvent) groupRun {
	return runGroupFixtureCmd(t, stream, service.ConversationCommand{Content: "@全体成员 全体都有！报数！"}, nil)
}

// runGroupFixtureCmd 驱动一轮 RunConversation 群聊并汇总观测结果。
func runGroupFixtureCmd(t *testing.T, stream []turnEvent, cmd service.ConversationCommand, prepare func(*Chat, *fakeChatDao, *model.ChatSession)) groupRun {
	t.Helper()
	paths := []string{}
	server := newGroupOrchestrationServer(t, &paths, stream)
	t.Cleanup(server.Close)

	svc, chats, mem, session := newLangGraphGroupFixture(t)
	svc.engineBaseURL = engineendpoint.Static(server.URL)
	if prepare != nil {
		prepare(svc, chats, session)
	}
	cmd.SessionUID = session.UUID
	rec := newTurnEventRecorder()
	svc.RunConversation(context.Background(), cmd, rec.emit)

	out := groupRun{Events: rec.events, Payloads: rec.data}
	out.Chunks = rec.contents("chunk")
	out.PromptDebug = rec.data["prompt_debug"]
	out.TitleEvents = rec.data["title"]
	out.SupervisorCalls = len(paths) - 1 // 编排流之外的非流式调用(自动命名)
	for _, raw := range rec.data["speaker_start"] {
		var p GroupSpeakerPayload
		if json.Unmarshal([]byte(raw), &p) == nil {
			out.SpeakerStarts = append(out.SpeakerStarts, p.AgentName)
		}
	}
	if len(rec.data["turn_done"]) > 0 {
		var done GroupTurnDonePayload
		_ = json.Unmarshal([]byte(rec.data["turn_done"][0]), &done)
		out.TurnDoneReason, out.TurnDoneSpoke, out.TurnDoneFailed = done.Reason, done.Spoke, done.FailedSpeakers
	}
	for _, m := range chats.messages {
		switch m.Role {
		case "assistant":
			out.PersistedAssistantContents = append(out.PersistedAssistantContents, m.Content)
			if m.AgentID != nil {
				if a, ok := chats.agentByID[*m.AgentID]; ok {
					out.SpeakerReplies = append(out.SpeakerReplies, a.Name+":"+m.Content)
				}
			}
		case "user":
			out.UserMessageCount++
		}
	}
	out.MemoryCalls = mem.memoryCalls
	out.DistillCount = len(mem.distillCalls)
	if len(chats.runs) > 0 {
		out.RunStatus = chats.runs[len(chats.runs)-1].Status
	}
	return out
}

// groupRollCallScript 计划锚点事件脚本:deterministic 路由 + 四道人按序报数。
// 每道人 speaker_started→两个增量→assistant_final(文本=序数)。
func groupRollCallScript(t *testing.T) []turnEvent {
	t.Helper()
	stream := []turnEvent{{"plan_created", `{"source":"deterministic","plan":{"steps":4}}`}}
	for i, id := range langGraphGroupAgentIDs {
		stream = append(stream,
			turnEvent{"speaker_started", fmt.Sprintf(`{"agent_id":%q,"task":"报数"}`, id)},
			turnEvent{"assistant_delta", fmt.Sprintf(`{"agent_id":%q,"text":"%d."}`, id, i+1)},
			turnEvent{"assistant_delta", fmt.Sprintf(`{"agent_id":%q,"text":"到！"}`, id)},
			turnEvent{"assistant_final", fmt.Sprintf(`{"agent_id":%q,"reply_id":"reply-%d","text":"%d"}`, id, i+1, i+1)},
		)
	}
	stream = append(stream, turnEvent{"run_completed", `{}`})
	return stream
}

// 锚点(计划 Task 13 Step 1):deterministic 路由四道人按序报数,公共序 speaker_start→
// chunk→speaker_done,turn_done answered;标题已预置→编排外零模型调用。
func TestLangGraphGroupRollCallMapsFourOrderedReplies(t *testing.T) {
	result := runGroupFixture(t, groupRollCallScript(t))
	if got, want := strings.Join(result.SpeakerReplies, "|"), "张雪峰:1|李雪琴:2|贾玲:3|沈腾:4"; got != want {
		t.Fatalf("speaker replies = %q, want %q", got, want)
	}
	if result.SupervisorCalls != 0 {
		t.Fatalf("supervisor calls = %d, want 0 (标题已预置,编排外零模型调用)", result.SupervisorCalls)
	}
	if got, want := strings.Join(result.SpeakerStarts, "|"), "张雪峰|李雪琴|贾玲|沈腾"; got != want {
		t.Fatalf("speaker starts = %q, want %q", got, want)
	}
	if result.TurnDoneReason != "answered" || result.TurnDoneSpoke != 4 || result.TurnDoneFailed != 0 {
		t.Fatalf("turn_done = spoke %d reason %q failed %d, want 4/answered/0", result.TurnDoneSpoke, result.TurnDoneReason, result.TurnDoneFailed)
	}
	if result.RunStatus != model.ChatRunStatusCompleted {
		t.Fatalf("run status = %q, want completed", result.RunStatus)
	}
	if result.UserMessageCount != 1 {
		t.Fatalf("user messages = %d, want 1", result.UserMessageCount)
	}
}

// 单道人失败续跑:张雪峰 started+delta 后无终稿(dispatch 吞异常,无终态事件),
// Go 以 started-无-final 判定失败→非终态 error(可继续)→李雪琴照常完成,计 failed_speakers。
func TestLangGraphGroupFailedSpeakerContinuesTurn(t *testing.T) {
	z, li := langGraphGroupAgentIDs[0], langGraphGroupAgentIDs[1]
	stream := []turnEvent{
		{"speaker_started", fmt.Sprintf(`{"agent_id":%q,"task":"报数"}`, z)},
		{"assistant_delta", fmt.Sprintf(`{"agent_id":%q,"text":"我"}`, z)},
		{"speaker_started", fmt.Sprintf(`{"agent_id":%q,"task":"报数"}`, li)},
		{"assistant_final", fmt.Sprintf(`{"agent_id":%q,"reply_id":"reply-2","text":"2"}`, li)},
		{"run_completed", `{}`},
	}
	result := runGroupFixture(t, stream)
	if got, want := strings.Join(result.SpeakerReplies, "|"), "李雪琴:2"; got != want {
		t.Fatalf("speaker replies = %q, want %q", got, want)
	}
	continued := false
	for _, raw := range result.Payloads["error"] {
		var p GroupSpeakerPayload
		if json.Unmarshal([]byte(raw), &p) == nil && p.AgentName == "张雪峰" {
			continued = true
			if p.Terminal || p.ErrorCode != "service.chat.stream_unavailable" {
				t.Fatalf("failed speaker error = %+v, want 非终态 stream_unavailable", p)
			}
		}
	}
	if !continued {
		t.Fatalf("errors = %v, want 张雪峰 非终态失败提示", result.Payloads["error"])
	}
	if result.TurnDoneReason != "answered" || result.TurnDoneSpoke != 1 || result.TurnDoneFailed != 1 {
		t.Fatalf("turn_done = spoke %d reason %q failed %d, want 1/answered/1", result.TurnDoneSpoke, result.TurnDoneReason, result.TurnDoneFailed)
	}
	if result.RunStatus != model.ChatRunStatusCompleted {
		t.Fatalf("run status = %q, want completed (个别失败不拖垮整轮)", result.RunStatus)
	}
}

// prompt_debug 关联发言人:内部 agent_id→公共 PromptDebugPayload(身份/模型/原文 messages)。
func TestLangGraphGroupPromptDebugAssociatesSpeaker(t *testing.T) {
	z := langGraphGroupAgentIDs[0]
	stream := []turnEvent{
		{"prompt_debug", fmt.Sprintf(`{"agent_id":%q,"task":"报数","model_ref":{"name":"test-model"},"messages":[{"role":"system","content":"人设"}]}`, z)},
		{"speaker_started", fmt.Sprintf(`{"agent_id":%q,"task":"报数"}`, z)},
		{"assistant_final", fmt.Sprintf(`{"agent_id":%q,"reply_id":"reply-1","text":"1"}`, z)},
		{"run_completed", `{}`},
	}
	result := runGroupFixtureCmd(t, stream, service.ConversationCommand{Content: "报数", DebugPrompt: true}, nil)
	if len(result.PromptDebug) != 1 {
		t.Fatalf("prompt_debug payloads = %d, want 1", len(result.PromptDebug))
	}
	var p service.PromptDebugPayload
	if err := json.Unmarshal([]byte(result.PromptDebug[0]), &p); err != nil {
		t.Fatalf("prompt_debug payload: %v", err)
	}
	if p.AgentName != "张雪峰" || p.Model != "test-model" {
		t.Fatalf("prompt debug identity = %q/%q, want 张雪峰/test-model", p.AgentName, p.Model)
	}
	if len(p.Messages) != 1 || p.Messages[0]["content"] != "人设" {
		t.Fatalf("prompt debug messages = %#v, want 原文转发", p.Messages)
	}
}

// 客户端停止:run_interrupted→stopped,零持久化(不保留半截发言,设计 §10),无 turn_done。
func TestLangGraphGroupStopKeepsNoPartialPersistence(t *testing.T) {
	z := langGraphGroupAgentIDs[0]
	stream := []turnEvent{
		{"speaker_started", fmt.Sprintf(`{"agent_id":%q,"task":"报数"}`, z)},
		{"assistant_delta", fmt.Sprintf(`{"agent_id":%q,"text":"半"}`, z)},
		{"run_interrupted", `{"reason":"cancelled"}`},
	}
	result := runGroupFixture(t, stream)
	if len(result.PersistedAssistantContents) != 0 {
		t.Fatalf("partial persisted = %v, want none (中断不保留不完整发言)", result.PersistedAssistantContents)
	}
	if n := len(result.Events); n == 0 || result.Events[n-1] != "stopped" {
		t.Fatalf("events = %v, want stopped 收尾", result.Events)
	}
	if result.TurnDoneReason != "" {
		t.Fatalf("turn_done reason = %q, want 无 turn_done", result.TurnDoneReason)
	}
	if result.RunStatus != model.ChatRunStatusInterrupted {
		t.Fatalf("run status = %q, want interrupted", result.RunStatus)
	}
}

// run_error 终态:terminal error(persisted_retry),零持久化,run failed,无 turn_done。
func TestLangGraphGroupRunErrorTerminatesTurn(t *testing.T) {
	z := langGraphGroupAgentIDs[0]
	stream := []turnEvent{
		{"speaker_started", fmt.Sprintf(`{"agent_id":%q,"task":"报数"}`, z)},
		{"assistant_delta", fmt.Sprintf(`{"agent_id":%q,"text":"半"}`, z)},
		{"run_error", `{"error":"ProviderTimeout"}`},
	}
	result := runGroupFixture(t, stream)
	if n := len(result.Events); n == 0 || result.Events[n-1] != "error" {
		t.Fatalf("events = %v, want terminal error 收尾", result.Events)
	}
	var p GroupSpeakerPayload
	if err := json.Unmarshal([]byte(result.Payloads["error"][len(result.Payloads["error"])-1]), &p); err != nil {
		t.Fatalf("error payload: %v", err)
	}
	if !p.Terminal || p.ErrorCode != "service.chat.stream_unavailable" || p.Recovery != StreamRecoveryPersistedRetry {
		t.Fatalf("error payload = %+v, want terminal stream_unavailable+persisted_retry", p)
	}
	if len(result.PersistedAssistantContents) != 0 {
		t.Fatalf("persisted = %v, want none on terminal error", result.PersistedAssistantContents)
	}
	if result.TurnDoneReason != "" {
		t.Fatalf("turn_done reason = %q, want 无 turn_done", result.TurnDoneReason)
	}
	if result.RunStatus != model.ChatRunStatusFailed {
		t.Fatalf("run status = %q, want failed", result.RunStatus)
	}
}

// 兼容入口:langgraph 开关下 RunGroupTurn 委托 RunConversation(群路由),
// 既有公共契约不变(turn_done answered + 四有序发言);开关关走 legacy(既有 legacy 测试覆盖)。
func TestRunGroupTurnDelegatesUnderLangGraphEngine(t *testing.T) {
	previous := configuration.Configuration.OrchestrationEngine
	configuration.Configuration.OrchestrationEngine = "langgraph"
	t.Cleanup(func() { configuration.Configuration.OrchestrationEngine = previous })

	paths := []string{}
	server := newGroupOrchestrationServer(t, &paths, groupRollCallScript(t))
	t.Cleanup(server.Close)
	svc, chats, _, session := newLangGraphGroupFixture(t)
	svc.engineBaseURL = engineendpoint.Static(server.URL)

	rec := newTurnEventRecorder()
	svc.RunGroupTurn(context.Background(), session.UUID, "@全体成员 全体都有！报数！", rec.emit)

	replies := make([]string, 0, 4)
	for _, m := range chats.messages {
		if m.Role == "assistant" && m.AgentID != nil {
			if a, ok := chats.agentByID[*m.AgentID]; ok {
				replies = append(replies, a.Name+":"+m.Content)
			}
		}
	}
	if got, want := strings.Join(replies, "|"), "张雪峰:1|李雪琴:2|贾玲:3|沈腾:4"; got != want {
		t.Fatalf("speaker replies = %q, want %q (委托 LangGraph 权威编排)", got, want)
	}
	if len(paths)-1 != 0 {
		t.Fatalf("supervisor calls = %d, want 0", len(paths)-1)
	}
	if n := len(rec.events); n == 0 || rec.events[n-1] != "turn_done" {
		t.Fatalf("events tail = %v, want turn_done", rec.events[max(n-3, 0):])
	}
}

// memory_proposed 接线 e2e:有效提案经校验落库(episode),来源 run/会话可溯;
// 未知 agent 拒绝;同 proposal_id 幂等(只存一次)。
func TestLangGraphGroupMemoryProposalPersistsValidated(t *testing.T) {
	li := langGraphGroupAgentIDs[1]
	stream := []turnEvent{
		{"speaker_started", fmt.Sprintf(`{"agent_id":%q,"task":"报数"}`, li)},
		{"assistant_final", fmt.Sprintf(`{"agent_id":%q,"reply_id":"reply-1","text":"2"}`, li)},
		{"memory_proposed", fmt.Sprintf(`{"proposal_id":"p1","agent_id":%q,"content":"李雪琴辰时约饭"}`, li)},
		{"memory_proposed", fmt.Sprintf(`{"proposal_id":"p1","agent_id":%q,"content":"重复提案应被幂等拒绝"}`, li)},
		{"memory_proposed", `{"proposal_id":"p2","agent_id":"99999999-9999-9999-9999-999999999999","content":"未知道人应被拒绝"}`},
		{"run_completed", `{}`},
	}
	var mem *groupMemory
	var chats *fakeChatDao
	var session *model.ChatSession
	result := runGroupFixtureCmd(t, stream, service.ConversationCommand{Content: "报数"}, func(svc *Chat, c *fakeChatDao, s *model.ChatSession) {
		mem = svc.Memory.(*groupMemory)
		chats, session = c, s
	})
	if len(mem.memoryCalls) != 1 {
		t.Fatalf("memory calls = %d, want 1 (重复+未知拒绝)", len(mem.memoryCalls))
	}
	call := mem.memoryCalls[0]
	if call.agentID != 2 {
		t.Fatalf("memory agent id = %d, want 2 (李雪琴)", call.agentID)
	}
	if call.in.Kind != "episode" {
		t.Fatalf("memory kind = %q, want episode", call.in.Kind)
	}
	if call.in.Content != "李雪琴辰时约饭" {
		t.Fatalf("memory content = %q", call.in.Content)
	}
	if call.in.SourceSessionID != session.UUID.String() {
		t.Fatalf("source session = %q, want %q", call.in.SourceSessionID, session.UUID.String())
	}
	if call.in.SourceMessageID != chats.runs[0].UUID.String() {
		t.Fatalf("source run = %q, want 当前 run %q", call.in.SourceMessageID, chats.runs[0].UUID.String())
	}
	if result.TurnDoneReason != "answered" {
		t.Fatalf("turn_done reason = %q, want answered (提案失败不影响发言)", result.TurnDoneReason)
	}
}

// 首轮自动命名+蒸馏:标题空→title 事件+1 次命名补全;蒸馏仅 MemoryEnabled 终稿者(贾玲关)。
func TestLangGraphGroupFirstTurnTitlesAndDistillsMemory(t *testing.T) {
	var mem *groupMemory
	result := runGroupFixtureCmd(t, groupRollCallScript(t), service.ConversationCommand{Content: "@全体成员 全体都有！报数！"},
		func(svc *Chat, _ *fakeChatDao, session *model.ChatSession) {
			session.Title = ""
			mem = svc.Memory.(*groupMemory)
		})
	if len(result.TitleEvents) != 1 {
		t.Fatalf("title events = %v, want 1", result.TitleEvents)
	}
	var tp GroupTitlePayload
	if err := json.Unmarshal([]byte(result.TitleEvents[0]), &tp); err != nil || tp.Title != "自动标题" {
		t.Fatalf("title payload = %v (err=%v), want 自动标题", result.TitleEvents, err)
	}
	if result.SupervisorCalls != 1 {
		t.Fatalf("supervisor calls = %d, want 1 (仅自动命名补全)", result.SupervisorCalls)
	}
	if result.DistillCount != 1 {
		t.Fatalf("distill enqueue = %d, want 1 (单 spec 多 target)", result.DistillCount)
	}
	if len(mem.distillCalls) != 1 || len(mem.distillCalls[0].Targets) != 3 {
		t.Fatalf("distill targets = %#v, want 3 (MemoryEnabled 终稿者,贾玲关闭)", mem.distillCalls)
	}
	if mem.distillCalls[0].Model != "test-model" {
		t.Fatalf("distill model = %q, want 首位发言人模型 test-model", mem.distillCalls[0].Model)
	}
	titleIdx, doneIdx := -1, -1
	for i, e := range result.Events {
		if e == "title" && titleIdx == -1 {
			titleIdx = i
		}
		if e == "turn_done" {
			doneIdx = i
		}
	}
	if titleIdx == -1 || doneIdx == -1 || titleIdx > doneIdx {
		t.Fatalf("events = %v, want title 先于 turn_done", result.Events)
	}
}

// ---- Task 14:群聊公共事件 run_id 与群聊续跑(设计 §10/§11)----

// rawGroupFixture 直连锚点群 fixture(不经 runGroupFixture 包装),供 run_id/续跑测试取 dao 观测。
func rawGroupFixture(t *testing.T, server *httptest.Server) (*Chat, *fakeChatDao, *groupMemory, *model.ChatSession) {
	t.Helper()
	svc, chats, mem, session := newLangGraphGroupFixture(t)
	svc.engineBaseURL = engineendpoint.Static(server.URL)
	return svc, chats, mem, session
}

// assertGroupRunID 校验某事件全部载荷携带指定 run_id。
func assertGroupRunID(t *testing.T, rec *turnEventRecorder, event, runID string) {
	t.Helper()
	if len(rec.data[event]) == 0 {
		t.Fatalf("缺少 %s 事件, events = %v", event, rec.events)
	}
	for _, raw := range rec.data[event] {
		var p struct {
			RunID string `json:"run_id"`
		}
		_ = json.Unmarshal([]byte(raw), &p)
		if p.RunID != runID {
			t.Fatalf("%s run_id = %q, want %q", event, p.RunID, runID)
		}
	}
}

// TestLangGraphGroupCarriesRunIDOnControlEvents speaker_start/speaker_done/turn_done
// 携带 run_id;chunk 不携带(高频增量省带宽)。
func TestLangGraphGroupCarriesRunIDOnControlEvents(t *testing.T) {
	paths := []string{}
	server := newGroupOrchestrationServer(t, &paths, groupRollCallScript(t))
	svc, chats, _, session := rawGroupFixture(t, server)

	rec := newTurnEventRecorder()
	svc.RunConversation(context.Background(), service.ConversationCommand{SessionUID: session.UUID, Content: "@全体成员 全体都有！报数！"}, rec.emit)

	if len(chats.runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(chats.runs))
	}
	runID := chats.runs[0].UUID.String()
	for _, event := range []string{"speaker_start", "speaker_done", "turn_done"} {
		assertGroupRunID(t, rec, event, runID)
	}
	for _, raw := range rec.data["chunk"] {
		var p struct {
			RunID string `json:"run_id"`
		}
		_ = json.Unmarshal([]byte(raw), &p)
		if p.RunID != "" {
			t.Fatalf("chunk 携带 run_id = %q, want 空(高频增量省带宽)", p.RunID)
		}
	}
}

// TestLangGraphGroupStoppedCarriesRunID 中断收尾 stopped 携带 run_id(前端由此提供继续按钮)。
func TestLangGraphGroupStoppedCarriesRunID(t *testing.T) {
	stream := []turnEvent{
		{"speaker_started", `{"agent_id":"` + langGraphGroupAgentIDs[0] + `"}`},
		{"assistant_delta", `{"agent_id":"` + langGraphGroupAgentIDs[0] + `","text":"一"}`},
		{"run_interrupted", `{}`},
	}
	paths := []string{}
	server := newGroupOrchestrationServer(t, &paths, stream)
	svc, chats, _, session := rawGroupFixture(t, server)

	rec := newTurnEventRecorder()
	svc.RunConversation(context.Background(), service.ConversationCommand{SessionUID: session.UUID, Content: "@全体成员 全体都有！报数！"}, rec.emit)

	if len(chats.runs) != 1 || chats.runs[0].Status != model.ChatRunStatusInterrupted {
		t.Fatalf("run = %d 个, want interrupted", len(chats.runs))
	}
	assertGroupRunID(t, rec, "stopped", chats.runs[0].UUID.String())
}

// TestResumeGroupInterruptedRunReplaysSpeakersWithoutNewRun 群聊续跑:不落新用户消息、
// 不新建 run、不发 accepted;发言事件携带原 run_id;蒸馏按 resume 流内终稿落账。
func TestResumeGroupInterruptedRunReplaysSpeakersWithoutNewRun(t *testing.T) {
	agentID := func(i int) string { return langGraphGroupAgentIDs[i] }
	firstStream := []turnEvent{
		{"speaker_started", `{"agent_id":"` + agentID(0) + `"}`},
		{"assistant_delta", `{"agent_id":"` + agentID(0) + `","text":"只出现一半"}`},
		{"run_interrupted", `{}`},
	}
	paths := []string{}
	firstServer := newGroupOrchestrationServer(t, &paths, firstStream)
	svc, chats, mem, session := rawGroupFixture(t, firstServer)

	rec := newTurnEventRecorder()
	svc.RunConversation(context.Background(), service.ConversationCommand{SessionUID: session.UUID, Content: "@全体成员 报数"}, rec.emit)
	if len(chats.runs) != 1 || chats.runs[0].Status != model.ChatRunStatusInterrupted {
		t.Fatalf("首轮 run = %d 个, want interrupted", len(chats.runs))
	}
	runID := chats.runs[0].UUID
	userCount := 0
	for _, m := range chats.messages {
		if m.Role == "user" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("首轮用户消息 = %d, want 1", userCount)
	}

	resumeStream := []turnEvent{
		{"speaker_started", `{"agent_id":"` + agentID(0) + `"}`},
		{"assistant_final", `{"agent_id":"` + agentID(0) + `","reply_id":"r-1","text":"一"}`},
		{"speaker_started", `{"agent_id":"` + agentID(1) + `"}`},
		{"assistant_final", `{"agent_id":"` + agentID(1) + `","reply_id":"r-2","text":"二"}`},
		{"run_completed", `{}`},
	}
	resumeServer := newGroupOrchestrationServer(t, &paths, resumeStream)
	svc.engineBaseURL = engineendpoint.Static(resumeServer.URL)
	rec2 := newTurnEventRecorder()
	svc.RunConversationResume(context.Background(), runID, rec2.emit)

	for _, e := range rec2.events {
		if e == "accepted" {
			t.Fatalf("resume 不应发 accepted, events = %v", rec2.events)
		}
	}
	assertGroupRunID(t, rec2, "speaker_start", runID.String())
	assertGroupRunID(t, rec2, "speaker_done", runID.String())
	assertGroupRunID(t, rec2, "turn_done", runID.String())
	var done GroupTurnDonePayload
	_ = json.Unmarshal([]byte(rec2.data["turn_done"][0]), &done)
	if done.Spoke != 2 || done.Reason != "answered" {
		t.Fatalf("turn_done = %+v, want spoke=2 answered", done)
	}
	// 不重发用户消息、不新建 run、状态推进 completed
	userCount = 0
	for _, m := range chats.messages {
		if m.Role == "user" {
			userCount++
		}
	}
	if userCount != 1 || len(chats.runs) != 1 || chats.runs[0].Status != model.ChatRunStatusCompleted {
		t.Fatalf("续跑后 user/run/status = %d/%d/%q, want 1/1/completed", userCount, len(chats.runs), chats.runs[len(chats.runs)-1].Status)
	}
	// 两位发言者落库;张/李记忆开启 → 蒸馏 2 条
	saved := 0
	for _, m := range chats.messages {
		if m.Role == "assistant" {
			saved++
		}
	}
	if saved != 2 {
		t.Fatalf("落库 assistant = %d, want 2", saved)
	}
	if len(mem.distillCalls) != 1 || len(mem.distillCalls[0].Targets) != 2 {
		t.Fatalf("蒸馏 = %d 组, want 1 组 2 目标(张/李记忆开启)", len(mem.distillCalls))
	}
}
