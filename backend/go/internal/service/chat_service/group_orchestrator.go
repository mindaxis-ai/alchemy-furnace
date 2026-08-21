// 群聊回合编排器
// 每条用户消息: ≤3 轮 × 逐道人顺序发言;[PASS] 沉默;被@者下轮必答;整轮沉默提前收束
package chat_service

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// passProbeRunes 沉默检测的流式前缀缓冲长度(rune)
const passProbeRunes = 16

// maxGroupRounds 每条用户消息的最大轮次
const maxGroupRounds = 3

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
	Spoke int `json:"spoke"`
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

	// 新回合落用户消息；重试则复用最近一次同内容用户消息。
	userMentionedNames, userPinged := ParseMentions(content, memberNames)
	if retry {
		latestUser, latestErr := s.chat.TakeLatestUserMessage(ctx, session.ID)
		if latestErr != nil || latestUser.Content != content {
			emit("error", GroupSpeakerPayload{Content: "无法重试该消息，请重新发送", ErrorCode: "service.chat.retry_unavailable", Terminal: true})
			return
		}
	} else {
		if err := s.chat.SaveMessage(ctx, &model.ChatMessage{
			SessionID: session.ID, Role: "user", Content: content,
			Mentions: buildMentionsJSON(members, userMentionedNames, userPinged),
		}); err != nil {
			emit("error", GroupSpeakerPayload{Content: "保存消息失败", ErrorCode: "service.chat.stream_unavailable", Terminal: true, Recovery: StreamRecoveryResend})
			return
		}
	}
	emit("accepted", struct{}{})

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

	totalSpoke := 0
	firstReply := "" // 首条 assistant 内容(自动命名用)

	for round := 1; round <= maxGroupRounds; round++ {
		// 发言队列:必答者优先(按被@顺序),其余按 SortOrder(members 本身已有序)
		queue := make([]*model.SessionMember, 0, len(members))
		for _, id := range mustOrder {
			for _, m := range members {
				if m.AgentID == id {
					queue = append(queue, m)
				}
			}
		}
		for _, m := range members {
			if !inMust[m.AgentID] {
				queue = append(queue, m)
			}
		}

		spokeThisRound := false
		voluntarySpoke := false // 当前轮是否有人主动发言(非必答)
		nextOrder := []uint{}
		nextInMust := map[uint]bool{}

		for _, m := range queue {
			if ctx.Err() != nil {
				emit("stopped", struct{}{})
				return
			}
			mustAnswer := inMust[m.AgentID]
			spoke, full, mentionedNames, _, terminal := s.letAgentSpeak(ctx, session, m, members, memberNames, memberCredentials[m.AgentID], mustAnswer, emit)
			if ctx.Err() != nil {
				emit("stopped", struct{}{})
				return
			}
			if terminal {
				return
			}
			if !spoke {
				continue
			}
			spokeThisRound = true
			totalSpoke++
			if !mustAnswer {
				voluntarySpoke = true
			}
			if firstReply == "" {
				firstReply = full
			}
			// 发言里的@ → 下轮必答(第 3 轮的@随循环结束自然失效)
			for _, name := range mentionedNames {
				for _, mm := range members {
					if mm.Agent.Name == name && !nextInMust[mm.AgentID] {
						nextInMust[mm.AgentID] = true
						nextOrder = append(nextOrder, mm.AgentID)
					}
				}
			}
		}

		if !spokeThisRound {
			break // 整轮沉默提前收束
		}
		// 链式@收束:本轮全是必答且下轮无新@ → 提前结束(否则仍按 3 轮上限)
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

	emit("turn_done", GroupTurnDonePayload{Spoke: totalSpoke})
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

// letAgentSpeak 单个道人的一次发言机会
// 沉默([PASS])不开气泡不落库;单道人失败推 error 事件并返回 spoke=false
func (s *Chat) letAgentSpeak(ctx context.Context, session *model.ChatSession, m *model.SessionMember, members []*model.SessionMember, memberNames []string, creds *credential.ModelCredentials, mustAnswer bool, emit func(event string, payload any)) (spoke bool, full string, mentionedNames []string, pingedUser bool, terminal bool) {
	pattern, err := s.pattern.GetOrBuildPattern(ctx, m.AgentID)
	if err != nil {
		emit("error", GroupSpeakerPayload{AgentID: m.Agent.UUID.String(), AgentName: m.Agent.Name, AgentAvatar: m.Agent.Avatar, Content: "化丹为性失败，请稍后重试"})
		return false, "", nil, false, false
	}
	systemPrompt := BuildGroupSystemPrompt(pattern.SystemPrompt, m.Agent.Name, m.Agent.Proactivity, memberNames, mustAnswer)

	_, history, err := s.chat.FindMessages(ctx, session.ID, 1, 20)
	if err != nil {
		emit("error", GroupSpeakerPayload{AgentID: m.Agent.UUID.String(), AgentName: m.Agent.Name, AgentAvatar: m.Agent.Avatar, Content: "获取历史消息失败"})
		return false, "", nil, false, false
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
	fullContent, canceled, streamErr := s.StreamChat(ctx, messages, creds, func(chunk string) {
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
		return false, fullContent, nil, false, false
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
		return false, "", nil, false, payload.Terminal
	case passed || fullContent == "":
		return false, "", nil, false, false // 沉默/空内容:零痕迹
	}

	fullContent = StripSpeakerPrefix(fullContent)
	mentionedNames, pingedUser = ParseMentions(fullContent, memberNames)
	mentions := buildMentionsJSON(members, mentionedNames, pingedUser)
	msg, err := s.SaveAgentMessage(ctx, session.ID, m.AgentID, "assistant", fullContent, mentions)
	if err != nil {
		emit("error", GroupSpeakerPayload{AgentID: m.Agent.UUID.String(), AgentName: m.Agent.Name, AgentAvatar: m.Agent.Avatar, Content: "保存消息失败"})
		return false, "", nil, false, false
	}
	emit("speaker_done", GroupSpeakerPayload{
		AgentID: m.Agent.UUID.String(), AgentName: m.Agent.Name, AgentAvatar: m.Agent.Avatar,
		MessageID: msg.UUID.String(), Mentions: mentions,
	})
	return true, fullContent, mentionedNames, pingedUser, false
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
