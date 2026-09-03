// 群聊回合编排器
// 每条用户消息: ≤TurnPlan.MaxRounds 轮 × 逐道人顺序发言;[PASS] 沉默;被@者下轮必答;
// 整轮沉默提前收束;候选按 @点名 > 丹性相关 > 表达欲桶 排序(§7.1 §9)
package chat_service

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alchemy-furnace/server/internal/behavior"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/orchestration"
	"github.com/alchemy-furnace/server/internal/service/turnpolicy"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// passProbeRunes 沉默检测的流式前缀缓冲长度(rune)
const passProbeRunes = 16

// GroupSpeakerPayload speaker_start/speaker_done/error 事件数据
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
}

// GroupTurnDonePayload turn_done 事件数据
type GroupTurnDonePayload struct {
	Spoke          int    `json:"spoke"`
	Reason         string `json:"reason"`
	FailedSpeakers int    `json:"failed_speakers"`
}

// GroupTitlePayload title 事件数据
type GroupTitlePayload struct {
	Title string `json:"title"`
}

// RunGroupTurn 群聊回合编排:落用户消息→≤3轮逐道人发言→自动命名→turn_done
// 单道人失败以 error 事件表达并继续;ctx 取消时保存半截发言并推 stopped
func (s *Chat) RunGroupTurn(ctx context.Context, sessionUID uuid.UUID, content string, emit func(event string, payload any)) {
	s.runGroupTurn(ctx, sessionUID, content, false, emit)
}

// RetryGroupTurn 重试最近一个同内容用户回合。既有 assistant 回复保留为上一次尝试的
// 历史，本次不会重复落用户消息。
func (s *Chat) RetryGroupTurn(ctx context.Context, sessionUID uuid.UUID, content string, emit func(event string, payload any)) {
	s.runGroupTurn(ctx, sessionUID, content, true, emit)
}

