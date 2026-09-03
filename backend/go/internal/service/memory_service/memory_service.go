// Package memory_service 道人本地记忆业务逻辑实现(spec §10)
// 检索:纯排序(无 embedding)——pinned > 关键词精确 > bigram > importance > 最近访问 > open_loop 加权;
// 创建:校验 → 内容哈希去重(同哈希 active 仅更新 confidence/importance)→ 冲突置替(同 kind+bigram≥0.85,pinned 永不置替);
// 蒸馏:容量 32 单 worker 非阻塞队列,LLM 结构化提取,按 agent UUID 分组逐目标蒸馏。
package memory_service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/google/uuid"

	"github.com/alchemy-furnace/server/internal/engineendpoint"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
)

// allowedKinds spec §10.1 类型枚举
var allowedKinds = map[string]bool{
	"user_fact": true, "user_preference": true, "relationship": true, "open_loop": true, "episode": true,
}

// 检索上限与队列容量(spec §10.3/§10.4)
const (
	maxSnippets     = 6
	maxSnippetChars = 1200
	queueCapacity   = 32
)

// MemoryService iservice.Memory 实现(spec §10)
type MemoryService struct {
	dao           dao.Memory
	creds         credential.Resolver
	engineBaseURL engineendpoint.Provider
	llmJSON       func(ctx context.Context, baseURL string, creds *credential.ModelCredentials, model string, messages []map[string]string) (string, error)

	queue     chan *distillJob
	startOnce sync.Once
	started   atomic.Bool
	closeOnce sync.Once
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// NewMemoryService 构造;creds/engineBaseURL 可为 nil(仅蒸馏用,测试不传)
func NewMemoryService(memoryDAO dao.Memory, creds credential.Resolver, engineBaseURL engineendpoint.Provider) *MemoryService {
	s := &MemoryService{
		dao:           memoryDAO,
		creds:         creds,
		engineBaseURL: engineBaseURL,
		queue:         make(chan *distillJob, queueCapacity),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	s.llmJSON = defaultLLMJSON
	return s
}

// ---------- CRUD ----------

// ListMemories 按道人列出记忆
func (s *MemoryService) ListMemories(ctx context.Context, agentID uint, kind string, onlyActive bool) ([]*model.AgentMemory, errors.Error) {
	return s.dao.ListMemories(ctx, agentID, kind, onlyActive)
}

// CreateMemory 创建记忆:校验 → 哈希去重 → 冲突置替(§10.2)
func (s *MemoryService) CreateMemory(ctx context.Context, agentID uint, in service.MemoryInput) (*model.AgentMemory, errors.Error) {
	if err := validateInput(in); err != nil {
		return nil, err
	}
	hash := memoryHash(in.Kind, in.Content)
	existing, err := s.dao.FindActiveByContentHash(ctx, agentID, hash)
	if err != nil && !errors.IsType(err, errors.ErrorTypeRecordNotFound) {
		return nil, err
	}
	if existing != nil {
		// 同哈希:只更新 confidence/importance(§10.2),pinned/status 不动
		if in.Importance != nil {
			existing.Importance = *in.Importance
		}
		if in.Confidence != nil {
			existing.Confidence = *in.Confidence
		}
		if err := s.dao.UpdateMemory(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}
	// 冲突检测:同 kind + bigram ≥0.85 → 旧 active 置替;pinned 永不置替(§10.2)
	if err := s.supersedeConflicts(ctx, agentID, in.Kind, in.Content); err != nil {
		return nil, err
	}
	m := &model.AgentMemory{
		UUID:            uuid.New(),
		AgentID:         agentID,
		Kind:            in.Kind,
		Content:         strings.TrimSpace(in.Content),
		Importance:      defaultInt(in.Importance, 3),
		Confidence:      defaultFloat(in.Confidence, 0.8),
		Pinned:          in.Pinned != nil && *in.Pinned,
		Status:          "active",
		SourceSessionID: in.SourceSessionID,
		SourceMessageID: in.SourceMessageID,
		ContentHash:     hash,
	}
	for _, kw := range in.Keywords {
		if len(m.Keywords) >= 12 {
			break
		}
		if kw = strings.TrimSpace(kw); kw != "" {
			m.Keywords = append(m.Keywords, kw)
		}
	}
	if err := s.dao.SaveMemory(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// UpdateMemory 按 UUID 部分更新记忆(nil 字段不更新);内容/类型变更时重算内容哈希
func (s *MemoryService) UpdateMemory(ctx context.Context, agentID uint, memoryUUID uuid.UUID, in service.MemoryInput) (*model.AgentMemory, errors.Error) {
	m, err := s.dao.TakeMemoryByUUID(ctx, memoryUUID)
	if err != nil {
		return nil, err
	}
	if m.AgentID != agentID {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "memory.agent_mismatch", "不属于该道人的记忆")
	}
	contentChanged, kindChanged := false, false
	if in.Content != "" {
		content := strings.TrimSpace(in.Content)
		if content == "" || len([]rune(content)) > 500 {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "memory.invalid_content", "记忆内容不能为空且不超过 500 字")
		}
		m.Content = content
		contentChanged = true
	}
	if in.Kind != "" {
		if !allowedKinds[in.Kind] {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "memory.invalid_kind", "非法的记忆类型")
		}
		kindChanged = in.Kind != m.Kind
		m.Kind = in.Kind
	}
	if contentChanged || kindChanged {
		m.ContentHash = memoryHash(m.Kind, m.Content)
	}
	if in.Importance != nil {
		if *in.Importance < 1 || *in.Importance > 5 {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "memory.invalid_importance", "重要性取值 1-5")
		}
		m.Importance = *in.Importance
	}
	if in.Confidence != nil {
		if *in.Confidence < 0 || *in.Confidence > 1 {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "memory.invalid_confidence", "置信度取值 0-1")
		}
		m.Confidence = *in.Confidence
	}
	if in.Pinned != nil {
		m.Pinned = *in.Pinned
	}
	if err := s.dao.UpdateMemory(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// DeleteMemory 物理删除单条记忆(§10.2)
func (s *MemoryService) DeleteMemory(ctx context.Context, agentID uint, memoryUUID uuid.UUID) errors.Error {
	m, err := s.dao.TakeMemoryByUUID(ctx, memoryUUID)
	if err != nil {
		return err
	}
	if m.AgentID != agentID {
		return errors.New(errors.ErrorTypeInvalidRequest, "memory.agent_mismatch", "不属于该道人的记忆")
	}
	return s.dao.DeleteMemory(ctx, m.ID)
}

// ClearMemories 物理清空道人全部记忆
func (s *MemoryService) ClearMemories(ctx context.Context, agentID uint) (int64, errors.Error) {
	return s.dao.DeleteMemoriesByAgent(ctx, agentID)
}

// ---------- 检索(§10.4) ----------

// Retrieve 按 pinned > 关键词精确 > bigram > importance > 最近访问 > open_loop 排序,
// 截 ≤6 条 ≤1200 字符;并 Touch 命中记忆的 LastAccessedAt。
func (s *MemoryService) Retrieve(ctx context.Context, agentID uint, userMessage string) ([]service.MemorySnippet, errors.Error) {
	list, err := s.dao.ListMemories(ctx, agentID, "", true)
	if err != nil {
		return nil, err
	}
	type scored struct {
		m     *model.AgentMemory
		score float64
		order int
	}
	scoredList := make([]scored, 0, len(list))
	query := normalizeForCompare(userMessage)
	for i, m := range list {
		score := 0.0
		if m.Pinned {
			score += 1e9 // pinned 恒优先
		}
		content := normalizeForCompare(m.Content)
		if query != "" && (strings.Contains(content, query) || containsKeyword(m.Keywords, userMessage)) {
			score += 1e6 // 关键词精确命中
		}
		if query != "" {
			score += bigramSimilarity(query, content) * 1e4
		}
		score += float64(m.Importance) * 100
		if m.Kind == "open_loop" {
			score += 50 // open_loop 加权(§10.4)
		}
		scoredList = append(scoredList, scored{m: m, score: score, order: i})
	}
	// 稳定排序:score 降序;同分按创建新近(order 倒序)
	for i := 1; i < len(scoredList); i++ {
		for j := i; j > 0 && (scoredList[j].score > scoredList[j-1].score ||
			(scoredList[j].score == scoredList[j-1].score && scoredList[j].order > scoredList[j-1].order)); j-- {
			scoredList[j], scoredList[j-1] = scoredList[j-1], scoredList[j]
		}
	}
	out := make([]service.MemorySnippet, 0, maxSnippets)
	total := 0
	touched := make([]uint, 0, maxSnippets)
	for _, s := range scoredList {
		if len(out) >= maxSnippets || total+len([]rune(s.m.Content)) > maxSnippetChars {
			break
		}
		out = append(out, service.MemorySnippet{Kind: s.m.Kind, Content: s.m.Content})
		total += len([]rune(s.m.Content))
		touched = append(touched, s.m.ID)
	}
	for _, id := range touched {
		_ = s.dao.TouchMemory(ctx, id)
	}
	return out, nil
}

// ---------- 校验与工具 ----------

func validateInput(in service.MemoryInput) errors.Error {
	if !allowedKinds[in.Kind] {
		return errors.New(errors.ErrorTypeInvalidRequest, "memory.invalid_kind", "非法的记忆类型")
	}
	content := strings.TrimSpace(in.Content)
	if content == "" || len([]rune(content)) > 500 {
		return errors.New(errors.ErrorTypeInvalidRequest, "memory.invalid_content", "记忆内容不能为空且不超过 500 字")
	}
	if in.Importance != nil && (*in.Importance < 1 || *in.Importance > 5) {
		return errors.New(errors.ErrorTypeInvalidRequest, "memory.invalid_importance", "重要性取值 1-5")
	}
	if in.Confidence != nil && (*in.Confidence < 0 || *in.Confidence > 1) {
		return errors.New(errors.ErrorTypeInvalidRequest, "memory.invalid_confidence", "置信度取值 0-1")
	}
	return nil
}

// memoryHash 内容哈希:SHA256(kind|normalized_content)(spec §10.2)
func memoryHash(kind, content string) string {
	sum := sha256.Sum256([]byte(kind + "|" + normalizeForCompare(content)))
	return hex.EncodeToString(sum[:])
}

// normalizeForCompare 归一化:去空白 + 小写(仅用于比较/哈希,不落库)
func normalizeForCompare(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if !unicode.IsSpace(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// containsKeyword 用户消息是否精确包含记忆关键词
func containsKeyword(keywords model.JSONList, userMessage string) bool {
	for _, k := range keywords {
		if s, ok := k.(string); ok && s != "" && strings.Contains(userMessage, s) {
			return true
		}
	}
	return false
}

// bigramSimilarity 两串共享 bigram 数 / 并集数(bigram Jaccard,与 behavior.BigramJaccard 语义一致;
// P2 合并后可直接改调 behavior.BigramJaccard)
func bigramSimilarity(a, b string) float64 {
	if a == "" || b == "" || len([]rune(a)) < 2 || len([]rune(b)) < 2 {
		return 0
	}
	setA := bigramSet(a)
	setB := bigramSet(b)
	inter := 0
	for k := range setB {
		if _, ok := setA[k]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func bigramSet(s string) map[string]struct{} {
	rs := []rune(s)
	if len(rs) < 2 {
		return nil
	}
	set := make(map[string]struct{}, len(rs)-1)
	for i := 0; i < len(rs)-1; i++ {
		set[string(rs[i:i+2])] = struct{}{}
	}
	return set
}

// supersedeConflicts 同 kind 且 bigram ≥0.85 的旧 active 置替;pinned 永不自动置替(§10.2)
func (s *MemoryService) supersedeConflicts(ctx context.Context, agentID uint, kind, content string) errors.Error {
	list, err := s.dao.ListMemories(ctx, agentID, kind, true)
	if err != nil {
		return err
	}
	norm := normalizeForCompare(content)
	for _, m := range list {
		if m.Pinned {
			continue // pinned 永不自动置替(§10.2)
		}
		if bigramSimilarity(norm, normalizeForCompare(m.Content)) >= 0.85 {
			if err := s.dao.SupersedeMemory(ctx, m.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func defaultInt(p *int, d int) int {
	if p != nil {
		return *p
	}
	return d
}

func defaultFloat(p *float64, d float64) float64 {
	if p != nil {
		return *p
	}
	return d
}
