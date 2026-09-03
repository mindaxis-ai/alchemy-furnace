package chat_service

// LangGraph 群聊轮测试(Task 13 起;自 group_orchestrator_test.go 拆出——
// legacy 群编排测试随 legacy 代码一并删除,权威编排测试保留)。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/engineendpoint"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

type fakePattern struct{}

func (fakePattern) GetOrBuildPattern(ctx context.Context, agentID uint) (*model.LanguagePattern, errors.Error) {
	return &model.LanguagePattern{SystemPrompt: "你是道人。"}, nil
}

type groupMemoryCall struct {
	agentID uint
	in      service.MemoryInput
}

// groupMemory 记忆服务测试替身:记录 CreateMemory/蒸馏入队,检索返回预置片段。
type groupMemory struct {
	service.Memory
	byAgent      map[uint][]service.MemorySnippet
	memoryCalls  []groupMemoryCall
	distillCalls []service.DistillationSpec
}

func (m *groupMemory) Retrieve(_ context.Context, agentID uint, _ string) ([]service.MemorySnippet, errors.Error) {
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
	mem := &groupMemory{byAgent: map[uint][]service.MemorySnippet{}}
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

// TestParseMentions(Task 15 自 group_prompt_test.go 迁入:ParseMentions 是权威
// 群编排的存活依赖,其原文件随 legacy 删除,覆盖随函数迁入本文件)
func TestParseMentions(t *testing.T) {
	members := []string{"太上老君", "孙悟空"}
	agents, user := ParseMentions("@孙悟空 你怎么看?@太上老君 呢", members)
	if len(agents) != 2 || agents[0] != "孙悟空" || agents[1] != "太上老君" || user {
		t.Fatalf("基本@解析失败: %v %v", agents, user)
	}
	// @用户 / @User 都识别(拉丁大小写不敏感)
	if _, user = ParseMentions("@用户 你觉得呢", members); !user {
		t.Fatal("@用户 未识别")
	}
	if _, user = ParseMentions("@user what", members); !user {
		t.Fatal("@user 大小写未识别")
	}
	// 非成员(含被踢者)丢弃;重复@去重保序
	agents, _ = ParseMentions("@红孩儿 @孙悟空 @孙悟空", members)
	if len(agents) != 1 || agents[0] != "孙悟空" {
		t.Fatalf("非成员过滤/去重失败: %v", agents)
	}
	// 中文标点截断
	agents, _ = ParseMentions("@太上老君,来聊聊", members)
	if len(agents) != 1 {
		t.Fatalf("标点截断失败: %v", agents)
	}
	// @全体成员:展开为全部成员(保序);别名与成员去重
	for _, trigger := range []string{"@全体成员", "@所有人", "@all", "@Everyone"} {
		agents, _ = ParseMentions(trigger+" 都来说说", members)
		if len(agents) != 2 || agents[0] != "太上老君" || agents[1] != "孙悟空" {
			t.Fatalf("@全体(%s)展开失败: %v", trigger, agents)
		}
	}
	// @全体成员 + 单点并存:去重后仍是全部成员
	agents, _ = ParseMentions("@全体成员 @孙悟空 加一个", members)
	if len(agents) != 2 {
		t.Fatalf("@全体+单点去重失败: %v", agents)
	}
}
