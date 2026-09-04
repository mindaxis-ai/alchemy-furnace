package service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// MemorySnippet 注入编排上下文的单条本地记忆(spec §10.4;自 turnpolicy 迁入——
// Task 15 LangGraph 权威化后记忆检索只服务编排快照,归属记忆接口)。
type MemorySnippet struct {
	Kind    string
	Content string
}

// DistillMessage 蒸馏输入的单条对话消息
type DistillMessage struct {
	Role    string
	Content string
}

// DistillTarget 蒸馏目标道人及其轮内消息
type DistillTarget struct {
	AgentID  string // 道人 UUID 文本
	Messages []DistillMessage
}

// DistillationSpec 一次蒸馏任务的完整输入(群聊一轮 N 个发言道人 = N 个 Target,一次调用)
type DistillationSpec struct {
	SessionUUID string
	Model       string
	UserMessage string
	Targets     []DistillTarget
}

// MemoryInput 记忆创建/更新输入(指针字段 = 不更新)
type MemoryInput struct {
	Kind            string
	Content         string
	Keywords        []string
	Importance      *int
	Confidence      *float64
	Pinned          *bool
	SourceSessionID string
	SourceMessageID string
}

// Memory 道人本地记忆业务接口(spec §10)
// 检索=纯排序无 embedding;蒸馏=LLM 结构化提取 + 哈希去重 + 冲突置替;队列=容量 32 单 worker 非阻塞
type Memory interface {
	// ListMemories 按道人列出记忆(kind 为空不过滤;onlyActive=true 仅 active);agentUID 为道人 UUID 文本
	ListMemories(ctx context.Context, agentUID string, kind string, onlyActive bool) ([]*model.AgentMemory, errors.Error)

	// CreateMemory 创建记忆:校验 → 哈希去重(同哈希仅更新 importance/confidence)→ 冲突置替(同 kind+bigram≥0.85,pinned 除外)
	CreateMemory(ctx context.Context, agentUID string, in MemoryInput) (*model.AgentMemory, errors.Error)

	// UpdateMemory 按 UUID 部分更新记忆(nil 字段不更新);不属于该道人返回 InvalidRequest
	UpdateMemory(ctx context.Context, agentUID string, memoryUUID uuid.UUID, in MemoryInput) (*model.AgentMemory, errors.Error)

	// DeleteMemory 物理删除单条记忆;不属于该道人返回 InvalidRequest
	DeleteMemory(ctx context.Context, agentUID string, memoryUUID uuid.UUID) errors.Error

	// ClearMemories 物理清空道人全部记忆,返回删除条数
	ClearMemories(ctx context.Context, agentUID string) (int64, errors.Error)

	// Retrieve 检索记忆:pinned > 关键词精确 > bigram > importance > 最近访问 > open_loop;
	// 每轮 ≤6 条 ≤1200 字符,并 Touch 命中记忆的 LastAccessedAt
	Retrieve(ctx context.Context, agentUID string, userMessage string) ([]MemorySnippet, errors.Error)

	// EnqueueDistillation 非阻塞入队蒸馏任务;队列满返回 false(§10.3)
	EnqueueDistillation(ctx context.Context, spec DistillationSpec) bool

	// Close 有限关闭:停收新任务,处理完当前任务后退出
	Close()
}
