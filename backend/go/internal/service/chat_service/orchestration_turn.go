package chat_service

// RunConversation:LangGraph 权威编排的统一对话轮入口(Task 12;迁移开关设计 §12)。
//
// 职责边界:handler 只做输入校验与委托;本入口全权负责编排请求组装、内部事件映射、
// run 生命周期与持久化语义;按会话类型路由单聊/群聊实现(Task 12/13)。
//
// 事件契约(公共 SSE,与 legacy 单聊一致):accepted/chunk/prompt_debug/title/done/error/stopped。
// 持久化语义:只落 assistant_final(经 SaveFinalReplyOnce 按 run+reply 幂等),增量不落库;
// 取消/中断不保留部分回复,run 行标记 interrupted(重试建新 run)。
//
// 安全:编排请求体含运行期凭据,本文件任何代码路径不得 log/format 请求体;
// 调试一律经 Request.StateProjection() 投影。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/orchestration"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// orchestrationEngineSelected 编排引擎迁移开关的 service 侧读取(临时,设计 §12)。
// 与 handler 侧 orchestrationEngineLangGraph 同判据:仅显式 langgraph 走权威编排。
func orchestrationEngineSelected() bool {
	return strings.TrimSpace(configuration.Configuration.OrchestrationEngine) == "langgraph"
}

// ConversationEventPayload LangGraph 路径公共事件载荷(JSON 形状与 handler ssePayload 一致)。
// RunID 注入经 withRunID(设计 §10/§11):accepted/stopped/done/error 等控制事件携带,
// 前端以此定位停止/续跑控件;chunk 高频增量不携带(省带宽)。
type ConversationEventPayload struct {
	Content   string             `json:"content,omitempty"`
	ErrorCode string             `json:"error_code,omitempty"`
	Terminal  bool               `json:"terminal,omitempty"`
	Recovery  StreamRecoveryMode `json:"recovery,omitempty"`
	RunID     string             `json:"run_id,omitempty"`
}

// RunConversation 驱动一轮 LangGraph 单聊:校验→落/复用用户消息→run 生命周期→
// 流式映射→按终态收尾。事件经 emit 直出(handler 原样写 SSE),无部分缓冲。
func (s *Chat) RunConversation(ctx context.Context, cmd service.ConversationCommand, emit func(event string, payload any)) {
	if cmd.DebugPrompt {
		ctx = WithPromptDebug(ctx, true)
	}
	session, serr := s.chat.TakeSessionByUUID(ctx, cmd.SessionUID)
	if serr != nil {
		emit("error", ConversationEventPayload{Content: "会话不存在或已删除", ErrorCode: "service.chat.session_not_found", Terminal: true, Recovery: StreamRecoveryResend})
		return
	}
	// 按会话类型路由(Task 13):单聊/群聊共用入口、run 生命周期与持久化语义
	switch session.Type {
	case model.SessionTypeSingle:
		s.runSingleConversation(ctx, session, cmd, emit)
	case model.SessionTypeGroup:
		s.runGroupConversation(ctx, session, cmd, emit)
	default:
		emit("error", ConversationEventPayload{Content: "该会话不支持对话通道", ErrorCode: "service.chat.session_not_found", Terminal: true, Recovery: StreamRecoveryResend})
	}
}

// runSingleConversation LangGraph 单聊轮:落/复用用户消息→run 生命周期→流式映射→收尾。
// run 建行前移到 accepted 之前(Task 14,设计 §10):accepted 携带 run_id,前端由此定位停止/续跑控件。
func (s *Chat) runSingleConversation(ctx context.Context, session *model.ChatSession, cmd service.ConversationCommand, emit func(event string, payload any)) {
	userMessage, perr := s.persistOrReuseUserMessage(ctx, session, cmd, nil)
	if perr != nil {
		emit("error", *perr)
		return
	}

	// run 生命周期:pending → running(UUID 显式生成;ChatRun 无 BeforeCreate 钩子)
	run := &model.ChatRun{
		UUID:          uuid.New(),
		SessionID:     session.ID,
		UserMessageID: userMessage.ID,
		Status:        model.ChatRunStatusPending,
	}
	if cerr := s.chat.CreateRun(ctx, run); cerr != nil {
		// 用户消息已落库,仍发 accepted(无 run_id)保持既有恢复语义
		emit("accepted", ConversationEventPayload{})
		emit("error", turnUnavailable())
		return
	}
	runEmit := withRunID(emit, run.UUID.String())
	if rerr := s.chat.UpdateRunStatus(ctx, run, model.ChatRunStatusRunning); rerr != nil {
		runEmit("accepted", ConversationEventPayload{})
		runEmit("error", turnUnavailable())
		return
	}
	runEmit("accepted", ConversationEventPayload{})

	req, berr := s.BuildOrchestrationRequest(ctx, session, userMessage, run)
	if berr != nil {
		s.settleRun(ctx, run, model.ChatRunStatusFailed)
		runEmit("error", ConversationEventPayload{Content: "道人使用的模型不可用，请更换模型后重试", ErrorCode: "service.chat.model_unavailable", Terminal: true, Recovery: StreamRecoveryPersistedRetry})
		return
	}

	st := &langGraphTurnState{svc: s, ctx: ctx, session: session, run: run, emit: runEmit}
	streamErr := orchestration.NewClient(s.engineBaseURL).Stream(ctx, req, st.consume)
	s.finishLangGraphTurn(ctx, run, cmd, st, streamErr, runEmit)
}

