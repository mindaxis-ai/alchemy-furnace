package chat_service

// persistMemoryProposal 校验规则锚点(计划 Task 13):目标 agent ∈ 参与者、内容非空且 ≤ 上限、
// 来源 run=当前 run、proposal_id run 内幂等;校验失败静默拒绝(不影响发言轮)。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// proposalMemory 最小记忆替身:只记录 CreateMemory 调用。
type proposalMemory struct {
	service.Memory
	calls []groupMemoryCall
}

func (m *proposalMemory) CreateMemory(_ context.Context, agentUID string, in service.MemoryInput) (*model.AgentMemory, errors.Error) {
	m.calls = append(m.calls, groupMemoryCall{agentID: agentUID, in: in})
	return &model.AgentMemory{}, nil
}

// TestMemoryProposalValidationRules 表驱动校验规则:每条 payload 依序投递,累计断言调用次数。
func TestMemoryProposalValidationRules(t *testing.T) {
	li := "22222222-2222-2222-2222-222222222222"
	mem := &proposalMemory{}
	svc := &Chat{Memory: mem}
	participants := map[string]*groupParticipant{
		li: {uid: li, name: "李雪琴", memoryEnabled: true},
	}
	seen := map[string]bool{}
	session := &model.ChatSession{UUID: uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")}
	run := &model.ChatRun{UUID: uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")}

	cases := []struct {
		name      string
		payload   string
		wantCalls int
	}{
		{"有效提案落库", fmt.Sprintf(`{"proposal_id":"p1","agent_id":%q,"content":"辰时约饭"}`, li), 1},
		{"未知道人拒绝", `{"proposal_id":"p2","agent_id":"99999999-9999-9999-9999-999999999999","content":"越权"}`, 1},
		{"空白内容拒绝", fmt.Sprintf(`{"proposal_id":"p3","agent_id":%q,"content":"   "}`, li), 1},
		{"超长拒绝", fmt.Sprintf(`{"proposal_id":"p4","agent_id":%q,"content":%q}`, li, strings.Repeat("长", 2001)), 1},
		{"幂等键拒绝重复", fmt.Sprintf(`{"proposal_id":"p1","agent_id":%q,"content":"重复提案"}`, li), 1},
		{"恰好上限通过", fmt.Sprintf(`{"proposal_id":"p5","agent_id":%q,"content":%q}`, li, strings.Repeat("满", 2000)), 2},
	}
	for _, tc := range cases {
		var p memoryProposalPayload
		if err := json.Unmarshal([]byte(tc.payload), &p); err != nil {
			t.Fatalf("%s: 载荷解析: %v", tc.name, err)
		}
		svc.persistMemoryProposal(context.Background(), session, run, participants, seen, p)
		if len(mem.calls) != tc.wantCalls {
			t.Fatalf("%s: memory calls = %d, want %d", tc.name, len(mem.calls), tc.wantCalls)
		}
	}

	last := mem.calls[len(mem.calls)-1]
	if last.agentID != li {
		t.Fatalf("memory agent id = %q, want %q (李雪琴)", last.agentID, li)
	}
	if last.in.Kind != "episode" {
		t.Fatalf("memory kind = %q, want episode", last.in.Kind)
	}
	if last.in.SourceSessionID != session.UUID.String() {
		t.Fatalf("source session = %q, want %q", last.in.SourceSessionID, session.UUID.String())
	}
	if last.in.SourceMessageID != run.UUID.String() {
		t.Fatalf("source run = %q, want 当前 run %q", last.in.SourceMessageID, run.UUID.String())
	}
}
