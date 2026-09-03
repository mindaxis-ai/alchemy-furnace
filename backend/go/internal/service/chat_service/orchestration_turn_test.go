package chat_service

// LangGraph 单聊流式迁移(Task 12):RunConversation 的公共事件契约与持久化语义锚点。
// 内部流由 httptest 假 Python 引擎注入;观测公共事件序、落库行、run 状态与请求投影。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/engineendpoint"
	ierr "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/turnpolicy"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// turnEvent 内部流测试事件(name + data 行 JSON 原文)。
type turnEvent struct {
	name    string
	payload string
}

// newTurnStreamServer 假 Python 引擎:按序吐内部事件并只捕获首个请求体
// (编排 run 必然先于标题补全调用,后者不得覆盖观测)。
func newTurnStreamServer(t *testing.T, captured *[]byte, events []turnEvent) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if captured != nil && len(*captured) == 0 {
			*captured = body
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, e := range events {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.name, e.payload)
			flusher.Flush()
		}
	}))
}

// turnEventRecorder 收集公共事件名与载荷原文(emit 即 RunConversation 的公共出口)。
type turnEventRecorder struct {
	events []string
	data   map[string][]string
}

func newTurnEventRecorder() *turnEventRecorder {
	return &turnEventRecorder{data: map[string][]string{}}
}

func (r *turnEventRecorder) emit(event string, payload any) {
	r.events = append(r.events, event)
	raw, err := json.Marshal(payload)
	if err != nil {
		r.data[event] = append(r.data[event], "<marshal-error>")
		return
	}
	r.data[event] = append(r.data[event], string(raw))
}

// contents 取某事件的 content 字段序列(chunk 用)。
func (r *turnEventRecorder) contents(event string) []string {
	out := make([]string, 0, len(r.data[event]))
	for _, raw := range r.data[event] {
		var p struct {
			Content string `json:"content"`
		}
		_ = json.Unmarshal([]byte(raw), &p)
		out = append(out, p.Content)
	}
	return out
}

// turnMemory 记录蒸馏入队调用;检索按 agentID 返回预置片段。
type turnMemory struct {
	service.Memory
	byAgent      map[uint][]turnpolicy.MemorySnippet
	enqueueCalls []service.DistillationSpec
}

func (m *turnMemory) Retrieve(_ context.Context, agentID uint, _ string) ([]turnpolicy.MemorySnippet, ierr.Error) {
	return m.byAgent[agentID], nil
}

func (m *turnMemory) EnqueueDistillation(_ context.Context, spec service.DistillationSpec) bool {
	m.enqueueCalls = append(m.enqueueCalls, spec)
	return true
}

// turnRun 一轮 RunConversation 的观测结果。
type turnRun struct {
	Events                     []string
	Chunks                     []string
	PromptDebug                []string
	Payloads                   map[string][]string
	PersistedAssistantContents []string
	ReplyInsertCount           int
	UserMessageCount           int
	EnqueueCount               int
	RunCount                   int
	RunStatus                  string
	UserTurnMessageID          string
	CapturedRequest            []byte
}

// newTurnFixture 单聊会话(道人 zhang)挂在快照 fixture 之上;记忆替换为可记录蒸馏的 turnMemory。
func newTurnFixture(t *testing.T) (*Chat, *fakeChatDao, *turnMemory, *model.ChatSession) {
	t.Helper()
	svc, chats, byName, _, _ := buildSnapshotFixture(t)
	agent := *byName["zhang"]
	session := &model.ChatSession{ID: 2, UUID: uuid.New(), Type: model.SessionTypeSingle, AgentID: &agent.ID, Agent: agent}
	chats.sessions[session.UUID.String()] = session
	mem := &turnMemory{byAgent: map[uint][]turnpolicy.MemorySnippet{}}
	svc.Memory = mem
	return svc, chats, mem, session
}

// runLangGraphSingle 计划锚点入口:默认命令驱动一轮。
func runLangGraphSingle(t *testing.T, stream []turnEvent) turnRun {
	return runLangGraphSingleCmd(t, stream, service.ConversationCommand{Content: "请回答"}, nil)
}

