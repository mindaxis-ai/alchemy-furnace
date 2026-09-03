package chat_service

// persistMemoryProposal:LangGraph 记忆提案落库(Task 13,设计 §10)。
// Python 图当前不发 memory_proposed(占位节点),Go 契约先行:四重校验——
// proposal_id run 内幂等、目标 agent ∈ 参与者、内容非空且 ≤ 上限、来源 run/会话可溯;
// 校验失败静默拒绝(提案是增益,不是发言依赖),存储失败仅日志。

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"go.uber.org/zap"
)

// memoryProposalMaxRunes 提案内容上限(rune)
const memoryProposalMaxRunes = 2000

// memoryProposalPayload 内部 memory_proposed 事件载荷(contracts.py MemoryProposal)
type memoryProposalPayload struct {
	ProposalID string `json:"proposal_id"`
	AgentID    string `json:"agent_id"`
	Content    string `json:"content"`
}

// groupParticipant 群轮参与者元数据(agent UUID → 群内身份;仅元数据查找,非编排决策)
type groupParticipant struct {
	id            uint
	name          string
	avatar        string
	memoryEnabled bool
}

// persistMemoryProposal 校验并落库一条记忆提案;seen 为 run 内 proposal_id 幂等表(调用方持有)。
func (s *Chat) persistMemoryProposal(ctx context.Context, session *model.ChatSession, run *model.ChatRun, participants map[string]*groupParticipant, seen map[string]bool, p memoryProposalPayload) {
	if p.ProposalID == "" || seen[p.ProposalID] {
		return
	}
	participant, ok := participants[p.AgentID]
	if !ok {
		return
	}
	content := strings.TrimSpace(p.Content)
	if content == "" || utf8.RuneCountInString(content) > memoryProposalMaxRunes {
		return
	}
	if s.Memory == nil {
		return
	}
	seen[p.ProposalID] = true
	if _, err := s.Memory.CreateMemory(ctx, participant.id, service.MemoryInput{
		Kind:            "episode",
		Content:         content,
		SourceSessionID: session.UUID.String(),
		SourceMessageID: run.UUID.String(),
	}); err != nil {
		zap.L().Warn("[炼丹炉] 记忆提案落库失败",
			zap.String("proposal_id", p.ProposalID),
			zap.String("run_id", run.UUID.String()))
	}
}
