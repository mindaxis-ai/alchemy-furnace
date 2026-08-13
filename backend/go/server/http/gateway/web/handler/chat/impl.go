// Package chat 对话管理 HTTP 处理器(新网关;对齐 Luna-CY 模板 handler 分包风格)
// 路由: /api/v1/chat;路径参数 :uuid 为会话对外唯一标识
// SSE 流式对话为 RAW handler(不经 Wrapper,自行写出标准 SSE 事件)
package chat

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Chat 对话处理器
type Chat struct {
	chat service.Chat
}

// New 构造对话处理器
func New(chat service.Chat) *Chat {
	return &Chat{chat: chat}
}

// ---------- 响应 DTO ----------

// SessionResponse 会话响应 DTO:id/agent_id 输出 UUID 字符串,不泄露数字主键
type SessionResponse struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // single | group
	AgentID   string    `json:"agent_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MessageResponse 消息响应 DTO:id 输出消息 UUID;session_id 不输出
type MessageResponse struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// toSessionResponse 内部模型 -> 对外 DTO(agent 需预加载以取 UUID)
// 群聊 AgentID 可能为 nil(单聊字段未使用),需空值安全
func toSessionResponse(s *model.ChatSession) *SessionResponse {
	agentID := ""
	if s.AgentID != nil {
		agentID = s.Agent.UUID.String()
	}
	typeStr := s.Type
	if typeStr == "" {
		typeStr = model.SessionTypeSingle
	}
	return &SessionResponse{
		ID:        s.UUID.String(),
		Type:      typeStr,
		AgentID:   agentID,
		Title:     s.Title,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

// toSessionResponseList 批量转换
func toSessionResponseList(sessions []*model.ChatSession) []*SessionResponse {
	list := make([]*SessionResponse, 0, len(sessions))
	for _, s := range sessions {
		list = append(list, toSessionResponse(s))
	}
	return list
}

// toMessageResponse 内部模型 -> 对外 DTO
func toMessageResponse(m *model.ChatMessage) *MessageResponse {
	return &MessageResponse{
		ID:        m.UUID.String(),
		Role:      m.Role,
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
	}
}

// toMessageResponseList 批量转换
func toMessageResponseList(messages []*model.ChatMessage) []*MessageResponse {
	list := make([]*MessageResponse, 0, len(messages))
	for _, m := range messages {
		list = append(list, toMessageResponse(m))
	}
	return list
}

// ---------- 路径参数解析 ----------

// parseUUID 解析 :uuid 路径参数(会话);非法形态返回 400
func parseUUID(c *gin.Context) (uuid.UUID, errors.Error) {
	uid, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		return uuid.Nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.chat.uuid_parse", "会话ID格式不正确")
	}
	return uid, nil
}

// ---------- SSE 工具 ----------

// sseHeartbeatInterval 等待上游超过该间隔则发送注释心跳(防代理空闲超时)
const sseHeartbeatInterval = 25 * time.Second

// setSSEHeaders 设置 SSE 响应头(X-Accel-Buffering: no 指示 nginx 不缓冲)
func setSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
}

// sseWriteEvent 写入一条标准 SSE 事件(event + data 双行,空行分隔)并立即 Flush
func sseWriteEvent(w http.ResponseWriter, flusher http.Flusher, event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		zap.L().Warn("[炼丹炉] SSE 事件序列化失败", zap.String("event", event), zap.Error(err))
		return
	}
	if _, err := w.Write([]byte("event: " + event + "\ndata: " + string(data) + "\n\n")); err != nil {
		zap.L().Warn("[炼丹炉] SSE 事件写入失败", zap.String("event", event), zap.Error(err))
	}
	flusher.Flush()
}

// sseWriteComment 写入 SSE 注释行(心跳,客户端解析器应忽略)并立即 Flush
func sseWriteComment(w http.ResponseWriter, flusher http.Flusher, comment string) {
	if _, err := w.Write([]byte(": " + comment + "\n\n")); err != nil {
		zap.L().Warn("[炼丹炉] SSE 心跳写入失败", zap.Error(err))
	}
	flusher.Flush()
}

// ssePayload 事件数据载体(chunk: {"content": "..."}, done: {}, error/stopped: {"content": "..."})
type ssePayload struct {
	Content string `json:"content,omitempty"`
}

// streamResult StreamChat goroutine 的收尾结果
type streamResult struct {
	full     string
	canceled bool
	err      error
}