// runLangGraphSingleCmd 驱动一轮 RunConversation 并汇总观测结果。
func runLangGraphSingleCmd(t *testing.T, stream []turnEvent, cmd service.ConversationCommand, prepare func(*Chat, *fakeChatDao)) turnRun {
	t.Helper()
	var captured []byte
	server := newTurnStreamServer(t, &captured, stream)
	t.Cleanup(server.Close)

	svc, chats, mem, session := newTurnFixture(t)
	svc.engineBaseURL = engineendpoint.Static(server.URL)
	if prepare != nil {
		prepare(svc, chats)
	}
	cmd.SessionUID = session.UUID
	rec := newTurnEventRecorder()
	svc.RunConversation(context.Background(), cmd, rec.emit)

	out := turnRun{Events: rec.events, Payloads: rec.data, CapturedRequest: captured}
	out.Chunks = rec.contents("chunk")
	out.PromptDebug = rec.data["prompt_debug"]
	for _, m := range chats.messages {
		switch m.Role {
		case "assistant":
			out.PersistedAssistantContents = append(out.PersistedAssistantContents, m.Content)
			if m.ReplyID != nil {
				out.ReplyInsertCount++
			}
		case "user":
			out.UserMessageCount++
		}
	}
	out.EnqueueCount = len(mem.enqueueCalls)
	out.RunCount = len(chats.runs)
	if out.RunCount > 0 {
		out.RunStatus = chats.runs[out.RunCount-1].Status
	}
	if len(captured) > 0 {
		var wire struct {
			UserTurn struct {
				MessageID string `json:"message_id"`
			} `json:"user_turn"`
		}
		if json.Unmarshal(captured, &wire) == nil {
			out.UserTurnMessageID = wire.UserTurn.MessageID
		}
	}
	return out
}

// 锚点(计划 Task 12 Step 1):只持久化 assistant_final,增量不落库。
func TestLangGraphSinglePersistsOnlyAssistantFinal(t *testing.T) {
	stream := []turnEvent{
		{"speaker_started", `{"agent_id":"a1","task":"请回答"}`},
		{"assistant_delta", `{"agent_id":"a1","text":"半"}`},
		{"assistant_delta", `{"agent_id":"a1","text":"截"}`},
		{"assistant_final", `{"agent_id":"a1","reply_id":"reply-1","text":"完整回答"}`},
		{"run_completed", `{}`},
	}
	result := runLangGraphSingle(t, stream)
	if strings.Join(result.PersistedAssistantContents, "|") != "完整回答" {
		t.Fatalf("persisted = %v, want [完整回答]", result.PersistedAssistantContents)
	}
	if result.ReplyInsertCount != 1 {
		t.Fatalf("reply insert count = %d, want 1", result.ReplyInsertCount)
	}
	if result.RunStatus != model.ChatRunStatusCompleted {
		t.Fatalf("run status = %q, want completed", result.RunStatus)
	}
	if result.UserMessageCount != 1 {
		t.Fatalf("user messages = %d, want 1", result.UserMessageCount)
	}
}

// 增量映射公共 chunk;单聊不透传 speaker_started;公共序 accepted→chunk…→done;回复后入队蒸馏。
func TestLangGraphSingleMapsDeltasToPublicChunks(t *testing.T) {
	stream := []turnEvent{
		{"assistant_delta", `{"text":"半"}`},
		{"assistant_delta", `{"text":"截"}`},
		{"assistant_final", `{"reply_id":"r1","text":"完整回答"}`},
		{"run_completed", `{}`},
	}
	result := runLangGraphSingle(t, stream)
	if strings.Join(result.Events, ",") != "accepted,chunk,chunk,done" {
		t.Fatalf("events = %v, want accepted,chunk,chunk,done", result.Events)
	}
	if strings.Join(result.Chunks, "|") != "半|截" {
		t.Fatalf("chunks = %v, want [半 截]", result.Chunks)
	}
	if result.EnqueueCount != 1 {
		t.Fatalf("distillation enqueue calls = %d, want 1 on success", result.EnqueueCount)
	}
}