func (s *Chat) runGroupTurn(ctx context.Context, sessionUID uuid.UUID, content string, retry bool, emit func(event string, payload any)) {
	// LangGraph 迁移开关(临时,设计 §12):langgraph 下本入口只是兼容委托,
	// Go 不做候选选择/轮次计算/提示词拼装/[PASS] 判定——全部由 Python 图权威决策。
	if orchestrationEngineSelected() {
		s.RunConversation(ctx, service.ConversationCommand{SessionUID: sessionUID, Content: content, Retry: retry, DebugPrompt: promptDebugEnabled(ctx)}, emit)
		return
	}
	preflightRecovery := StreamRecoveryResend
	if retry {
		preflightRecovery = StreamRecoveryPersistedRetry
	}
	session, err := s.chat.TakeSessionByUUID(ctx, sessionUID)
	if err != nil {
		emit("error", GroupSpeakerPayload{Content: "会话不存在或已删除", ErrorCode: "service.chat.session_not_found", Terminal: true, Recovery: preflightRecovery})
		return
	}
	members, err := s.chat.FindMembers(ctx, session.ID)
	if err != nil {
		emit("error", GroupSpeakerPayload{Content: "获取群成员失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: preflightRecovery})
		return
	}
	memberCredentials := make(map[uint]*credential.ModelCredentials, len(members))
	for _, member := range members {
		agent, credentials, validationErr := s.validateChatAgentAccess(ctx, member.Agent.UUID)
		if validationErr != nil {
			emit("error", GroupSpeakerPayload{
				AgentID: member.Agent.UUID.String(), AgentName: member.Agent.Name,
				AgentAvatar: member.Agent.Avatar, Content: validationErr.Error(), ErrorCode: validationErr.GetCode(), Terminal: true, Recovery: preflightRecovery,
			})
			return
		}
		member.Agent = *agent
		memberCredentials[member.AgentID] = credentials
	}

	memberNames := make([]string, 0, len(members))
	for _, m := range members {
		memberNames = append(memberNames, m.Agent.Name)
	}

	// 用户当轮约束 → 回合级 TurnPlan(§8.2):仅取档位边界,成员资格在 letAgentSpeak 内按各自表达欲
	constraints := turnpolicy.ExtractUserTurnConstraints(content)
	turnPlan := turnpolicy.BuildTurnPlan(constraints, turnpolicy.PolicyForProactivity(0), len(members), nil)

	// 新回合落用户消息；重试则复用最近一次同内容用户消息。
	userMentionedNames, userPinged := ParseMentions(content, memberNames)
	var userMessageUUID string // 表达欲桶键的一部分(§7.1)
	if retry {
		latestUser, latestErr := s.chat.TakeLatestUserMessage(ctx, session.ID)
		if latestErr != nil || latestUser.Content != content {
			emit("error", GroupSpeakerPayload{Content: "无法重试该消息，请重新发送", ErrorCode: "service.chat.retry_unavailable", Terminal: true})
			return
		}
		userMessageUUID = latestUser.UUID.String()
	} else {
		if err := s.chat.SaveMessage(ctx, &model.ChatMessage{
			SessionID: session.ID, Role: "user", Content: content,
			Mentions: buildMentionsJSON(members, userMentionedNames, userPinged),
		}); err != nil {
			emit("error", GroupSpeakerPayload{Content: "保存消息失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: StreamRecoveryResend})
			return
		}
		// DAO SaveMessage 无返回值,取回已落库消息的 UUID 作为桶键
		if latest, lerr := s.chat.TakeLatestUserMessage(ctx, session.ID); lerr == nil {
			userMessageUUID = latest.UUID.String()
		}
	}
	emit("accepted", struct{}{})

	// 停止语义:accepted 已发,零引擎调用直接收束(spec §8.2)
	if turnPlan.Stop {
		emit("turn_done", GroupTurnDonePayload{Spoke: 0, Reason: "user_stop"})
		return
	}

	// 必答队列(按被@顺序):用户@的进第 1 轮
	mustOrder := []uint{}
	inMust := map[uint]bool{}
	for _, name := range userMentionedNames {
		for _, m := range members {
			if m.Agent.Name == name && !inMust[m.AgentID] {
				inMust[m.AgentID] = true
				mustOrder = append(mustOrder, m.AgentID)
			}
		}
	}

	// 候选排序(§9):@点名 > 丹性相关(ScoreUserMessageRelevance) > 表达欲桶;
	// profile 缺失(0 分)退化为成员顺序。档案一次取齐供排序与发言共用。
	memberProfiles := make(map[uint]*behavior.DaoistBehaviorProfile, len(members))
	for _, m := range members {
		memberProfiles[m.AgentID] = s.memberProfile(ctx, m.AgentID)
	}
	type scoredMember struct {
		m     *model.SessionMember
		score int
	}
	scored := make([]scoredMember, 0, len(members))
	for _, m := range members {
		if inMust[m.AgentID] {
			continue
		}
		sc := 0
		if p := memberProfiles[m.AgentID]; p != nil {
			sc = behavior.ScoreUserMessageRelevance(content, p)
		}
		scored = append(scored, scoredMember{m: m, score: sc})
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	restOrder := make([]*model.SessionMember, 0, len(scored))
	for _, sm := range scored {
		restOrder = append(restOrder, sm.m)
	}
	// 只有候选超过本轮名额且表达欲桶将其全部过滤时，才强制指定主回答者。
	// 名额足够时保留原有逐成员尝试语义，避免把普通讨论不必要地缩成一人。
	var primaryAgentID uint
	forcePrimary := len(mustOrder) == 0 && len(restOrder) > turnPlan.MaxSpeakers
	if forcePrimary {
		anyEligible := false
		for _, candidate := range restOrder {
			if turnpolicy.WantsToVolunteer(
				turnpolicy.PolicyForProactivity(candidate.Agent.Proactivity),
				session.UUID.String(), candidate.AgentID, userMessageUUID, 1,
			) {
				anyEligible = true
				break
			}
		}
		forcePrimary = !anyEligible
	}
	if forcePrimary {
		primary := restOrder[0]
		primaryAgentID = primary.AgentID
		mustOrder = append(mustOrder, primary.AgentID)
		inMust[primary.AgentID] = true
		restOrder = restOrder[1:]
	}

	totalSpoke := 0
	failedSpeakers := 0
	firstReply := ""      // 首条 assistant 内容(自动命名用)
	replies := []string{} // 去重库(§9.2):整回合累计,≥8 字符且 ≥0.85 相似即收敛
	// 蒸馏目标(§10.3):仅 memory_enabled 且发言成功的成员;模型=首个成功发言人
	distillTargets := []service.DistillTarget{}
	firstSpeakerModel := ""

	for round := 1; round <= turnPlan.MaxRounds; round++ {
		// 发言队列:必答者优先(按被@顺序),其余按相关度/成员顺序(必答者不再重复入列)
		queue := make([]*model.SessionMember, 0, len(members))
		for _, id := range mustOrder {
			for _, m := range members {
				if m.AgentID == id {
					queue = append(queue, m)
				}
			}
		}
		for _, m := range restOrder {
			if !inMust[m.AgentID] {
				queue = append(queue, m)
			}
		}

		spokeThisRound := false
		voluntarySpoke := false // 当前轮是否有人主动发言(非必答)
		convergedTurn := false  // §9.2 去重命中:整回合收敛
		nextOrder := []uint{}
		nextInMust := map[uint]bool{}

		succeededThisRound := 0
		for _, m := range queue {
			if ctx.Err() != nil {
				emit("stopped", struct{}{})
				return
			}
			// 预算收敛(§8.2):累计回复估算超 MaxTurnTokens 不再开新发言人
			if turnPlan.MaxTurnTokens > 0 && estimatedReplyTokens(replies) >= turnPlan.MaxTurnTokens {
				break
			}
			if turnPlan.MaxSpeakers > 0 && succeededThisRound >= turnPlan.MaxSpeakers {
				break
			}
			mustAnswer := inMust[m.AgentID]
			// 表达欲桶(§7.1):候选多于轮内名额时按成员表达欲过滤(名额内全员有机会)
			if !mustAnswer && len(queue) > turnPlan.MaxSpeakers &&
				!turnpolicy.WantsToVolunteer(turnpolicy.PolicyForProactivity(m.Agent.Proactivity), session.UUID.String(), m.AgentID, userMessageUUID, round) {
				continue
			}
			spoke, full, mentionedNames, _, terminal, converged, errored := s.letAgentSpeak(ctx, session, m, members, memberNames, memberCredentials[m.AgentID], mustAnswer, memberProfiles[m.AgentID], turnPlan, constraints, &replies, emit)
			if ctx.Err() != nil {
				emit("stopped", struct{}{})
				return
			}
			if terminal {
				return // 传输中断:整回合终止,不推 turn_done
			}
			if converged {
				convergedTurn = true
				break // §9.2 去重命中:本回合收敛,不再开新发言人
			}
			if errored {
				failedSpeakers++
			}
			if errored && mustAnswer && primaryAgentID == 0 {
				break // §9.4 被点名者失败:显示失败,不静默换人补位
			}
			if !spoke {
				continue
			}
			spokeThisRound = true
			totalSpoke++
			succeededThisRound++
			if !mustAnswer {
				voluntarySpoke = true
			}
			if firstSpeakerModel == "" {
				firstSpeakerModel = memberCredentials[m.AgentID].Model
			}
			if m.Agent.MemoryEnabled {
				distillTargets = append(distillTargets, service.DistillTarget{
					AgentID: m.AgentID,
					Messages: []service.DistillMessage{
						{Role: "user", Content: content},
						{Role: "assistant", Content: full},
					},
				})
			}
			if firstReply == "" {
				firstReply = full
			}
			// 发言里的@ → 下轮必答(第 MaxRounds 轮的@随循环结束自然失效)
			for _, name := range mentionedNames {
				for _, mm := range members {
					if mm.Agent.Name == name && !nextInMust[mm.AgentID] {
						nextInMust[mm.AgentID] = true
						nextOrder = append(nextOrder, mm.AgentID)
					}
				}
			}
		}

		if convergedTurn || !spokeThisRound {
			break // 去重收敛 / 整轮沉默提前收束
		}
		// 链式@收束:本轮全是必答且下轮无新@ → 提前结束(否则按 MaxRounds 上限)
		if !voluntarySpoke && len(nextOrder) == 0 {
			break
		}
		mustOrder, inMust = nextOrder, nextInMust
	}

	// 自动命名:标题仍空且存在首条回复(失败静默)
	if session.Title == "" && firstReply != "" {
		if title := s.generateSessionTitle(ctx, session, members, content, firstReply); title != "" {
			emit("title", GroupTitlePayload{Title: title})
		}
	}

	// 蒸馏:每轮一次、模型=首个成功发言人(spec §10.3;失败/禁用静默)
	if len(distillTargets) > 0 && firstSpeakerModel != "" {
		s.EnqueueMemoryDistillation(ctx, service.DistillationSpec{
			SessionUUID: session.UUID.String(),
			Model:       firstSpeakerModel,
			UserMessage: content,
			Targets:     distillTargets,
		})
	}

	reason := "answered"
	if totalSpoke == 0 {
		reason = "failed"
	}
	emit("turn_done", GroupTurnDonePayload{Spoke: totalSpoke, Reason: reason, FailedSpeakers: failedSpeakers})
	zap.L().Info("[炼丹炉] 群聊回合结束",
		zap.String("session_uuid", sessionUID.String()), zap.Int("spoke", totalSpoke))
}

// buildMentionsJSON 名字列表 → mentions JSON({"agents":[uuid…],"user":bool})
func buildMentionsJSON(members []*model.SessionMember, names []string, user bool) model.JSONMap {
	uuids := make([]string, 0, len(names))
	for _, name := range names {
		for _, m := range members {
			if m.Agent.Name == name {
				uuids = append(uuids, m.Agent.UUID.String())
			}
		}
	}
	return model.JSONMap{"agents": uuids, "user": user}
}

// memberProfile 反序列化道人行为档案(§9.3 身份隔离);缺失/损坏 → nil(退化=仅静态分区)
func (s *Chat) memberProfile(ctx context.Context, agentID uint) *behavior.DaoistBehaviorProfile {
	pattern, err := s.pattern.GetOrBuildPattern(ctx, agentID)
	if err != nil || len(pattern.BehaviorProfile) == 0 {
		return nil
	}
	raw, merr := json.Marshal(pattern.BehaviorProfile)
	if merr != nil {
		return nil
	}
	var p behavior.DaoistBehaviorProfile
	if uerr := json.Unmarshal(raw, &p); uerr != nil {
		return nil
	}
	return &p
}

// estimatedReplyTokens 累计回复 rune 数作为 token 估算(预算收敛用,§8.2)
func estimatedReplyTokens(replies []string) int {
	n := 0
	for _, r := range replies {
		n += utf8.RuneCountInString(r)
	}
	return n
}

// dedupHit 回复与既有回复 bigram Jaccard ≥0.85 即去重命中(§9.2;仅 ≥8 字符参与)
func dedupHit(content string, replies []string) bool {
	if utf8.RuneCountInString(content) < 8 {
		return false
	}
	for _, prev := range replies {
		if behavior.BigramJaccard(prev, content) >= 0.85 {
			return true
		}
	}
	return false
}

// letAgentSpeak 单个道人的一次发言机会
// 沉默([PASS])不开气泡不落库;单道人失败推 error 事件并返回 spoke=false;
// 去重命中(§9.2)返回 converged=true 收束整回合;必答者失败由调用方决定是否补位。
// 每次生成只注入当前发言道人自己的档案(§9.3 身份隔离)。
func (s *Chat) letAgentSpeak(ctx context.Context, session *model.ChatSession, m *model.SessionMember, members []*model.SessionMember, memberNames []string, creds *credential.ModelCredentials, mustAnswer bool, profile *behavior.DaoistBehaviorProfile, turnPlan *turnpolicy.TurnPlan, constraints turnpolicy.UserTurnConstraints, replies *[]string, emit func(event string, payload any)) (spoke bool, full string, mentionedNames []string, pingedUser bool, terminal bool, converged bool, errored bool) {
	pattern, err := s.pattern.GetOrBuildPattern(ctx, m.AgentID)
	if err != nil {
		emit("error", GroupSpeakerPayload{AgentID: m.Agent.UUID.String(), AgentName: m.Agent.Name, AgentAvatar: m.Agent.Avatar, Content: "化丹为性失败，请稍后重试"})
		return false, "", nil, false, false, false, false
	}
	// 成员级 TurnPlan:表达欲取该道人自己的档位(§7.1);名额/轮次继承回合级
	memberPolicy := turnpolicy.PolicyForProactivity(m.Agent.Proactivity)
	var activatedRules []turnpolicy.ActivatedPillRule
	if profile != nil {
		activatedRules = behavior.ActivatePillRules(constraints.LatestQuestion, profile)
	}
	memberPlan := turnpolicy.BuildTurnPlan(constraints, memberPolicy, len(members), activatedRules)
	memberPlan.MustAnswer = mustAnswer
	memberPlan.MaxSpeakers = turnPlan.MaxSpeakers
	memberPlan.MaxRounds = turnPlan.MaxRounds
	// 本地记忆注入(§10.4;memory_enabled 门控,检索失败静默降级为无记忆)
	if m.Agent.MemoryEnabled {
		memberPlan.Memories = s.RetrieveMemories(ctx, m.AgentID, constraints.LatestQuestion)
	}

	systemPrompt := behavior.ComposeSystemPrompt(profile, m.Agent.Name, memberPlan)
	if systemPrompt == "" {
		systemPrompt = pattern.SystemPrompt // 防御:profile 缺失时用缓存提示词
	}
	systemPrompt = BuildGroupSystemPrompt(systemPrompt, m.Agent.Name, m.Agent.Proactivity, memberNames, mustAnswer)

	_, history, err := s.chat.FindMessages(ctx, session.ID, 1, 20)
	if err != nil {
		emit("error", GroupSpeakerPayload{AgentID: m.Agent.UUID.String(), AgentName: m.Agent.Name, AgentAvatar: m.Agent.Avatar, Content: "获取历史消息失败"})
		return false, "", nil, false, false, false, false
	}
	messages := BuildGroupMessages(systemPrompt, history)

	// 带前缀缓冲的沉默检测:先攒 passProbeRunes 个 rune 再决定开气泡还是丢弃
	var probe strings.Builder
	decided := false
	passed := false
	started := false
	identityPayload := func() GroupSpeakerPayload {
		return GroupSpeakerPayload{AgentID: m.Agent.UUID.String(), AgentName: m.Agent.Name, AgentAvatar: m.Agent.Avatar}
	}
	chunkForward := func(chunk string) {
		payload := identityPayload()
		payload.Content = chunk
		emit("chunk", payload)
	}

	// 实时剥前缀:把 LLM 误加的【name】prefix 在 streaming 阶段就丢弃,前端无需重渲染
	// 实现:累计到一个完整「【xxx】」或判定非 prefix 后再决定
	prefixBuf := strings.Builder{}
	prefixDecided := false
	strippedForward := func(chunk string) {
		if prefixDecided {
			chunkForward(chunk)
			return
		}
		prefixBuf.WriteString(chunk)
		cur := prefixBuf.String()
		// 尝试匹配 prefix 模式;若还没匹配完,继续攒
		m := speakerPrefixPattern.FindString(cur)
		if m != "" && len(m) == len(cur) {
			// 整个 buffer 都是 prefix(可能多次重复如【name】【name】),继续攒,看看是否还有更多
			// 当下一个 chunk 来时,若继续匹配 prefix,继续丢弃;若不匹配,转为 forward
			return
		}
		// 不再是纯 prefix(出现了正常字符):把 prefix 段丢弃,正常段开始 forward
		prefixDecided = true
		rest := cur
		if m != "" {
			rest = cur[len(m):]
		}
		if rest != "" {
			chunkForward(rest)
		}
	}
	generationOptions := service.GenerationOptions{MaxTokens: memberPlan.MaxTokens, MaxSentences: memberPlan.MaxSentences}
	if promptDebugEnabled(ctx) {
		emit("prompt_debug", service.NewPromptDebugPayload(
			m.Agent.UUID.String(), m.Agent.Name, creds.Model, messages, generationOptions,
		))
	}
	fullContent, canceled, streamErr := s.StreamChat(ctx, messages, creds, generationOptions, func(chunk string) {
		if passed {
			return // 已判沉默,后续内容全部丢弃
		}
		if decided {
			strippedForward(chunk)
			return
		}
		probe.WriteString(chunk)
		if utf8.RuneCountInString(probe.String()) >= passProbeRunes {
			decided = true
			if IsPass(probe.String()) {
				passed = true
			} else {
				started = true
				emit("speaker_start", identityPayload())
				strippedForward(probe.String())
			}
		}
	})

	// 流结束但前缀不足缓冲长度(短回复):按完整前缀判定
	if !decided && !canceled {
		decided = true
		if IsPass(probe.String()) {
			passed = true
		} else if probe.Len() > 0 {
			// §9.2 去重:短内容在开气泡前整段判定,命中即收敛(不开气泡不落库)
			if dedupHit(StripSpeakerPrefix(probe.String()), *replies) {
				return false, "", nil, false, false, true, false
			}
			started = true
			emit("speaker_start", identityPayload())
			strippedForward(probe.String())
		}
	}

	switch {
	case canceled:
		// 用户叫停:已开气泡的半截内容尽力落库
		if started && fullContent != "" {
			fullContent = StripSpeakerPrefix(fullContent)
			if _, err := s.SaveAgentMessage(context.WithoutCancel(ctx), session.ID, m.AgentID, "assistant", fullContent, nil); err != nil {
				zap.L().Warn("[炼丹炉] 群聊半截发言落库失败", zap.Error(err))
			}
		}
		return false, fullContent, nil, false, false, false, false
	case streamErr != nil:
		payload := identityPayload()
		var interrupted *StreamInterruptedError
		payload.Terminal = stderrors.As(streamErr, &interrupted)
		if payload.Terminal {
			payload.Content = interrupted.Error()
			payload.ErrorCode = interrupted.StreamErrorCode()
			payload.Recovery = StreamRecoveryPersistedRetry
		} else {
			payload.Content = "语言引擎服务异常，请稍后重试"
			payload.ErrorCode = "service.chat.stream_unavailable"
		}
		emit("error", payload)
		return false, "", nil, false, payload.Terminal, false, true
	case passed || fullContent == "":
		if mustAnswer {
			emit("error", GroupSpeakerPayload{
				AgentID: m.Agent.UUID.String(), AgentName: m.Agent.Name, AgentAvatar: m.Agent.Avatar,
				Content: "该道人未生成有效回答", ErrorCode: "service.chat.required_response_empty",
			})
			return false, "", nil, false, false, false, true
		}
		return false, "", nil, false, false, false, false // 自愿沉默/空内容:零痕迹
	}

	fullContent = StripSpeakerPrefix(fullContent)
	// §9.2 去重:≥8 字符且与既有回复 bigram Jaccard ≥0.85 → 丢弃并不再启动新发言人
	if dedupHit(fullContent, *replies) {
		return false, "", nil, false, false, true, false
	}
	*replies = append(*replies, fullContent)
	mentionedNames, pingedUser = ParseMentions(fullContent, memberNames)
	mentions := buildMentionsJSON(members, mentionedNames, pingedUser)
	msg, err := s.SaveAgentMessage(ctx, session.ID, m.AgentID, "assistant", fullContent, mentions)
	if err != nil {
		emit("error", GroupSpeakerPayload{AgentID: m.Agent.UUID.String(), AgentName: m.Agent.Name, AgentAvatar: m.Agent.Avatar, Content: "保存消息失败"})
		return false, "", nil, false, false, false, false
	}
	emit("speaker_done", GroupSpeakerPayload{
		AgentID: m.Agent.UUID.String(), AgentName: m.Agent.Name, AgentAvatar: m.Agent.Avatar,
		MessageID: msg.UUID.String(), Mentions: mentions,
	})
	return true, fullContent, mentionedNames, pingedUser, false, false, false
}

// generateSessionTitle 生成会话标题并落库;title 已非空(用户手改)放弃;任何失败返回 ""
// 单聊: members 传 nil,取 session.Agent.ModelName;群聊:取首成员 ModelName
func (s *Chat) generateSessionTitle(ctx context.Context, session *model.ChatSession, members []*model.SessionMember, userContent, firstReply string) string {
	// 重读再判空,防覆盖用户手动改名
	fresh, err := s.chat.TakeSessionByUUID(ctx, session.UUID)
	if err != nil || fresh.Title != "" {
		return ""
	}
	modelName := ""
	if len(members) > 0 {
		modelName = members[0].Agent.ModelName
	} else if fresh.AgentID != nil {
		modelName = fresh.Agent.ModelName
	}
	creds, rerr := s.ResolveCredentials(ctx, modelName)
	if rerr != nil {
		return ""
	}
	if creds == nil {
		return ""
	}
	reply := firstReply
	if utf8.RuneCountInString(reply) > 200 {
		reply = string([]rune(reply)[:200])
	}
	messages := []map[string]string{{"role": "user", "content": fmt.Sprintf(
		"根据对话开头生成不超过15个字的标题,只输出标题本身,无引号无结尾标点。\n用户:%s\n回复:%s", userContent, reply)}}
	title, cerr := s.callChatCompletion(ctx, messages, creds)
	if cerr != nil {
		zap.L().Warn("[炼丹炉] 自动命名失败", zap.Error(cerr))
		return ""
	}
	if err != nil {
		zap.L().Warn("[炼丹炉] 自动命名失败", zap.Error(err))
		return ""
	}
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "\"'「」『』。,.，!！?？")
	if title == "" {
		return ""
	}
	if utf8.RuneCountInString(title) > 30 {
		title = string([]rune(title)[:30])
	}
	if err := s.chat.UpdateSession(ctx, fresh, map[string]any{"title": title}); err != nil {
		return ""
	}
	return title
}

// GenerateSessionTitle 单聊自动命名入口(公共方法)
func (s *Chat) GenerateSessionTitle(ctx context.Context, sessionUID uuid.UUID, userContent, firstReply string) string {
	session, err := s.chat.TakeSessionByUUID(ctx, sessionUID)
	if err != nil {
		return ""
	}
	return s.generateSessionTitle(ctx, session, nil, userContent, firstReply)
}

// runGroupConversation LangGraph 群聊轮(Task 13,设计 §7/§10):
// 参与者元数据→用户消息(含 mentions)→run 生命周期→内部事件流式映射→收尾。
// Go 侧只做元数据查找与公共契约映射,不选候选/不算轮次/不拼提示词/不判 [PASS]。
func (s *Chat) runGroupConversation(ctx context.Context, session *model.ChatSession, cmd service.ConversationCommand, emit func(event string, payload any)) {
	members, merr := s.chat.FindMembers(ctx, session.ID)
	if merr != nil {
		emit("error", GroupSpeakerPayload{Content: "获取群成员失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: StreamRecoveryResend})
		return
	}
	// 参与者元数据查找表(名称/头像/记忆开关);成员资格与发言顺序由 Python 图权威决策
	participants := make(map[string]*groupParticipant, len(members))
	memberNames := make([]string, 0, len(members))
	for _, m := range members {
		participants[m.Agent.UUID.String()] = &groupParticipant{id: m.Agent.ID, name: m.Agent.Name, avatar: m.Agent.Avatar, memoryEnabled: m.Agent.MemoryEnabled}
		memberNames = append(memberNames, m.Agent.Name)
	}
	mentionedNames, userMentioned := ParseMentions(cmd.Content, memberNames)

	userMessage, perr := s.persistOrReuseUserMessage(ctx, session, cmd, buildMentionsJSON(members, mentionedNames, userMentioned))
	if perr != nil {
		emit("error", GroupSpeakerPayload{Content: perr.Content, ErrorCode: perr.ErrorCode, Terminal: perr.Terminal, Recovery: perr.Recovery})
		return
	}

	// 群聊公共契约无 accepted(发言即反馈)
	run := &model.ChatRun{UUID: uuid.New(), SessionID: session.ID, UserMessageID: userMessage.ID, Status: model.ChatRunStatusPending}
	if cerr := s.chat.CreateRun(ctx, run); cerr != nil {
		emit("error", turnUnavailable())
		return
	}
	if rerr := s.chat.UpdateRunStatus(ctx, run, model.ChatRunStatusRunning); rerr != nil {
		emit("error", turnUnavailable())
		return
	}

	req, berr := s.BuildOrchestrationRequest(ctx, session, userMessage, run)
	if berr != nil {
		s.settleRun(ctx, run, model.ChatRunStatusFailed)
		emit("error", turnUnavailable())
		return
	}
	modelByAgent := make(map[string]string, len(req.Agents))
	for _, a := range req.Agents {
		modelByAgent[a.AgentID] = a.ModelRef.Name
	}

	st := &langGraphGroupState{
		svc: s, ctx: ctx, session: session, run: run, emit: emit,
		participants: participants, members: members, modelByAgent: modelByAgent,
		userContent: cmd.Content,
		pending:     map[string]bool{}, proposals: map[string]bool{},
	}
	streamErr := orchestration.NewClient(s.engineBaseURL).Stream(ctx, req, st.consume)
	s.finishLangGraphGroupTurn(ctx, run, st, streamErr, emit)
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
		agentID := pt.id
		saved, serr := st.svc.chat.SaveFinalReplyOnce(context.WithoutCancel(st.ctx), st.run.UUID, p.ReplyID, &model.ChatMessage{
			UUID:      uuid.New(), // 显式生成:DAO 各实现/幂等返回均携带稳定 MessageID
			SessionID: st.session.ID,
			Role:      "assistant",
			AgentID:   &agentID,
			Content:   p.Text,
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
				AgentID: pt.id,
				Messages: []service.DistillMessage{
					{Role: "user", Content: st.userContent},
					{Role: "assistant", Content: saved.Content},
				},
			})
		}
		messageID := ""
		if saved != nil {
			messageID = saved.UUID.String()
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
			SessionUUID: st.session.UUID.String(),
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
