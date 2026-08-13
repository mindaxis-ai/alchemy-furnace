package chat

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/alchemy-furnace/server/model"

	"github.com/google/uuid"
)

// sseChatRequest SSE 流式对话请求体
type sseChatRequest struct {
	Content string `json:"content"` // 用户问题
}

// SSEChat 标准 SSE 流式对话(RAW handler,不经 Wrapper,自行写出标准 SSE 事件)
// POST /api/v1/chat/sse/:uuid  body: {"content": "..."}
//
// 服务端 -> 客户端事件:
//
//	event: chunk   data: {"content": "回答片段"}
//	event: done    data: {}                     （完整回复已入库）
//	event: error   data: {"content": "可读中文错误"}
//	event: stopped data: {}                     （客户端中断,尽力写出）
//	: ping                                    （25s 注释心跳）
//
// 停止生成: 客户端中断连接(AbortController.abort()),ctx 取消贯穿至上游 LLM 流;
// 已生成的部分内容(非空时)保存为 assistant 消息。
func (cls *Chat) SSEChat(c *gin.Context) {
	sessionUID, err := parseUUID(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var body sseChatRequest
	if berr := request.ShouldBindJSON(c, &body); berr != nil {
		response.BadRequest(c, berr.Error())
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		response.BadRequest(c, "消息内容不能为空")
		return
	}
	content := body.Content

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.InternalError(c, "当前服务不支持流式响应")
		return
	}

	setSSEHeaders(c)
	w := c.Writer
	// NewContextWithGin 以 c.Request.Context() 为基底,保留客户端断连取消语义并携带 request_id
	ctx := contextutil.NewContextWithGin(c)

	// 获取会话关联的道人信息(session 预加载 Agent,边界: session 由 :uuid 解析)
	session, err := cls.chat.GetSessionAgentInfo(ctx, sessionUID)
	if err != nil {
		sseWriteEvent(w, flusher, "error", ssePayload{Content: "获取会话信息失败"})
		return
	}
	// 群聊 Type=group 走专门通道(编排器驱动,带心跳保活)
	if session.Type == model.SessionTypeGroup {
		cls.runGroupSSE(c, sessionUID, content)
		return
	}
	// 群聊 AgentID=nil 走单聊入口视为错误(防御性兜底)
	if session.AgentID == nil {
		sseWriteEvent(w, flusher, "error", ssePayload{Content: "该会话不支持单聊通道"})
		return
	}
	agentID := *session.AgentID
	sessionID := session.ID
	modelName := session.Agent.ModelName

	// 加载/合成道人的语言模式(系统提示词)
	pattern, err := cls.chat.GetOrBuildPattern(ctx, agentID)
	if err != nil {
		sseWriteEvent(w, flusher, "error", ssePayload{Content: "化丹为性失败: " + err.Error()})
		return
	}

	// 保存用户消息到数据库
	if _, err := cls.chat.SaveMessage(ctx, sessionID, "user", content); err != nil {
		sseWriteEvent(w, flusher, "error", ssePayload{Content: "保存消息失败"})
		return
	}

	zap.L().Info("[炼丹炉] 收到论道问题",
		zap.String("session_uuid", sessionUID.String()),
		zap.Uint("agent_id", agentID),
		zap.String("model", modelName),
		zap.String("content", content))

	// 获取历史消息(最近 20 条,用于上下文)
	_, recentMessages, err := cls.chat.GetMessages(ctx, sessionUID, 1, 20)
	if err != nil {
		sseWriteEvent(w, flusher, "error", ssePayload{Content: "获取历史消息失败"})
		return
	}

	// 构建消息列表(OpenAI 格式,首条为合成后的系统提示词)
	messages := []map[string]string{{"role": "system", "content": pattern.SystemPrompt}}
	for _, m := range recentMessages {
		messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
	}

	// 解析模型凭证(每轮解析,模型停用/换钥即时生效)
	creds, err := cls.chat.ResolveCredentials(ctx, modelName)
	if err != nil {
		sseWriteEvent(w, flusher, "error", ssePayload{Content: err.Error()})
		return
	}

	// 请求级生命周期: 客户端中断 -> ctx 取消 -> 上游 LLM 流中断
	chunkCh := make(chan string)
	resultCh := make(chan streamResult, 1)
	go func() {
		full, canceled, streamErr := cls.chat.StreamChat(ctx, messages, creds, func(chunk string) {
			select {
			case chunkCh <- chunk:
			case <-ctx.Done():
			}
		})
		resultCh <- streamResult{full: full, canceled: canceled, err: streamErr}
	}()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case chunk := <-chunkCh:
			sseWriteEvent(w, flusher, "chunk", ssePayload{Content: chunk})
			heartbeat.Reset(sseHeartbeatInterval)

		case <-heartbeat.C:
			// 生成期间无 chunk 间隙保活(如思考型模型长时间静默)
			sseWriteComment(w, flusher, "ping")

		case res := <-resultCh:
			cls.finishSSEStream(ctx, sessionUID, sessionID, content, w, flusher, res)
			return

		case <-ctx.Done():
			// 客户端已中断: StreamChat 每轮检查 ctx,会迅速收尾返回部分内容
			res := <-resultCh
			// 保存用脱离取消的上下文(客户端已断连,原 ctx 已取消,但落库仍需执行)
			saveCtx := context.WithoutCancel(ctx)
			if res.full != "" {
				if _, err := cls.chat.SaveMessage(saveCtx, sessionID, "assistant", res.full); err != nil {
					zap.L().Error("[炼丹炉] 保存中断时的部分回复失败", zap.Error(err))
				}
			}
			sseWriteEvent(w, flusher, "stopped", ssePayload{}) // best-effort: 客户端可能已断开,写入失败被忽略
			zap.L().Info("[炼丹炉] 论道已被叫停",
				zap.String("session_uuid", sessionUID.String()), zap.Int("partial_length", len(res.full)))
			return
		}
	}
}

