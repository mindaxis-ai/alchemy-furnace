// LangGraph 群聊轮(Task 13;自 group_orchestrator.go 拆出——Task 15 LangGraph 权威化后
// legacy 群编排(轮次规划/候选排序/[PASS]/表达欲桶)已整体删除,本文件只保留权威编排路径)。
// 参与者元数据→用户消息(含 mentions)→run 生命周期→内部事件映射→收尾;
// Go 侧只做元数据查找与公共契约映射,不选候选/不算轮次/不拼提示词/不判 [PASS]。
package chat_service

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/orchestration"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// GroupSpeakerPayload speaker_start/speaker_done/error 事件数据
// RunID 经 withRunID 注入(设计 §10/§11),前端以 run_id 续跑群轮;chunk 不携带。
type GroupSpeakerPayload struct {
	AgentID     string             `json:"agent_id"`
	AgentName   string             `json:"agent_name"`
	AgentAvatar string             `json:"agent_avatar,omitempty"`
	MessageID   string             `json:"message_id,omitempty"`
	Mentions    model.JSONMap      `json:"mentions,omitempty"`
	Content     string             `json:"content,omitempty"`
	ErrorCode   string             `json:"error_code,omitempty"`
	Terminal    bool               `json:"terminal"`
	Recovery    StreamRecoveryMode `json:"recovery,omitempty"`
	RunID       string             `json:"run_id,omitempty"`
}

// GroupTurnDonePayload turn_done 事件数据
type GroupTurnDonePayload struct {
	Spoke          int    `json:"spoke"`
	Reason         string `json:"reason"`
	FailedSpeakers int    `json:"failed_speakers"`
	RunID          string `json:"run_id,omitempty"`
}

// GroupTitlePayload title 事件数据
type GroupTitlePayload struct {
	Title string `json:"title"`
}

// ---------- @提及解析与 mentions 落库纯函数 ----------
// Task 15 从 group_prompt.go/group_orchestrator.go 抢救:legacy 编排已删,
// 但权威群编排路径(runGroupConversation)仍依赖这两个纯函数,故迁入本文件。

// UserAliases @用户 的可识别别名(拉丁别名大小写不敏感)
var UserAliases = []string{"用户", "User"}

// EveryoneAliases @全体成员 的可识别别名(拉丁别名大小写不敏感)
var EveryoneAliases = []string{"全体成员", "所有人", "all", "everyone"}

// mentionPattern @名字 提取:名字到空白/中英文标点为止
var mentionPattern = regexp.MustCompile(`@([^\s@，。,.!?？！:：;；]+)`)

// ParseMentions 从消息文本解析@提及,返回被@的群成员名(去重保序)与是否@了用户
// 不匹配当前成员(含已被踢出者)的@丢弃;@全体成员(EveryoneAliases)展开为全部成员
func ParseMentions(content string, memberNames []string) (agentNames []string, userMentioned bool) {
	inMembers := map[string]bool{}
	for _, n := range memberNames {
		inMembers[n] = true
	}
	seen := map[string]bool{}
	for _, m := range mentionPattern.FindAllStringSubmatch(content, -1) {
		name := m[1]
		if inMembers[name] {
			if !seen[name] {
				seen[name] = true
				agentNames = append(agentNames, name)
			}
			continue
		}
		everyone := false
		for _, alias := range EveryoneAliases {
			if strings.EqualFold(name, alias) {
				everyone = true
				break
			}
		}
		if everyone {
			// @全体成员:全部成员按成员顺序入列(去重)
			for _, n := range memberNames {
				if !seen[n] {
					seen[n] = true
					agentNames = append(agentNames, n)
				}
			}
			continue
		}
		for _, alias := range UserAliases {
			if strings.EqualFold(name, alias) {
				userMentioned = true
				break
			}
		}
	}
	return agentNames, userMentioned
}

