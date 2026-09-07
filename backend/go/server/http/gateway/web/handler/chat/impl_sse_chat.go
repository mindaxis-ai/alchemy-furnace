package chat

import (
	"net/http"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/context/contextutil"
	ierr "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	chatservice "github.com/alchemy-furnace/server/internal/service/chat_service"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"

	"github.com/alchemy-furnace/server/model"

	"github.com/google/uuid"
)

// sseChatRequest SSE 流式对话请求体
type sseChatRequest struct {
	ModelName   string `json:"model_name"`
	Content     string `json:"content"`      // 用户问题
	Retry       bool   `json:"retry"`        // 重试既有用户消息，不重复落库
	DebugPrompt bool   `json:"debug_prompt"` // 返回本次实际模型输入；默认关闭
}

// SSEChat 标准 SSE 流式对话(RAW handler,不经 Wrapper,自行写出标准 SSE 事件)
// POST /api/v1/chat/sse/:uuid  body: {"content": "..."}
//
// 服务端 -> 客户端事件:
//
//	event: accepted data: {}                     （用户消息已保存或确认复用）
//	event: chunk    data: {"content": "回答片段"}
//	event: done     data: {}                     （完整回复已入库）
//	event: error    data: {"content": "可读中文错误"}
//	event: stopped  data: {}                     （客户端中断,尽力写出）
//	: ping                                      （25s 注释心跳）
//
// 停止生成: 客户端中断连接(AbortController.abort()),ctx 取消贯穿编排引擎;
// 取消/中断不保留部分回复(run 标记 interrupted,续跑走 /chat/runs/:run_id/resume)。
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
	preflightRecovery := chatservice.StreamRecoveryResend
	if body.Retry {
		preflightRecovery = chatservice.StreamRecoveryPersistedRetry
	}

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
		sseWriteEvent(w, flusher, "error", sseErrorPayload(err, true, preflightRecovery))
		return
	}
	// 群聊 Type=group 走专门通道(编排器驱动,带心跳保活)
	if session.Type == model.SessionTypeGroup {
		cls.runGroupSSE(c, sessionUID, content, body.Retry, body.DebugPrompt, strings.TrimSpace(body.ModelName))
		return
	}
	// 群聊 AgentID=nil 走单聊入口视为错误(防御性兜底)
	if session.AgentID == nil {
		sseWriteEvent(w, flusher, "error", ssePayload{Content: "该会话不支持单聊通道", Terminal: true, Recovery: preflightRecovery})
		return
	}
	// LangGraph 权威编排(Task 15 起唯一路径):handler 只做传输适配,
	// 校验、编排、持久化、事件语义全权委托服务层 RunConversation。
	cls.runLangGraphSSE(c, sessionUID, content, body.Retry, body.DebugPrompt, strings.TrimSpace(body.ModelName))
}

func sseErrorPayload(err ierr.Error, sessionLookup bool, recovery chatservice.StreamRecoveryMode) ssePayload {
	if sessionLookup && err.IsType(ierr.ErrorTypeRecordNotFound) {
		return ssePayload{Content: "会话不存在或已删除", ErrorCode: "service.chat.session_not_found", Terminal: true, Recovery: recovery}
	}
	switch err.GetCode() {
	case "service.chat.agent_inactive":
		return ssePayload{Content: "道人已停用", ErrorCode: err.GetCode(), Terminal: true, Recovery: recovery}
	case "service.chat.agent_not_found":
		return ssePayload{Content: "道人不存在", ErrorCode: err.GetCode(), Terminal: true, Recovery: recovery}
	case "service.chat.model_unavailable":
		return ssePayload{Content: "道人使用的模型不可用", ErrorCode: err.GetCode(), Terminal: true, Recovery: recovery}
	default:
		return ssePayload{Content: "暂时无法开始论道，请稍后重试", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: recovery}
	}
}

// runLangGraphSSE LangGraph 单聊路径:handler 只做传输适配(头/心跳承载/事件写回),
// 校验、编排、持久化、事件语义全权委托服务层 RunConversation。
func (cls *Chat) runLangGraphSSE(c *gin.Context, sessionUID uuid.UUID, content string, retry bool, debugPrompt bool, modelName string) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.InternalError(c, "当前服务不支持流式响应")
		return
	}
	setSSEHeaders(c)
	ctx := contextutil.NewContextWithGin(c)

	sw := &sseWriter{w: c.Writer, flusher: flusher}
	done := make(chan struct{})
	defer close(done)
	go func() { // 心跳:思考型模型长时间静默时防代理空闲超时(与 legacy/群聊通道同语义)
		ticker := time.NewTicker(sseHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				sw.ping()
			}
		}
	}()

	cls.chat.RunConversation(ctx, service.ConversationCommand{
		SessionUID:  sessionUID,
		Content:     content,
		Retry:       retry,
		DebugPrompt: debugPrompt,
		ModelName:   modelName,
	}, sw.event)
}

// ResumeRunSSE 续跑 run 的 RAW SSE 端点(Task 14;设计 §10/§11):
// POST /api/v1/chat/runs/:run_id/resume
// 只做传输适配(头/心跳承载/事件写回),校验、编排、持久化全权委托服务层 RunConversationResume。
// 契约:不发 accepted;chunk/done/error/stopped 与首轮同语义,控制事件携带 run_id。
func (cls *Chat) ResumeRunSSE(c *gin.Context) {
	runUID, uerr := uuid.Parse(c.Param("run_id"))
	if uerr != nil {
		response.BadRequest(c, "回合ID格式不正确")
		return
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.InternalError(c, "当前服务不支持流式响应")
		return
	}
	setSSEHeaders(c)
	ctx := contextutil.NewContextWithGin(c)

	sw := &sseWriter{w: c.Writer, flusher: flusher}
	done := make(chan struct{})
	defer close(done)
	go func() { // 心跳:续跑与首轮同语义,防代理空闲超时
		ticker := time.NewTicker(sseHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				sw.ping()
			}
		}
	}()

	cls.chat.RunConversationResume(ctx, runUID, sw.event)
}