// prompt_debug:仅 DebugPrompt 开启(请求体 debug_enabled=true)并原样转发内部事件。
func TestLangGraphSingleForwardsPromptDebugWhenEnabled(t *testing.T) {
	stream := []turnEvent{
		{"prompt_debug", `{"agent_id":"a1","messages":[{"role":"system","content":"人设"}]}`},
		{"assistant_final", `{"reply_id":"r1","text":"答"}`},
		{"run_completed", `{}`},
	}
	result := runLangGraphSingleCmd(t, stream, service.ConversationCommand{Content: "请回答", DebugPrompt: true}, nil)
	if len(result.PromptDebug) != 1 || !strings.Contains(result.PromptDebug[0], "人设") {
		t.Fatalf("prompt_debug payloads = %v, want 原文转发", result.PromptDebug)
	}
	if !strings.Contains(string(result.CapturedRequest), `"debug_enabled":true`) {
		t.Fatalf("request body must carry debug_enabled=true, got %s", result.CapturedRequest)
	}
}

// 默认关闭:请求体 debug_enabled=false,无 prompt_debug 转发。
func TestLangGraphSinglePromptDebugDisabledByDefault(t *testing.T) {
	stream := []turnEvent{
		{"assistant_final", `{"reply_id":"r1","text":"答"}`},
		{"run_completed", `{}`},
	}
	result := runLangGraphSingle(t, stream)
	if len(result.PromptDebug) != 0 {
		t.Fatalf("prompt_debug payloads = %v, want none", result.PromptDebug)
	}
	if strings.Contains(string(result.CapturedRequest), `"debug_enabled":true`) {
		t.Fatalf("request must not enable debug: %s", result.CapturedRequest)
	}
}

// 终态错误:run_error → 公共 error(terminal+persisted_retry),增量不落库,run failed。
func TestLangGraphSingleTerminalErrorDoesNotPersist(t *testing.T) {
	stream := []turnEvent{
		{"assistant_delta", `{"text":"部分"}`},
		{"run_error", `{"error":"ProviderTimeout"}`},
	}
	result := runLangGraphSingle(t, stream)
	if len(result.PersistedAssistantContents) != 0 {
		t.Fatalf("persisted = %v, want none on terminal error", result.PersistedAssistantContents)
	}
	if n := len(result.Events); n == 0 || result.Events[n-1] != "error" {
		t.Fatalf("events = %v, want terminal error", result.Events)
	}
	var p ConversationEventPayload
	if err := json.Unmarshal([]byte(result.Payloads["error"][0]), &p); err != nil {
		t.Fatalf("error payload: %v", err)
	}
	if !p.Terminal || p.ErrorCode != "service.chat.stream_unavailable" {
		t.Fatalf("error payload = %+v, want terminal stream_unavailable", p)
	}
	if result.RunStatus != model.ChatRunStatusFailed {
		t.Fatalf("run status = %q, want failed", result.RunStatus)
	}
}

// 中断(run_interrupted):不保留部分回复,公共 stopped 收尾,run 标记 interrupted(可续跑)。
func TestLangGraphSingleCancelKeepsNoPartialPersistence(t *testing.T) {
	stream := []turnEvent{
		{"assistant_delta", `{"text":"半"}`},
		{"run_interrupted", `{"reason":"cancelled"}`},
	}
	result := runLangGraphSingle(t, stream)
	if strings.Join(result.Events, ",") != "accepted,chunk,stopped" {
		t.Fatalf("events = %v, want accepted,chunk,stopped", result.Events)
	}
	if len(result.PersistedAssistantContents) != 0 {
		t.Fatalf("partial persisted = %v, want none", result.PersistedAssistantContents)
	}
	if result.RunStatus != model.ChatRunStatusInterrupted {
		t.Fatalf("run status = %q, want interrupted", result.RunStatus)
	}
}

