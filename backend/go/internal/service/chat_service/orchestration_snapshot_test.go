package chat_service

// 编排执行快照构建(Task 11):BuildOrchestrationRequest 的行为锚点。
// 覆盖:成员序与 provider 元数据透传 / 单聊会话 / 默认 Supervisor 解析 /
// 记忆开关过滤 / 重试场景历史去重 / 模型停用失败。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ierr "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// snapshotMemory 仅实现 Retrieve;快照构建只读检索,其余接口方法不应被触达。
type snapshotMemory struct {
	service.Memory
	byAgent map[string][]service.MemorySnippet // 键=道人 UUID 文本
}

func (m snapshotMemory) Retrieve(_ context.Context, agentUID string, _ string) ([]service.MemorySnippet, ierr.Error) {
	return m.byAgent[agentUID], nil
}

// snapshotResolver deepseek 凭据(带 provider 元数据);键 "" 代理默认模型解析。
func snapshotResolver() fakeCredentialResolver {
	deepseek := &credential.ModelCredentials{
		Model: "deepseek", ProviderName: "DeepSeek 官方", ProviderType: "deepseek", APIKey: "test-api-key",
	}
	return fakeCredentialResolver{credentials: map[string]*credential.ModelCredentials{
		"deepseek": deepseek,
		"":         deepseek,
	}}
}

// buildSnapshotFixture zhang/li/jia/shen 四人群聊 + @zhang 用户轮 + 记忆库。
// jia 关闭记忆开关但记忆库有条目:验证快照按开关过滤而非按库存取。
func buildSnapshotFixture(t *testing.T) (*Chat, *fakeChatDao, map[string]*model.DaoAgent, *model.ChatSession, *model.ChatMessage) {
	t.Helper()
	names := []string{"zhang", "li", "jia", "shen"}
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{}}
	byName := map[string]*model.DaoAgent{}
	byID := map[string]*model.DaoAgent{} // 键=道人 UUID 文本
	for _, key := range names {
		a := &model.DaoAgent{
			DaoAgentID: uuid.New().String(), Name: "道人·" + key,
			Status: "active", ModelName: "deepseek", MemoryEnabled: true,
		}
		agents.agents[a.DaoAgentID] = a
		byName[key] = a
		byID[a.DaoAgentID] = a
	}
	byName["jia"].MemoryEnabled = false

	chats := &fakeChatDao{
		sessions:  map[string]*model.ChatSession{},
		members:   map[string][]*model.SessionMember{},
		agentByID: byID,
	}
	svc := New(chats, agents, fakePattern{}, snapshotResolver(), "")
	svc.Memory = snapshotMemory{byAgent: map[string][]service.MemorySnippet{
		byName["zhang"].DaoAgentID: {{Kind: "preference", Content: "zhang 记忆一"}},
		byName["jia"].DaoAgentID:   {{Kind: "preference", Content: "jia 记忆一"}},
	}}

	session := &model.ChatSession{ChatSessionID: uuid.New().String(), Type: model.SessionTypeGroup}
	for i, key := range names {
		chats.members[session.ChatSessionID] = append(chats.members[session.ChatSessionID], &model.SessionMember{
			SessionID: session.ChatSessionID, AgentID: byName[key].DaoAgentID, SortOrder: i,
		})
	}
	chats.sessions[session.ChatSessionID] = session

	userMessage := &model.ChatMessage{
		ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "user",
		Content:  "@zhang 请报数",
		Mentions: model.JSONMap{"agents": []string{byName["zhang"].DaoAgentID}, "user": true},
	}
	return svc, chats, byName, session, userMessage
}

