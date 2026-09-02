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
	"github.com/alchemy-furnace/server/internal/service/turnpolicy"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// snapshotMemory 仅实现 Retrieve;快照构建只读检索,其余接口方法不应被触达。
type snapshotMemory struct {
	service.Memory
	byAgent map[uint][]turnpolicy.MemorySnippet
}

func (m snapshotMemory) Retrieve(_ context.Context, agentID uint, _ string) ([]turnpolicy.MemorySnippet, ierr.Error) {
	return m.byAgent[agentID], nil
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
	byID := map[uint]*model.DaoAgent{}
	for i, key := range names {
		a := &model.DaoAgent{
			ID: uint(i + 1), UUID: uuid.New(), Name: "道人·" + key,
			Status: "active", ModelName: "deepseek", MemoryEnabled: true,
		}
		agents.agents[a.UUID.String()] = a
		byName[key] = a
		byID[a.ID] = a
	}
	byName["jia"].MemoryEnabled = false

	chats := &fakeChatDao{
		sessions:  map[string]*model.ChatSession{},
		members:   map[uint][]*model.SessionMember{},
		agentByID: byID,
	}
	svc := New(chats, agents, fakePattern{}, snapshotResolver(), "")
	svc.Memory = snapshotMemory{byAgent: map[uint][]turnpolicy.MemorySnippet{
		byName["zhang"].ID: {{Kind: "preference", Content: "zhang 记忆一"}},
		byName["jia"].ID:   {{Kind: "preference", Content: "jia 记忆一"}},
	}}

	session := &model.ChatSession{ID: 1, UUID: uuid.New(), Type: model.SessionTypeGroup}
	for i, key := range names {
		chats.members[session.ID] = append(chats.members[session.ID], &model.SessionMember{
			SessionID: session.ID, AgentID: byName[key].ID, SortOrder: i,
		})
	}
	chats.sessions[session.UUID.String()] = session

	userMessage := &model.ChatMessage{
		ID: 99, UUID: uuid.New(), SessionID: session.ID, Role: "user",
		Content:  "@zhang 请报数",
		Mentions: model.JSONMap{"agents": []string{byName["zhang"].UUID.String()}, "user": true},
	}
	return svc, chats, byName, session, userMessage
}

func snapshotRun(session *model.ChatSession) *model.ChatRun {
	return &model.ChatRun{UUID: uuid.New(), SessionID: session.ID, Status: model.ChatRunStatusPending}
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
		byName["zhang"].UUID.String(), byName["li"].UUID.String(),
		byName["jia"].UUID.String(), byName["shen"].UUID.String(),
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
	if strings.Join(req.UserTurn.Mentions, ",") != byName["zhang"].UUID.String() {
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
	session := &model.ChatSession{ID: 2, UUID: uuid.New(), Type: model.SessionTypeSingle, AgentID: &agent.ID, Agent: agent}
	userMessage := &model.ChatMessage{ID: 1, UUID: uuid.New(), SessionID: session.ID, Role: "user", Content: "你好"}

	req, err := svc.BuildOrchestrationRequest(context.Background(), session, userMessage, snapshotRun(session))
	if err != nil {
		t.Fatalf("BuildOrchestrationRequest: %v", err)
	}
	if len(req.Agents) == 0 || req.Agents[0].AgentID != agent.UUID.String() {
		t.Fatalf("agents = %+v, want exactly [%s]", req.Agents, agent.UUID.String())
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
	if byAgent[byName["zhang"].UUID.String()] != 1 {
		t.Fatalf("zhang memories = %d, want 1", byAgent[byName["zhang"].UUID.String()])
	}
	if byAgent[byName["jia"].UUID.String()] != 0 {
		t.Fatalf("jia memory_enabled=false but got %d snapshots", byAgent[byName["jia"].UUID.String()])
	}
}

// 重试场景:本轮用户消息已在库(重跑 run)。历史=更早两轮(去掉 system 通知),不含本轮。
func TestBuildOrchestrationRequestExcludesUserTurnAndSystemFromHistory(t *testing.T) {
	svc, chats, byName, session, userMessage := buildSnapshotFixture(t)
	olderUser := &model.ChatMessage{ID: 10, UUID: uuid.New(), SessionID: session.ID, Role: "user", Content: "早前的问题"}
	olderReply := &model.ChatMessage{ID: 11, UUID: uuid.New(), SessionID: session.ID, Role: "assistant", Content: "早前的回答", AgentID: &byName["zhang"].ID}
	notification := &model.ChatMessage{ID: 12, UUID: uuid.New(), SessionID: session.ID, Role: "system", Content: "系统通知"}
	chats.messages = []*model.ChatMessage{olderUser, olderReply, notification, userMessage}

	req, err := svc.BuildOrchestrationRequest(context.Background(), session, userMessage, snapshotRun(session))
	if err != nil {
		t.Fatalf("BuildOrchestrationRequest: %v", err)
	}
	if req.UserTurn.MessageID != userMessage.UUID.String() {
		t.Fatalf("user turn = %q, want %q", req.UserTurn.MessageID, userMessage.UUID.String())
	}
	if len(req.History) != 2 {
		t.Fatalf("history = %d messages, want 2", len(req.History))
	}
	if req.History[0].MessageID != olderUser.UUID.String() || req.History[0].Role != "user" {
		t.Fatalf("history[0] = %+v, want older user turn", req.History[0])
	}
	if req.History[1].MessageID != olderReply.UUID.String() || req.History[1].Role != "assistant" {
		t.Fatalf("history[1] = %+v, want older assistant reply", req.History[1])
	}
	if req.History[1].AgentID == nil || *req.History[1].AgentID != byName["zhang"].UUID.String() {
		t.Fatalf("history[1].agent_id = %v, want zhang", req.History[1].AgentID)
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