// buildMentionsJSON 名字列表 → mentions JSON({"agents":[uuid…],"user":bool})
func buildMentionsJSON(members []*model.SessionMember, names []string, user bool) model.JSONMap {
	uuids := make([]string, 0, len(names))
	for _, name := range names {
		for _, m := range members {
			if m.Agent.Name == name {
				uuids = append(uuids, m.Agent.DaoAgentID)
			}
		}
	}
	return model.JSONMap{"agents": uuids, "user": user}
}

// runGroupConversation LangGraph 群聊轮(Task 13,设计 §7/§10):
// 参与者元数据→用户消息(含 mentions)→run 生命周期→内部事件流式映射→收尾。
// Go 侧只做元数据查找与公共契约映射,不选候选/不算轮次/不拼提示词/不判 [PASS]。
// groupParticipantTables 由群成员构建参与者元数据表与 agent→模型 对照表(首轮/续跑共用)。
// 模型名与 BuildOrchestrationRequest 的 req.Agents 同源(成员 Agent.ModelName);
// 成员资格与发言顺序由 Python 图权威决策。
func groupParticipantTables(members []*model.SessionMember) (map[string]*groupParticipant, map[string]string) {
	participants := make(map[string]*groupParticipant, len(members))
	modelByAgent := make(map[string]string, len(members))
	for _, m := range members {
		participants[m.Agent.DaoAgentID] = &groupParticipant{uid: m.Agent.DaoAgentID, name: m.Agent.Name, avatar: m.Agent.Avatar, memoryEnabled: m.Agent.MemoryEnabled}
		modelByAgent[m.Agent.DaoAgentID] = m.Agent.ModelName
	}
	return participants, modelByAgent
}