func snapshotRun(session *model.ChatSession) *model.ChatRun {
	return &model.ChatRun{ChatRunID: uuid.New().String(), SessionID: session.ChatSessionID, Status: model.ChatRunStatusPending}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// 锚点:成员序=建群序、provider 元数据透传、用户轮 mentions 透传、投影零密钥。
func TestBuildOrchestrationRequestPreservesMemberOrderAndProviderType(t *testing.T) {
	svc, _, byName, session, userMessage := buildSnapshotFixture(t)

	req, err := svc.BuildOrchestrationRequest(context.Background(), session, userMessage, snapshotRun(session))
	if err != nil {
		t.Fatalf("BuildOrchestrationRequest: %v", err)
	}
	wantIDs := []string{
		byName["zhang"].DaoAgentID, byName["li"].DaoAgentID,
		byName["jia"].DaoAgentID, byName["shen"].DaoAgentID,
	}
	gotIDs := make([]string, 0, len(req.Agents))
	for _, a := range req.Agents {
		gotIDs = append(gotIDs, a.AgentID)
	}
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("member order = %v, want %v", gotIDs, wantIDs)
	}
	if req.Agents[0].ModelRef.ProviderType != "deepseek" {
		t.Fatalf("provider_type = %q, want deepseek", req.Agents[0].ModelRef.ProviderType)
	}
	if req.Agents[0].ModelRef.Name != "deepseek" {
		t.Fatalf("model name = %q, want deepseek", req.Agents[0].ModelRef.Name)
	}
	for _, agent := range req.Agents {
		if !strings.Contains(agent.SystemPrompt, "具体的人") {
			t.Fatalf("agent %s system prompt = %q, want person-oriented prompt", agent.Name, agent.SystemPrompt)
		}
		if len(agent.ExampleDialogues) != 1 || agent.ExampleDialogues[0].Assistant != "先看事实。" {
			t.Fatalf("agent %s examples = %+v, want selected voice example", agent.Name, agent.ExampleDialogues)
		}
	}
	if strings.Join(req.UserTurn.Mentions, ",") != byName["zhang"].DaoAgentID {
		t.Fatalf("mentions = %v, want [zhang]", req.UserTurn.Mentions)
	}
	projection := mustJSON(t, req.StateProjection())
	if strings.Contains(projection, "test-api-key") || strings.Contains(projection, "sk-test") {
		t.Fatalf("projection leaks credentials: %s", projection)
	}
}

// 单聊没有成员记录:参与者=会话归属道人,序=单元素。
func TestBuildOrchestrationRequestSingleChatUsesSessionAgent(t *testing.T) {
	svc, _, byName, _, _ := buildSnapshotFixture(t)
	agent := *byName["li"]
	agentUID := agent.DaoAgentID
	session := &model.ChatSession{ChatSessionID: uuid.New().String(), Type: model.SessionTypeSingle, AgentID: &agentUID, Agent: agent}
	userMessage := &model.ChatMessage{ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "user", Content: "你好"}

	req, err := svc.BuildOrchestrationRequest(context.Background(), session, userMessage, snapshotRun(session))
	if err != nil {
		t.Fatalf("BuildOrchestrationRequest: %v", err)
	}
	if len(req.Agents) == 0 || req.Agents[0].AgentID != agent.DaoAgentID {
		t.Fatalf("agents = %+v, want exactly [%s]", req.Agents, agent.DaoAgentID)
	}
	if req.SessionType != model.SessionTypeSingle {
		t.Fatalf("session_type = %q, want single", req.SessionType)
	}
}

// 群聊 Supervisor=当前默认模型(非人格);凭据按模型名与道人凭据并存。
func TestBuildOrchestrationRequestResolvesDefaultSupervisor(t *testing.T) {
	svc, _, _, session, userMessage := buildSnapshotFixture(t)

	req, err := svc.BuildOrchestrationRequest(context.Background(), session, userMessage, snapshotRun(session))
	if err != nil {
		t.Fatalf("BuildOrchestrationRequest: %v", err)
	}
	if req.DefaultModelRef == nil {
		t.Fatal("DefaultModelRef = nil, want default-model resolution")
	}
	if req.DefaultModelRef.Name != "deepseek" || req.DefaultModelRef.ProviderType != "deepseek" {
		t.Fatalf("default model ref = %+v, want deepseek", *req.DefaultModelRef)
	}
	if cred, ok := req.Credentials[req.DefaultModelRef.Name]; !ok || cred.APIKey == "" {
		t.Fatalf("credentials[%q] missing or keyless", req.DefaultModelRef.Name)
	}
}

// 记忆按开关过滤:启用者取检索结果,关闭者纵有库存也不进快照。
func TestBuildOrchestrationRequestFiltersMemoryByEnabled(t *testing.T) {
	svc, _, byName, session, userMessage := buildSnapshotFixture(t)

	req, err := svc.BuildOrchestrationRequest(context.Background(), session, userMessage, snapshotRun(session))
	if err != nil {
		t.Fatalf("BuildOrchestrationRequest: %v", err)
	}
	byAgent := map[string]int{}
	for _, m := range req.Memories {
		byAgent[m.AgentID]++
		if m.MemoryID == "" || m.Text == "" {
			t.Errorf("memory snapshot incomplete: %+v", m)
		}
	}
	if byAgent[byName["zhang"].DaoAgentID] != 1 {
		t.Fatalf("zhang memories = %d, want 1", byAgent[byName["zhang"].DaoAgentID])
	}
	if byAgent[byName["jia"].DaoAgentID] != 0 {
		t.Fatalf("jia memory_enabled=false but got %d snapshots", byAgent[byName["jia"].DaoAgentID])
	}
}