// withRunID 为公共事件载荷注入 run_id(设计 §10/§11:前端以 run_id 定位停止/续跑控件)。
// chunk 不注入(高频增量省带宽);非 run 载荷(prompt_debug/title/struct{}{})原样透传。
func withRunID(emit func(event string, payload any), runID string) func(event string, payload any) {
	return func(event string, payload any) {
		if event == "chunk" {
			emit(event, payload)
			return
		}
		switch p := payload.(type) {
		case ConversationEventPayload:
			p.RunID = runID
			emit(event, p)
		case GroupSpeakerPayload:
			p.RunID = runID
			emit(event, p)
		case GroupTurnDonePayload:
			p.RunID = runID
			emit(event, p)
		default:
			emit(event, payload)
		}
	}
}

// RunConversationResume 续跑 interrupted run(Task 14;设计 §10/§11):按 run 定位会话与
// 用户消息,校验可续状态后以同一事件映射消费 Python Resume 流。不落用户消息、不新建
// run、不发 accepted——前端调起续跑即已知晓轮次上下文。
func (s *Chat) RunConversationResume(ctx context.Context, runUID uuid.UUID, emit func(event string, payload any)) {
	run, rerr := s.chat.TakeRunByUUID(ctx, runUID)
	if rerr != nil {
		emit("error", ConversationEventPayload{Content: "续跑回合不存在", ErrorCode: "service.chat.run_not_found", Terminal: true})
		return
	}
	// 可续状态门(设计 §10 状态机):pending/running(断线重连)/interrupted 可续;终态拒绝
	switch run.Status {
	case model.ChatRunStatusPending, model.ChatRunStatusRunning, model.ChatRunStatusInterrupted:
	default:
		emit("error", ConversationEventPayload{Content: "该回合已结束，无法继续", ErrorCode: "service.chat.run_not_resumable", Terminal: true})
		return
	}
	session, serr := s.chat.TakeSessionByID(ctx, run.SessionID)
	if serr != nil {
		emit("error", ConversationEventPayload{Content: "会话不存在或已删除", ErrorCode: "service.chat.session_not_found", Terminal: true})
		return
	}
	// 用户消息定位与过期校验:run 对应的用户消息仍是会话最新一条(新用户消息即作废旧 run)
	latest, lerr := s.chat.TakeLatestUserMessage(ctx, session.ID)
	if lerr != nil || latest == nil || latest.ID != run.UserMessageID {
		emit("error", ConversationEventPayload{Content: "该回合已结束，无法继续", ErrorCode: "service.chat.run_not_resumable", Terminal: true})
		return
	}
	if uerr := s.chat.UpdateRunStatus(ctx, run, model.ChatRunStatusRunning); uerr != nil {
		emit("error", turnUnavailable())
		return
	}
	runEmit := withRunID(emit, run.UUID.String())
	// 按会话类型续跑:与首轮共用 consume 映射与收尾语义(设计 §11:不重复已完成发言)
	switch session.Type {
	case model.SessionTypeSingle:
		s.resumeSingleConversation(ctx, session, run, latest.Content, runEmit)
	case model.SessionTypeGroup:
		s.resumeGroupConversation(ctx, session, run, latest.Content, runEmit)
	default:
		runEmit("error", ConversationEventPayload{Content: "该会话不支持对话通道", ErrorCode: "service.chat.session_not_found", Terminal: true})
	}
}

