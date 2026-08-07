// Package handler 对话管理 HTTP 处理器
// 处理会话管理和 WebSocket 流式对话
// 对应 API: /api/v1/chat/sessions, /api/v1/chat/ws/:session_id
// WebSocket 聊天流程:
//  1. 接收用户消息
//  2. 查询会话关联的道人
//  3. 加载/合成道人的语言模式（系统提示词）
//  4. 调用 Python 语言引擎 /chat/completions/stream (SSE)
//  5. 流式转发给 WebSocket 客户端
//  6. 保存消息到数据库
package handler

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/pkg/response"
	"github.com/alchemy-furnace/server/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WebSocket 升级器配置
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// 允许所有来源的跨域 WebSocket 连接
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ChatHandler 对话 HTTP 处理器
type ChatHandler struct {
	service  *service.ChatService
	patterns *service.LanguagePatternService
}

// NewChatHandler 创建对话处理器
func NewChatHandler() *ChatHandler {
	return &ChatHandler{
		service:  service.NewChatService(),
		patterns: service.NewLanguagePatternService(),
	}
}

// CreateSession 创建对话会话
// POST /api/v1/chat/sessions
// Body: { "agent_id": 1, "title": "论道标题" }
func (h *ChatHandler) CreateSession(c *gin.Context) {
	var req model.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	session, err := h.service.CreateSession(&req)
	if err != nil {
		zap.L().Error("[炼丹炉] 创建会话失败", zap.Error(err))
		response.InternalError(c, "创建会话失败")
		return
	}

	response.Created(c, session)
}

// ListSessions 会话列表
// GET /api/v1/chat/sessions?page=1&page_size=10
func (h *ChatHandler) ListSessions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	sessions, total, err := h.service.ListSessions(page, pageSize)
	if err != nil {
		zap.L().Error("[炼丹炉] 查询会话列表失败", zap.Error(err))
		response.InternalError(c, "查询会话列表失败")
		return
	}

	response.SuccessWithPage(c, total, page, pageSize, sessions)
}

// GetMessages 获取会话消息历史
// GET /api/v1/chat/sessions/:id/messages?page=1&page_size=20
func (h *ChatHandler) GetMessages(c *gin.Context) {
	sessionID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "会话ID格式不正确")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	messages, total, err := h.service.GetMessages(uint(sessionID), page, pageSize)
	if err != nil {
		zap.L().Error("[炼丹炉] 查询消息历史失败", zap.Uint64("session_id", sessionID), zap.Error(err))
		response.InternalError(c, "查询消息历史失败")
		return
	}

	response.SuccessWithPage(c, total, page, pageSize, messages)
}