// 重试场景:本轮用户消息已在库(重跑 run)。历史=更早两轮(去掉 system 通知),不含本轮。
func TestBuildOrchestrationRequestExcludesUserTurnAndSystemFromHistory(t *testing.T) {
	svc, chats, byName, session, userMessage := buildSnapshotFixture(t)
	zhangUID := byName["zhang"].DaoAgentID
	olderUser := &model.ChatMessage{ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "user", Content: "早前的问题"}
	olderReply := &model.ChatMessage{ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "assistant", Content: "早前的回答", AgentID: &zhangUID}
	notification := &model.ChatMessage{ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "system", Content: "系统通知"}
	chats.messages = []*model.ChatMessage{olderUser, olderReply, notification, userMessage}

	req, err := svc.BuildOrchestrationRequest(context.Background(), session, userMessage, snapshotRun(session))
	if err != nil {
		t.Fatalf("BuildOrchestrationRequest: %v", err)
	}
	if req.UserTurn.MessageID != userMessage.ChatMessageID {
		t.Fatalf("user turn = %q, want %q", req.UserTurn.MessageID, userMessage.ChatMessageID)
	}
	if len(req.History) != 2 {
		t.Fatalf("history = %d messages, want 2", len(req.History))
	}
	if req.History[0].MessageID != olderUser.ChatMessageID || req.History[0].Role != "user" {
		t.Fatalf("history[0] = %+v, want older user turn", req.History[0])
	}
	if req.History[1].MessageID != olderReply.ChatMessageID || req.History[1].Role != "assistant" {
		t.Fatalf("history[1] = %+v, want older assistant reply", req.History[1])
	}
	if req.History[1].AgentID == nil || *req.History[1].AgentID != byName["zhang"].DaoAgentID {
		t.Fatalf("history[1].agent_id = %v, want zhang", req.History[1].AgentID)
	}
}

// 空快照上线契约:全新会话(库中只有本轮消息)的历史/记忆为空时必须上线为 []
// 而非 null——Python 必填 list/dict 字段拒收 null(2026-09-03 全新单聊会话首条
// 消息 8 连 422 的根因回归锚点);字段亦不得因 omitempty 静默缺席。
func TestBuildOrchestrationRequestWireNeverEmitsNullForEmptySnapshots(t *testing.T) {
	svc, chats, byName, _, _ := buildSnapshotFixture(t)
	agent := *byName["li"]
	agentUID := agent.DaoAgentID
	session := &model.ChatSession{ChatSessionID: uuid.New().String(), Type: model.SessionTypeSingle, AgentID: &agentUID, Agent: agent}
	userMessage := &model.ChatMessage{ChatMessageID: uuid.New().String(), SessionID: session.ChatSessionID, Role: "user", Content: "你好"}
	chats.messages = []*model.ChatMessage{userMessage}

	req, err := svc.BuildOrchestrationRequest(context.Background(), session, userMessage, snapshotRun(session))
	if err != nil {
		t.Fatalf("BuildOrchestrationRequest: %v", err)
	}
	raw := mustJSON(t, req)
	for _, field := range []string{"history_snapshot", "agent_snapshots", "memory_snapshots", "credentials"} {
		if strings.Contains(raw, `"`+field+`":null`) {
			t.Fatalf("%s marshals as null on the wire, want []/{}: %s", field, raw)
		}
	}
	for _, want := range []string{`"history_snapshot":[]`, `"memory_snapshots":[]`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("missing %s on the wire: %s", want, raw)
		}
	}
}

// 模型停用:快照构建失败,不产出可执行请求。
func TestBuildOrchestrationRequestFailsOnInactiveModel(t *testing.T) {
	svc, _, _, session, userMessage := buildSnapshotFixture(t)
	svc.creds = fakeCredentialResolver{errors: map[string]error{"deepseek": errors.New("该模型已停用，请更换模型")}}

	if _, err := svc.BuildOrchestrationRequest(context.Background(), session, userMessage, snapshotRun(session)); err == nil {
		t.Fatal("error = nil, want inactive-model failure")
	} else if !strings.Contains(err.Error(), "不可用") {
		t.Fatalf("error = %v, want model-unavailable signal", err)
	}
}