func (s *Chat) runGroupConversation(ctx context.Context, session *model.ChatSession, cmd service.ConversationCommand, emit func(event string, payload any)) {
	members, merr := s.chat.FindMembers(ctx, session.ChatSessionID)
	if merr != nil {
		emit("error", GroupSpeakerPayload{Content: "获取群成员失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: StreamRecoveryResend})
		return
	}
	// 参与者元数据查找表(名称/头像/记忆开关)
	participants, modelByAgent := groupParticipantTables(members)
	memberNames := make([]string, 0, len(members))
	for _, m := range members {
		memberNames = append(memberNames, m.Agent.Name)
	}
	mentionedNames, userMentioned := ParseMentions(cmd.Content, memberNames)

	userMessage, perr := s.persistOrReuseUserMessage(ctx, session, cmd, buildMentionsJSON(members, mentionedNames, userMentioned))
	if perr != nil {
		emit("error", GroupSpeakerPayload{Content: perr.Content, ErrorCode: perr.ErrorCode, Terminal: perr.Terminal, Recovery: perr.Recovery})
		return
	}

	// 群聊公共契约无 accepted(发言即反馈)
	userMessageUID := userMessage.ChatMessageID
	run := &model.ChatRun{ChatRunID: uuid.New().String(), SessionID: session.ChatSessionID, UserMessageID: &userMessageUID, Status: model.ChatRunStatusPending}
	if cerr := s.chat.CreateRun(ctx, run); cerr != nil {
		emit("error", turnUnavailable())
		return
	}
	// run 建行后事件切 runEmit:发言/收束事件携带 run_id(Task 14,设计 §10/§11)
	runEmit := withRunID(emit, run.ChatRunID)
	if rerr := s.chat.UpdateRunStatus(ctx, run, model.ChatRunStatusRunning); rerr != nil {
		runEmit("error", turnUnavailable())
		return
	}

	req, berr := s.BuildOrchestrationRequest(ctx, session, userMessage, run, cmd.ModelName)
	if berr != nil {
		s.settleRun(ctx, run, model.ChatRunStatusFailed)
		runEmit("error", turnUnavailable())
		return
	}

	st := &langGraphGroupState{
		svc: s, ctx: ctx, session: session, run: run, emit: runEmit,
		participants: participants, members: members, modelByAgent: modelByAgent,
		userContent: cmd.Content,
		pending:     map[string]bool{}, proposals: map[string]bool{},
	}
	streamErr := orchestration.NewClient(s.engineBaseURL).Stream(ctx, req, st.consume)
	s.finishLangGraphGroupTurn(ctx, run, st, streamErr, runEmit)
}

// langGraphGroupState 一轮群聊内部事件的累计状态(仅 run 内有效,跨 run 不复用)。
type langGraphGroupState struct {
	svc          *Chat
	ctx          context.Context // 请求 ctx;落库/收尾经 WithoutCancel 存活于客户端断连
	session      *model.ChatSession
	run          *model.ChatRun
	emit         func(event string, payload any)
	participants map[string]*groupParticipant
	members      []*model.SessionMember // 自动命名取 members[0].Agent.ModelName
	modelByAgent map[string]string      // agent UUID → 模型名(蒸馏取首位发言人模型)
	userContent  string
	pending      map[string]bool // started-未-final 的 agent_id(下一边界 flush 失败)
	proposals    map[string]bool // proposal_id 幂等表
	finals       int
	firstReply   string
	firstModel   string
	failed       int
	targets      []service.DistillTarget
	interrupted  bool
	errored      bool
}

// consume 内部事件→公共事件/持久化(orchestration.Stream 的 emit 回调):
// speaker_started→speaker_start(先 flush 未终稿的前任);assistant_delta→chunk;
// assistant_final→SaveFinalReplyOnce 幂等落库→speaker_done;prompt_debug 关联发言人转发;
// memory_proposed 四重校验落库;run_interrupted/run_error 只置旗标,收尾统一定夺。
func (st *langGraphGroupState) consume(e orchestration.Event) error {
	switch e.Name {
	case "speaker_started":
		st.flushPending()
		var p struct {
			AgentID string `json:"agent_id"`
		}
		if json.Unmarshal(e.Payload, &p) == nil {
			st.pending[p.AgentID] = true
			if pt, ok := st.participants[p.AgentID]; ok {
				st.emit("speaker_start", GroupSpeakerPayload{AgentID: p.AgentID, AgentName: pt.name, AgentAvatar: pt.avatar})
			}
		}
	case "assistant_delta":
		var p struct {
			AgentID string `json:"agent_id"`
			Text    string `json:"text"`
		}
		if json.Unmarshal(e.Payload, &p) == nil && p.Text != "" {
			if pt, ok := st.participants[p.AgentID]; ok {
				st.emit("chunk", GroupSpeakerPayload{AgentID: p.AgentID, AgentName: pt.name, AgentAvatar: pt.avatar, Content: p.Text})
			}
		}
	case "assistant_final":
		var p struct {
			AgentID string `json:"agent_id"`
			ReplyID string `json:"reply_id"`
			Text    string `json:"text"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("编排终稿载荷解析失败: %w", err)
		}
		pt, ok := st.participants[p.AgentID]
		if !ok {
			return nil // 未知 agent 的终稿不入库(防越权写入)
		}
		if p.ReplyID == "" {
			p.ReplyID = "assistant_final"
		}
		agentUID := pt.uid
		runUID, perr := uuid.Parse(st.run.ChatRunID)
		if perr != nil {
			return fmt.Errorf("编排终稿落库失败: run 标识无效: %w", perr)
		}
		saved, serr := st.svc.chat.SaveFinalReplyOnce(context.WithoutCancel(st.ctx), runUID, p.ReplyID, &model.ChatMessage{
			ChatMessageID: uuid.New().String(), // 显式生成:DAO 各实现/幂等返回均携带稳定 MessageID
			SessionID:     st.session.ChatSessionID,
			Role:          "assistant",
			AgentID:       &agentUID,
			Content:       p.Text,
		})
		if serr != nil {
			return fmt.Errorf("编排终稿落库失败: %w", serr)
		}
		delete(st.pending, p.AgentID)
		st.finals++
		if saved != nil && st.finals == 1 {
			st.firstReply = saved.Content
			st.firstModel = st.modelByAgent[p.AgentID]
		}
		if saved != nil && pt.memoryEnabled {
			st.targets = append(st.targets, service.DistillTarget{
				AgentID: pt.uid,
				Messages: []service.DistillMessage{
					{Role: "user", Content: st.userContent},
					{Role: "assistant", Content: saved.Content},
				},
			})
		}
		messageID := ""
		if saved != nil {
			messageID = saved.ChatMessageID
		}
		st.emit("speaker_done", GroupSpeakerPayload{AgentID: p.AgentID, AgentName: pt.name, AgentAvatar: pt.avatar, MessageID: messageID})
	case "prompt_debug":
		var p struct {
			AgentID  string                 `json:"agent_id"`
			ModelRef orchestration.ModelRef `json:"model_ref"`
			Messages []map[string]string    `json:"messages"`
		}
		if json.Unmarshal(e.Payload, &p) == nil {
			name := ""
			if pt, ok := st.participants[p.AgentID]; ok {
				name = pt.name
			}
			st.emit("prompt_debug", service.NewPromptDebugPayload(p.AgentID, name, p.ModelRef.Name, p.Messages, service.GenerationOptions{}))
		}
	case "memory_proposed":
		var p memoryProposalPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			st.svc.persistMemoryProposal(context.WithoutCancel(st.ctx), st.session, st.run, st.participants, st.proposals, p)
		}
	case "run_interrupted":
		st.interrupted = true
	case "run_error":
		st.errored = true
	}
	return nil
}

// flushPending 把 started-未-final 的发言人按失败收尾:非终态 error(前端可继续),计 failed。
// Python 图逐道人顺序发言且失败静默 continue(无事件),Go 以 started-无-final 判定失败。
func (st *langGraphGroupState) flushPending() {
	for agentID := range st.pending {
		st.failed++
		if pt, ok := st.participants[agentID]; ok {
			st.emit("error", GroupSpeakerPayload{
				AgentID: agentID, AgentName: pt.name, AgentAvatar: pt.avatar,
				Content: "语言引擎服务失败，请稍后重试", ErrorCode: "service.chat.stream_unavailable", Terminal: false,
			})
		}
		delete(st.pending, agentID)
	}
}

// finishLangGraphGroupTurn 流结束后的群轮收尾:错误/中断/成功三分支。
// 成功:自动命名(私有版,模型取首成员)→蒸馏入队(MemoryEnabled 终稿者)→turn_done;
// 中断:stopped 收束,不保留不完整发言(设计 §10);无 turn_done 的错误分支由调用前事件表达。
func (s *Chat) finishLangGraphGroupTurn(ctx context.Context, run *model.ChatRun, st *langGraphGroupState, streamErr error, emit func(string, any)) {
	saveCtx := context.WithoutCancel(ctx)
	switch {
	case streamErr != nil && stderrors.Is(streamErr, context.Canceled):
		s.settleRun(saveCtx, run, model.ChatRunStatusInterrupted)
		emit("stopped", ConversationEventPayload{})
		return
	case streamErr != nil || st.errored:
		st.flushPending()
		s.settleRun(saveCtx, run, model.ChatRunStatusFailed)
		emit("error", turnUnavailable())
		return
	case st.interrupted:
		s.settleRun(saveCtx, run, model.ChatRunStatusInterrupted)
		emit("stopped", ConversationEventPayload{})
		return
	}
	s.settleRun(saveCtx, run, model.ChatRunStatusCompleted)
	if st.firstReply != "" {
		if title := s.generateSessionTitle(saveCtx, st.session, st.members, st.userContent, st.firstReply); title != "" {
			emit("title", GroupTitlePayload{Title: title})
		}
	}
	if len(st.targets) > 0 {
		s.EnqueueMemoryDistillation(saveCtx, service.DistillationSpec{
			SessionUUID: st.session.ChatSessionID,
			Model:       st.firstModel,
			UserMessage: st.userContent,
			Targets:     st.targets,
		})
	}
	reason := "answered"
	if st.finals == 0 {
		reason = "failed"
	}
	emit("turn_done", GroupTurnDonePayload{Spoke: st.finals, Reason: reason, FailedSpeakers: st.failed})
}