// WebSocketChat WebSocket 流式对话
// GET /api/v1/chat/ws/:session_id
// 协议: WebSocket
//
// 消息格式（JSON）:
//
//	客户端 -> 服务端: { "content": "用户问题" }
//	服务端 -> 客户端: { "type": "chunk", "content": "回答片段" }
//	服务端 -> 客户端: { "type": "done", "content": "" }
//	服务端 -> 客户端: { "type": "error", "content": "错误信息" }
func (h *ChatHandler) WebSocketChat(c *gin.Context) {
	sessionID, err := strconv.ParseUint(c.Param("session_id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "会话ID格式不正确")
		return
	}

	// 升级 HTTP 连接为 WebSocket
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.L().Error("[炼丹炉] WebSocket 升级失败", zap.Error(err))
		return
	}
	defer ws.Close()

	zap.L().Info("[炼丹炉] 仙缘已到，WebSocket 连接已建立",
		zap.Uint64("session_id", sessionID))

	// 获取会话关联的道人信息
	agentID, modelName, err := h.service.GetSessionAgentInfo(uint(sessionID))
	if err != nil {
		h.sendWSMessage(ws, "error", "获取会话信息失败: "+err.Error())
		return
	}

	// 加载/合成道人的语言模式（系统提示词）
	pattern, err := h.patterns.GetOrBuildPattern(agentID)
	if err != nil {
		zap.L().Error("[炼丹炉] 语言模式合成失败", zap.Error(err))
		h.sendWSMessage(ws, "error", "化丹为性失败: "+err.Error())
		return
	}

	zap.L().Info("[炼丹炉] 论道准备就绪",
		zap.Uint64("session_id", sessionID),
		zap.Uint("agent_id", agentID),
		zap.String("model", modelName),
		zap.String("fingerprint", pattern.SourceFingerprint))

	// 持续监听客户端消息
	for {
		// 读取客户端消息
		var msgReq model.ChatMessageRequest
		if err := ws.ReadJSON(&msgReq); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				zap.L().Warn("[炼丹炉] WebSocket 连接异常断开", zap.Error(err))
			}
			break
		}

		// 校验消息内容
		if strings.TrimSpace(msgReq.Content) == "" {
			h.sendWSMessage(ws, "error", "消息内容不能为空")
			continue
		}

		// 1. 保存用户消息到数据库
		_, err := h.service.SaveMessage(uint(sessionID), "user", msgReq.Content, nil)
		if err != nil {
			zap.L().Error("[炼丹炉] 保存用户消息失败", zap.Error(err))
			h.sendWSMessage(ws, "error", "保存消息失败")
			continue
		}

		zap.L().Info("[炼丹炉] 收到论道问题", zap.Uint64("session_id", sessionID), zap.String("content", msgReq.Content))

		// 2. 获取历史消息（最近 20 条，用于上下文）
		recentMessages, _, err := h.service.GetMessages(uint(sessionID), 1, 20)
		if err != nil {
			zap.L().Error("[炼丹炉] 获取历史消息失败", zap.Error(err))
			h.sendWSMessage(ws, "error", "获取历史消息失败")
			continue
		}

		// 构建消息列表（OpenAI 格式，首条为合成后的系统提示词）
		messages := []map[string]string{
			{"role": "system", "content": pattern.SystemPrompt},
		}
		for _, m := range recentMessages {
			messages = append(messages, map[string]string{
				"role":    m.Role,
				"content": m.Content,
			})
		}

		// 3. 调用语言引擎流式对话接口（SSE）
		ragStream, err := h.service.CallChatStream(messages, modelName)
		if err != nil {
			zap.L().Error("[炼丹炉] 调用语言引擎流式接口失败", zap.Error(err))
			h.sendWSMessage(ws, "error", "炼丹服务暂时不可用: "+err.Error())
			continue
		}

		// 4. 读取 SSE 流并转发到 WebSocket
		var fullContent strings.Builder
		scanner := bufio.NewScanner(ragStream)
		for scanner.Scan() {
			line := scanner.Text()

			// SSE 格式: data: {...}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// 处理流结束标记
			if data == "[DONE]" {
				break
			}

			// 解析 SSE JSON（语言引擎直接输出 {"content": "..."} 格式）
			var chunk struct {
				Content string `json:"content"`
				Error   string `json:"error"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue // 跳过无法解析的行
			}

			if chunk.Error != "" {
				h.sendWSMessage(ws, "error", chunk.Error)
				break
			}

			if chunk.Content != "" {
				fullContent.WriteString(chunk.Content)
				// 实时转发到 WebSocket
				h.sendWSMessage(ws, "chunk", chunk.Content)
			}
		}
		ragStream.Close()

		if err := scanner.Err(); err != nil {
			zap.L().Warn("[炼丹炉] SSE 流读取异常", zap.Error(err))
		}

		// 5. 发送完成标记
		h.sendWSMessage(ws, "done", "")

		// 6. 保存助手回复到数据库
		fullText := fullContent.String()
		if fullText != "" {
			_, err = h.service.SaveMessage(uint(sessionID), "assistant", fullText, nil)
			if err != nil {
				zap.L().Error("[炼丹炉] 保存助手回复失败", zap.Error(err))
			}
		}

		zap.L().Info("[炼丹炉] 论道一轮完成", zap.Uint64("session_id", sessionID), zap.Int("response_length", len(fullText)))
	}

	zap.L().Info("[炼丹炉] WebSocket 论道结束", zap.Uint64("session_id", sessionID))
}

// sendWSMessage 发送 WebSocket 消息（带重试和超时保护）
func (h *ChatHandler) sendWSMessage(ws *websocket.Conn, msgType, content string) {
	msg := map[string]string{
		"type":    msgType,
		"content": content,
	}

	// 设置写超时
	ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	defer ws.SetWriteDeadline(time.Time{})

	if err := ws.WriteJSON(msg); err != nil {
		zap.L().Warn("[炼丹炉] WebSocket 消息发送失败", zap.Error(err))
	}
}

// WSMessage WebSocket 消息结构（用于文档说明）
type WSMessage struct {
	Type    string `json:"type"`    // chunk / done / error
	Content string `json:"content"` // 消息内容
}