// 重试幂等:复用最近同内容用户消息(不重复落库),请求 user_turn 指向既有消息。
func TestLangGraphSingleRetryReusesPersistedUserMessage(t *testing.T) {
	var existing *model.ChatMessage
	stream := []turnEvent{
		{"assistant_final", `{"reply_id":"r1","text":"答"}`},
		{"run_completed", `{}`},
	}
	result := runLangGraphSingleCmd(t, stream, service.ConversationCommand{Content: "请回答", Retry: true},
		func(_ *Chat, chats *fakeChatDao) {
			existing = &model.ChatMessage{SessionID: 2, Role: "user", Content: "请回答"}
			if err := chats.SaveMessage(context.Background(), existing); err != nil {
				t.Fatalf("seed user message: %v", err)
			}
		})
	if result.UserMessageCount != 1 {
		t.Fatalf("user messages = %d, want 1 (reused, not duplicated)", result.UserMessageCount)
	}
	if result.UserTurnMessageID != existing.UUID.String() {
		t.Fatalf("user_turn.message_id = %q, want %q", result.UserTurnMessageID, existing.UUID.String())
	}
	if result.RunStatus != model.ChatRunStatusCompleted {
		t.Fatalf("run status = %q, want completed", result.RunStatus)
	}
}

// 重试内容不符:明确报错 retry_unavailable,不建 run、不重复落库。
func TestLangGraphSingleRetryMismatchFailsFast(t *testing.T) {
	stream := []turnEvent{{"run_completed", `{}`}}
	result := runLangGraphSingleCmd(t, stream, service.ConversationCommand{Content: "别的内容", Retry: true},
		func(_ *Chat, chats *fakeChatDao) {
			_ = chats.SaveMessage(context.Background(), &model.ChatMessage{SessionID: 2, Role: "user", Content: "请回答"})
		})
	if n := len(result.Events); n == 0 || result.Events[n-1] != "error" {
		t.Fatalf("events = %v, want error", result.Events)
	}
	var p ConversationEventPayload
	_ = json.Unmarshal([]byte(result.Payloads["error"][0]), &p)
	if p.ErrorCode != "service.chat.retry_unavailable" {
		t.Fatalf("error_code = %q, want retry_unavailable", p.ErrorCode)
	}
	if result.RunCount != 0 {
		t.Fatalf("runs = %d, want 0 (fail fast before run creation)", result.RunCount)
	}
	if result.UserMessageCount != 1 {
		t.Fatalf("user messages = %d, want 1", result.UserMessageCount)
	}
}

// 同 reply_id 重复 assistant_final:SaveFinalReplyOnce 幂等,单行。
func TestLangGraphSingleDuplicateFinalIsIdempotent(t *testing.T) {
	stream := []turnEvent{
		{"assistant_final", `{"reply_id":"r1","text":"答"}`},
		{"assistant_final", `{"reply_id":"r1","text":"答"}`},
		{"run_completed", `{}`},
	}
	result := runLangGraphSingle(t, stream)
	if result.ReplyInsertCount != 1 || strings.Join(result.PersistedAssistantContents, "|") != "答" {
		t.Fatalf("persisted = %v inserts = %d, want single row", result.PersistedAssistantContents, result.ReplyInsertCount)
	}
}

// 无终稿即完成:语义错误 empty_response+persisted_retry,run failed。
func TestLangGraphSingleEmptyFinalErrorsWithPersistedRetry(t *testing.T) {
	stream := []turnEvent{{"run_completed", `{}`}}
	result := runLangGraphSingle(t, stream)
	if n := len(result.Events); n == 0 || result.Events[n-1] != "error" {
		t.Fatalf("events = %v, want error", result.Events)
	}
	var p ConversationEventPayload
	_ = json.Unmarshal([]byte(result.Payloads["error"][0]), &p)
	if p.ErrorCode != "service.chat.empty_response" || p.Recovery != StreamRecoveryPersistedRetry {
		t.Fatalf("error payload = %+v, want empty_response+persisted_retry", p)
	}
	if result.RunStatus != model.ChatRunStatusFailed {
		t.Fatalf("run status = %q, want failed", result.RunStatus)
	}
}
