package chat_service

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// readinessAgentDao 分页+按 UUID 查询的道人 fake;其余方法经嵌入接口 nil-panic(意外调用即暴露)
type readinessAgentDao struct {
	dao.Agent
	agents    []*model.DaoAgent
	findErr   errors.Error
	findCalls []int
}

func (f *readinessAgentDao) FindAgents(_ context.Context, page, size int, status string) (int64, []*model.DaoAgent, errors.Error) {
	if f.findErr != nil {
		return 0, nil, f.findErr
	}
	if status != "active" {
		panic("readiness must only page active agents")
	}
	f.findCalls = append(f.findCalls, page)
	start := (page - 1) * size
	if start >= len(f.agents) {
		return int64(len(f.agents)), []*model.DaoAgent{}, nil
	}
	end := min(start+size, len(f.agents))
	out := make([]*model.DaoAgent, 0, end-start)
	for _, agent := range f.agents[start:end] {
		cp := *agent
		out = append(out, &cp)
	}
	return int64(len(f.agents)), out, nil
}

func (f *readinessAgentDao) TakeAgentByUUID(_ context.Context, uid uuid.UUID) (*model.DaoAgent, errors.Error) {
	for _, agent := range f.agents {
		if agent.UUID == uid {
			cp := *agent
			return &cp, nil
		}
	}
	return nil, errors.ErrorRecordNotFound("test.readiness.take_agent")
}

func newReadinessService(agentDao dao.Agent, resolver credential.Resolver) *Chat {
	return New(&fakeChatDao{
		sessions: map[string]*model.ChatSession{},
		members:  map[uint][]*model.SessionMember{},
	}, agentDao, nil, resolver, "http://unused")
}

func TestGetReadinessPagesAllActiveAgentsAndFiltersByFormalCredentials(t *testing.T) {
	agents := make([]*model.DaoAgent, 0, 101)
	credentials := map[string]*credential.ModelCredentials{}
	resolverErrors := map[string]error{}
	var wantReady []uuid.UUID
	for i := 0; i < 101; i++ {
		uid := uuid.New()
		modelName := fmt.Sprintf("model-%d", i)
		agents = append(agents, &model.DaoAgent{
			ID: uint(i + 1), UUID: uid, Name: fmt.Sprintf("道人%d", i), Status: "active", ModelName: modelName,
		})
		switch i % 4 {
		case 0:
			// 正式凭证可用(API key 非空,BaseURL 可空) -> ready
			credentials[modelName] = &credential.ModelCredentials{Model: modelName, APIKey: "must-not-leak"}
			wantReady = append(wantReady, uid)
		case 1:
			// 空 API key -> 不就绪
			credentials[modelName] = &credential.ModelCredentials{Model: modelName, BaseURL: "https://example.invalid"}
		case 2:
			// 停用模型/供应商导致的解析失败 -> 不就绪,且底层错误不得外泄
			resolverErrors[modelName] = stderrors.New("provider disabled; raw secret must-not-leak")
		default:
			// 无记录(nil 凭证) -> 不就绪
		}
	}
	agentDao := &readinessAgentDao{agents: agents}
	svc := newReadinessService(agentDao, fakeCredentialResolver{credentials: credentials, errors: resolverErrors})

	readiness, err := svc.GetReadiness(context.Background())

	if err != nil {
		t.Fatalf("GetReadiness() error = %v", err)
	}
	if readiness.ActiveAgentCount != 101 {
		t.Fatalf("GetReadiness() ActiveAgentCount = %d, want 101", readiness.ActiveAgentCount)
	}
	if len(agentDao.findCalls) < 2 {
		t.Fatalf("GetReadiness() only paged %v, want traversal beyond the first page", agentDao.findCalls)
	}
	if len(readiness.ReadyAgentIDs) != len(wantReady) {
		t.Fatalf("GetReadiness() ready %d agents, want %d", len(readiness.ReadyAgentIDs), len(wantReady))
	}
	for i, uid := range wantReady {
		if readiness.ReadyAgentIDs[i] != uid {
			t.Fatalf("GetReadiness() ReadyAgentIDs[%d] = %s, want %s (order preserved)", i, readiness.ReadyAgentIDs[i], uid)
		}
	}
}

func TestGetReadinessFailsOnlyWhenAgentListingFails(t *testing.T) {
	agentDao := &readinessAgentDao{findErr: errors.ErrorServerInternalError("dao.agent.find_agents")}
	svc := newReadinessService(agentDao, availableCredentialResolver("formal-model"))

	readiness, err := svc.GetReadiness(context.Background())

	if err == nil {
		t.Fatal("GetReadiness() error = nil, want listing failure")
	}
	if readiness != nil {
		t.Fatalf("GetReadiness() readiness = %+v, want nil", readiness)
	}
	if !err.IsType(errors.ErrorTypeServerInternalError) {
		t.Fatalf("GetReadiness() error type = %v, want 5xx internal", err)
	}
	if strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("GetReadiness() error leaks details: %q", err.Error())
	}
}

func TestGetReadinessWithoutAgentsOrResolver(t *testing.T) {
	// 零道人: 就绪名单为空且不出错
	emptyDao := &readinessAgentDao{}
	readiness, err := newReadinessService(emptyDao, availableCredentialResolver()).GetReadiness(context.Background())
	if err != nil {
		t.Fatalf("GetReadiness() with zero agents error = %v", err)
	}
	if readiness.ActiveAgentCount != 0 || len(readiness.ReadyAgentIDs) != 0 {
		t.Fatalf("GetReadiness() = %+v, want empty readiness", readiness)
	}

	// 有 active 道人但无凭证解析器: 全部不就绪,请求本身不失败
	uid := uuid.New()
	agentDao := &readinessAgentDao{agents: []*model.DaoAgent{
		{ID: 1, UUID: uid, Name: "无凭道人", Status: "active", ModelName: "formal-model"},
	}}
	readiness, err = newReadinessService(agentDao, nil).GetReadiness(context.Background())
	if err != nil {
		t.Fatalf("GetReadiness() with nil resolver error = %v", err)
	}
	if readiness.ActiveAgentCount != 1 || len(readiness.ReadyAgentIDs) != 0 {
		t.Fatalf("GetReadiness() = %+v, want 1 active agent with none ready", readiness)
	}
}
