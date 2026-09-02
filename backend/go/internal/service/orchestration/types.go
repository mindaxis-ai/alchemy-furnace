// Package orchestration Python 编排引擎的 Go 内部客户端(Task 10)。
//
// 消费 Python 内部 SSE API(Task 8):
//   - POST /api/v1/orchestration/runs/stream/resume/cancel 三端点
//
// 请求类型逐字镜像 Python wire 契约(backend/python/app/orchestration/contracts.py)。
// 请求体含运行期模型凭据:本包任何代码路径不得 log/format 请求体。
package orchestration

import "encoding/json"

// Request 一次编排用户轮的完整输入(镜像 OrchestrationRequest)。
type Request struct {
	RunID        string                `json:"run_id"`
	SessionID    string                `json:"session_id"`
	SessionType  string                `json:"session_type"`
	UserTurn     UserTurn              `json:"user_turn"`
	History      []Message             `json:"history_snapshot"`
	Agents       []Agent               `json:"agent_snapshots"`
	Memories     []Memory              `json:"memory_snapshots"`
	Credentials  map[string]Credential `json:"credentials"`
	DebugEnabled bool                  `json:"debug_enabled"`
}

// UserTurn 本轮用户输入(镜像 UserTurnSnapshot)。
type UserTurn struct {
	MessageID string   `json:"message_id"`
	Text      string   `json:"text"`
	Mentions  []string `json:"mentioned_agent_ids,omitempty"`
}

// Message 历史消息(镜像 MessageSnapshot)。
type Message struct {
	MessageID string  `json:"message_id"`
	Role      string  `json:"role"`
	Text      string  `json:"text"`
	AgentID   *string `json:"agent_id,omitempty"`
}

// ModelRef 模型引用(镜像 ModelRef)。
type ModelRef struct {
	ProviderType string `json:"provider_type"`
	Name         string `json:"name"`
}

// Credential 按请求透传的模型凭据(镜像 ModelCredential)。
type Credential struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// Agent 编排参与道人快照(镜像 AgentSnapshot)。
type Agent struct {
	AgentID  string   `json:"agent_id"`
	Name     string   `json:"name"`
	ModelRef ModelRef `json:"model_ref"`
}

// Memory 道人记忆快照(镜像 MemorySnapshot;图只选择注入,不改写)。
type Memory struct {
	MemoryID string `json:"memory_id"`
	AgentID  string `json:"agent_id"`
	Text     string `json:"text"`
}

// Event 一条内部编排事件:name 来自 SSE event: 行;payload 为 data: 行的 JSON 原文。
type Event struct {
	Name    string
	RunID   string
	Payload json.RawMessage
}