// finishSSEStream 处理 StreamChat 正常收尾: done / error / 取消(恰好遇完成按 done 处理)
// 取消与落库使用脱离取消的上下文,保证客户端断连后仍能写入部分回复
func (cls *Chat) finishSSEStream(ctx context.Context, sessionUID uuid.UUID, sessionID uint, userContent string, w http.ResponseWriter, flusher http.Flusher, res streamResult) {
	saveCtx := context.WithoutCancel(ctx)
	switch {
	case res.canceled:
		// 连接已中断(取消仅由 ctx 触发): 保存部分内容,尽力写 stopped 事件
		if res.full != "" {
			if _, err := cls.chat.SaveMessage(saveCtx, sessionID, "assistant", res.full); err != nil {
				zap.L().Error("[炼丹炉] 保存停止时的部分回复失败", zap.Error(err))
			}
		}
		sseWriteEvent(w, flusher, "stopped", ssePayload{}) // best-effort
		zap.L().Info("[炼丹炉] 论道已被叫停",
			zap.Uint("session_id", sessionID), zap.Int("partial_length", len(res.full)))

	case res.err != nil:
		// 结构化中文错误(模型凭证无效 / 引擎超时 / 引擎异常等)
		sseWriteEvent(w, flusher, "error", ssePayload{Content: res.err.Error()})

	default:
		if res.full != "" {
			if _, err := cls.chat.SaveMessage(saveCtx, sessionID, "assistant", res.full); err != nil {
				zap.L().Error("[炼丹炉] 保存助手回复失败", zap.Error(err))
			}
		}
		// 首问答自动命名(单聊触发点): 失败静默
		if title := cls.chat.GenerateSessionTitle(saveCtx, sessionUID, userContent, res.full); title != "" {
			sseWriteEvent(w, flusher, "title", struct {
				Title string `json:"title"`
			}{Title: title})
		}
		sseWriteEvent(w, flusher, "done", ssePayload{})
		zap.L().Info("[炼丹炉] 论道一轮完成",
			zap.Uint("session_id", sessionID), zap.Int("response_length", len(res.full)))
	}
}
