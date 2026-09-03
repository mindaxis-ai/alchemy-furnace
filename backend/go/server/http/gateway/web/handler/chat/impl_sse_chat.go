package chat

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/behavior"
	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	ierr "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	chatservice "github.com/alchemy-furnace/server/internal/service/chat_service"
	"github.com/alchemy-furnace/server/internal/service/turnpolicy"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/alchemy-furnace/server/model"

	"github.com/google/uuid"
)

// sseChatRequest SSE 流式对话请求体
type sseChatRequest struct {
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
		cls.runGroupSSE(c, sessionUID, content, body.Retry, body.DebugPrompt)
		return
	}
	// 群聊 AgentID=nil 走单聊入口视为错误(防御性兜底)
	if session.AgentID == nil {
		sseWriteEvent(w, flusher, "error", ssePayload{Content: "该会话不支持单聊通道", Terminal: true, Recovery: preflightRecovery})
		return
	}
	// 编排引擎迁移开关(临时,设计 §12):显式 langgraph 时 handler 只做输入校验与委托,
	// 不触碰 legacy 组装链(TurnPlan/ComposeSystemPrompt/记忆检索/StreamChat);其余值一律 legacy。
	if orchestrationEngineLangGraph() {
		cls.runLangGraphSSE(c, sessionUID, content, body.Retry, body.DebugPrompt)
		return
	}
	agentID := *session.AgentID
	sessionID := session.ID
	creds, err := cls.chat.AuthorizeSessionForStream(ctx, session)
	if err != nil {
		sseWriteEvent(w, flusher, "error", sseErrorPayload(err, false, preflightRecovery))
		return
	}
	modelName := session.Agent.ModelName

	// 加载/合成道人的语言模式(系统提示词)
	pattern, err := cls.chat.GetOrBuildPattern(ctx, agentID)
	if err != nil {
		sseWriteEvent(w, flusher, "error", ssePayload{Content: "暂时无法开始论道，请稍后重试", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: preflightRecovery})
		return
	}

	// 行为档案(JSONMap)→ 结构化 profile;缺失/损坏则 nil,退化=只有静态分区
	var profile *behavior.DaoistBehaviorProfile
	if len(pattern.BehaviorProfile) > 0 {
		if raw, merr := json.Marshal(pattern.BehaviorProfile); merr == nil {
			var p behavior.DaoistBehaviorProfile
			if uerr := json.Unmarshal(raw, &p); uerr == nil {
				profile = &p
			}
		}
	}

	var recentMessages []*model.ChatMessage
	if body.Retry {
		// 重试复用最近一次同内容用户消息，避免前端重发导致本地与数据库各多一行。
		latestUser, latestErr := cls.chat.TakeLatestUserMessage(ctx, sessionID)
		if latestErr != nil {
			sseWriteEvent(w, flusher, "error", ssePayload{Content: "获取历史消息失败", ErrorCode: "service.chat.retry_unavailable", Terminal: true})
			return
		}
		if latestUser.Content != content {
			sseWriteEvent(w, flusher, "error", ssePayload{Content: "无法重试该消息，请重新发送", ErrorCode: "service.chat.retry_unavailable", Terminal: true})
			return
		}
	} else {
		// 新回合才保存用户消息。
		if _, err := cls.chat.SaveMessage(ctx, sessionID, "user", content); err != nil {
			sseWriteEvent(w, flusher, "error", ssePayload{Content: "保存消息失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: chatservice.StreamRecoveryResend})
			return
		}
	}
	sseWriteEvent(w, flusher, "accepted", struct{}{})

	zap.L().Info("[炼丹炉] 收到论道问题",
		zap.String("session_uuid", sessionUID.String()),
		zap.Uint("agent_id", agentID),
		zap.String("model", modelName),
		zap.String("content", content))

	// 获取历史消息(最近 20 条,用于上下文)
	_, recentMessages, err = cls.chat.GetMessages(ctx, sessionUID, 1, 20)
	if err != nil {
		sseWriteEvent(w, flusher, "error", ssePayload{Content: "获取历史消息失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: chatservice.StreamRecoveryPersistedRetry})
		return
	}

	// 构建消息列表(OpenAI 格式,首条为动态分区合成的系统提示词)
	// 用户当轮约束 → TurnPlan → 动态分区提示词(spec §8/§11)
	constraints := turnpolicy.ExtractUserTurnConstraints(content)
	plan := turnpolicy.BuildTurnPlan(constraints, turnpolicy.PolicyForProactivity(session.Agent.Proactivity), 1, nil)
	if profile != nil {
		plan.ActivatedRules = behavior.ActivatePillRules(content, profile)
	}
	// 本地记忆注入(§10.4;memory_enabled 门控,检索失败静默降级为无记忆)
	if session.Agent.MemoryEnabled {
		plan.Memories = cls.chat.RetrieveMemories(ctx, session.Agent.ID, content)
	}
	systemPrompt := behavior.ComposeSystemPrompt(profile, session.Agent.Name, plan)
	if systemPrompt == "" {
		systemPrompt = pattern.SystemPrompt // 防御:profile 缺失时用缓存提示词
	}
	messages := []map[string]string{{"role": "system", "content": systemPrompt}}
	// few-shot:system → 示例对话 → 真实历史(Task 13;最新 user 消息仍须是最后一项)
	if profile != nil {
		messages = append(messages, behavior.BuildFewShotMessages(profile, 2, 400)...)
	}
	for _, m := range recentMessages {
		messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
	}

	// 停止语义:accepted 已发,直接 done,引擎 0 调用(spec §8.2)
	if plan.Stop {
		sseWriteEvent(w, flusher, "done", ssePayload{})
		return
	}
	generationOptions := service.GenerationOptions{MaxTokens: plan.MaxTokens, MaxSentences: plan.MaxSentences}
	if body.DebugPrompt {
		sseWriteEvent(w, flusher, "prompt_debug", service.NewPromptDebugPayload(
			session.Agent.UUID.String(), session.Agent.Name, creds.Model, messages, generationOptions,
		))
	}

	// 请求级生命周期: 客户端中断 -> ctx 取消 -> 上游 LLM 流中断
	chunkCh := make(chan string)
	resultCh := make(chan streamResult, 1)
	go func() {
		full, canceled, streamErr := cls.chat.StreamChat(ctx, messages, creds, generationOptions, func(chunk string) {
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
			// 记忆蒸馏异步入队(§10.3;失败/禁用/中断/空回复静默)
			if session.Agent.MemoryEnabled && res.err == nil && !res.canceled && res.full != "" {
				cls.chat.EnqueueMemoryDistillation(ctx, service.DistillationSpec{
					SessionUUID: session.UUID.String(),
					Model:       session.Agent.ModelName,
					UserMessage: content,
					Targets: []service.DistillTarget{{
						AgentID: session.Agent.ID,
						Messages: []service.DistillMessage{
							{Role: "user", Content: content},
							{Role: "assistant", Content: res.full},
						},
					}},
				})
			}
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
		var interrupted *chatservice.StreamInterruptedError
		if stderrors.As(res.err, &interrupted) {
			sseWriteEvent(w, flusher, "error", ssePayload{
				Content: interrupted.Error(), ErrorCode: interrupted.StreamErrorCode(), Terminal: true, Recovery: chatservice.StreamRecoveryPersistedRetry,
			})
			return
		}
		// 未知上游错误不越过 HTTP 边界，避免原始 provider/凭证细节进入客户端。
		sseWriteEvent(w, flusher, "error", ssePayload{Content: "语言引擎服务异常，请稍后重试", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: chatservice.StreamRecoveryPersistedRetry})

	default:
		if strings.TrimSpace(res.full) == "" {
			sseWriteEvent(w, flusher, "error", ssePayload{
				Content:   "道人未生成有效回复，请稍后重试",
				ErrorCode: "service.chat.empty_response", Terminal: true,
				Recovery: chatservice.StreamRecoveryPersistedRetry,
			})
			return
		}
		if res.full != "" {
			if _, err := cls.chat.SaveMessage(saveCtx, sessionID, "assistant", res.full); err != nil {
				zap.L().Error("[炼丹炉] 保存助手回复失败", zap.Error(err))
				sseWriteEvent(w, flusher, "error", ssePayload{
					Content: "回复保存失败，请重试", ErrorCode: "service.chat.stream_unavailable",
					Terminal: true, Recovery: chatservice.StreamRecoveryPersistedRetry,
				})
				return
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

// orchestrationEngineLangGraph 编排引擎迁移开关(临时,设计 §12):
// 仅显式 langgraph 走权威编排,其余(含空)一律 legacy,存量部署零改动。
// loader 启动期已归一化(空→legacy,非法值报错);此处防御性再 trim。
func orchestrationEngineLangGraph() bool {
	return strings.TrimSpace(configuration.Configuration.OrchestrationEngine) == "langgraph"
}

// runLangGraphSSE LangGraph 单聊路径:handler 只做传输适配(头/心跳承载/事件写回),
// 校验、编排、持久化、事件语义全权委托服务层 RunConversation。
func (cls *Chat) runLangGraphSSE(c *gin.Context, sessionUID uuid.UUID, content string, retry bool, debugPrompt bool) {
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