// resumeSingleConversation 单聊续跑:复用首轮 consume/收尾,流来自 Python Resume 端点。
func (s *Chat) resumeSingleConversation(ctx context.Context, session *model.ChatSession, run *model.ChatRun, userContent string, runEmit func(string, any)) {
	st := &langGraphTurnState{svc: s, ctx: ctx, session: session, run: run, emit: runEmit}
	streamErr := orchestration.NewClient(s.engineBaseURL).Resume(ctx, run.UUID.String(), st.consume)
	cmd := service.ConversationCommand{SessionUID: session.UUID, Content: userContent}
	s.finishLangGraphTurn(ctx, run, cmd, st, streamErr, runEmit)
}

// resumeGroupConversation 群聊续跑:参与者/模型表按当前成员重建,流来自 Python Resume 端点。
func (s *Chat) resumeGroupConversation(ctx context.Context, session *model.ChatSession, run *model.ChatRun, userContent string, runEmit func(string, any)) {
	members, merr := s.chat.FindMembers(ctx, session.ID)
	if merr != nil {
		runEmit("error", GroupSpeakerPayload{Content: "获取群成员失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: StreamRecoveryResend})
		return
	}
	participants, modelByAgent := groupParticipantTables(members)
	st := &langGraphGroupState{
		svc: s, ctx: ctx, session: session, run: run, emit: runEmit,
		participants: participants, members: members, modelByAgent: modelByAgent,
		userContent: userContent,
		pending:     map[string]bool{}, proposals: map[string]bool{},
	}
	streamErr := orchestration.NewClient(s.engineBaseURL).Resume(ctx, run.UUID.String(), st.consume)
	s.finishLangGraphGroupTurn(ctx, run, st, streamErr, runEmit)
}

// turnUnavailable 引擎/存储暂不可用的稳定错误载荷(可换 persisted_retry)。
func turnUnavailable() ConversationEventPayload {
	return ConversationEventPayload{Content: "暂时无法开始论道，请稍后重试", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: StreamRecoveryPersistedRetry}
}

// langGraphTurnState 一轮内部事件的累计状态(仅 run 内有效,跨 run 不复用)。
type langGraphTurnState struct {
	svc         *Chat
	ctx         context.Context // 请求 ctx;终稿落库经 WithoutCancel 存活于客户端断连
	session     *model.ChatSession
	run         *model.ChatRun
	emit        func(event string, payload any)
	finalSeen   bool
	finalText   string
	interrupted bool
	errored     bool
}

// consume 内部事件→公共事件/持久化(orchestration.Stream 的 emit 回调)。
// assistant_delta→chunk;assistant_final→SaveFinalReplyOnce 幂等落库;
// prompt_debug 原样转发(Python 图的真实模型输入);speaker_started/计划类事件单聊不透传。
func (st *langGraphTurnState) consume(e orchestration.Event) error {
	switch e.Name {
	case "assistant_delta":
		var p struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(e.Payload, &p) == nil && p.Text != "" {
			st.emit("chunk", ConversationEventPayload{Content: p.Text})
		}
	case "assistant_final":
		var p struct {
			ReplyID string `json:"reply_id"`
			Text    string `json:"text"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("编排终稿载荷解析失败: %w", err)
		}
		if p.ReplyID == "" {
			p.ReplyID = "assistant_final"
		}
		replyID := p.ReplyID
		saved, serr := st.svc.chat.SaveFinalReplyOnce(context.WithoutCancel(st.ctx), st.run.UUID, replyID, &model.ChatMessage{
			SessionID: st.session.ID,
			Role:      "assistant",
			Content:   p.Text,
		})
		if serr != nil {
			return fmt.Errorf("编排终稿落库失败: %w", serr)
		}
		st.finalSeen = true
		if saved != nil {
			st.finalText = saved.Content
		}
	case "prompt_debug":
		st.emit("prompt_debug", json.RawMessage(e.Payload))
	case "run_interrupted":
		st.interrupted = true
	case "run_error":
		st.errored = true
	}
	return nil
}

// finishLangGraphTurn 流结束后的终态收尾:按 错误/中断/空终稿/成功 四分支定夺。
// saveCtx 脱离请求取消:收尾(状态/命名/蒸馏)不因客户端断连而丢失。
func (s *Chat) finishLangGraphTurn(ctx context.Context, run *model.ChatRun, cmd service.ConversationCommand, st *langGraphTurnState, streamErr error, emit func(string, any)) {
	saveCtx := context.WithoutCancel(ctx)
	switch {
	case streamErr != nil && errors.Is(streamErr, context.Canceled):
		// 客户端取消:不保留部分回复,run 标记 interrupted(重试建新 run)
		s.settleRun(saveCtx, run, model.ChatRunStatusInterrupted)
		emit("stopped", ConversationEventPayload{})
		return
	case streamErr != nil:
		// 上游异常:不越过 HTTP 边界暴露引擎细节
		s.settleRun(saveCtx, run, model.ChatRunStatusFailed)
		emit("error", turnUnavailable())
		return
	case st.errored:
		s.settleRun(saveCtx, run, model.ChatRunStatusFailed)
		emit("error", turnUnavailable())
		return
	case st.interrupted:
		// Python 判定中断(取消/暂停):run 保留可续跑状态
		s.settleRun(saveCtx, run, model.ChatRunStatusInterrupted)
		emit("stopped", ConversationEventPayload{})
		return
	case !st.finalSeen:
		s.settleRun(saveCtx, run, model.ChatRunStatusFailed)
		emit("error", ConversationEventPayload{Content: "道人未生成有效回复，请稍后重试", ErrorCode: "service.chat.empty_response", Terminal: true, Recovery: StreamRecoveryPersistedRetry})
		return
	}
	s.settleRun(saveCtx, run, model.ChatRunStatusCompleted)
	title := s.GenerateSessionTitle(saveCtx, cmd.SessionUID, cmd.Content, st.finalText)
	if title != "" {
		emit("title", struct {
			Title string `json:"title"`
		}{Title: title})
	}
	if st.session.Agent.MemoryEnabled && st.finalText != "" {
		s.EnqueueMemoryDistillation(saveCtx, service.DistillationSpec{
			SessionUUID: st.session.UUID.String(),
			Model:       st.session.Agent.ModelName,
			UserMessage: cmd.Content,
			Targets: []service.DistillTarget{{
				AgentID: st.session.Agent.ID,
				Messages: []service.DistillMessage{
					{Role: "user", Content: cmd.Content},
					{Role: "assistant", Content: st.finalText},
				},
			}},
		})
	}
	emit("done", ConversationEventPayload{})
}

// persistOrReuseUserMessage 新回合落库用户消息;重试复用最近一条同内容用户消息(legacy 同语义)。
// mentions 非 nil(群聊)时随用户消息落库({"agents":[uuid],"user":bool});单聊传 nil。
// 返回 nil payload=成功;非 nil=应作为公共 error 事件直出的稳定载荷。
func (s *Chat) persistOrReuseUserMessage(ctx context.Context, session *model.ChatSession, cmd service.ConversationCommand, mentions model.JSONMap) (*model.ChatMessage, *ConversationEventPayload) {
	if cmd.Retry {
		latest, err := s.chat.TakeLatestUserMessage(ctx, session.ID)
		if err != nil || latest == nil {
			return nil, &ConversationEventPayload{Content: "无法重试该消息，请重新发送", ErrorCode: "service.chat.retry_unavailable", Terminal: true}
		}
		if latest.Content != cmd.Content {
			return nil, &ConversationEventPayload{Content: "无法重试该消息，请重新发送", ErrorCode: "service.chat.retry_unavailable", Terminal: true}
		}
		return latest, nil
	}
	if mentions != nil {
		if err := s.chat.SaveMessage(ctx, &model.ChatMessage{SessionID: session.ID, Role: "user", Content: cmd.Content, Mentions: mentions}); err != nil {
			return nil, &ConversationEventPayload{Content: "保存消息失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: StreamRecoveryResend}
		}
		// DAO SaveMessage 无返回值,取回已落库消息(run 的 user_message_id 需真实 UUID/ID)
		saved, ferr := s.chat.TakeLatestUserMessage(ctx, session.ID)
		if ferr != nil || saved == nil {
			return nil, &ConversationEventPayload{Content: "保存消息失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: StreamRecoveryResend}
		}
		return saved, nil
	}
	saved, err := s.SaveMessage(ctx, session.ID, "user", cmd.Content)
	if err != nil {
		return nil, &ConversationEventPayload{Content: "保存消息失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: StreamRecoveryResend}
	}
	return saved, nil
}

// settleRun 推进 run 状态;失败仅静默(公共事件与回复落库已就绪,run 行只影响观测/续跑)。
func (s *Chat) settleRun(ctx context.Context, run *model.ChatRun, status string) {
	_ = s.chat.UpdateRunStatus(ctx, run, status)
}
