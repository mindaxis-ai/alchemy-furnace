// Package chat_service 对话业务逻辑实现(新架构 internal 分层)
// 处理会话管理、消息存储与 SSE 流式对话;对外以 UUID 标识会话,内部联结用自增 ID。
// 流式对话调用 Python 语言引擎 /api/v1/chat/completions/stream(SSE),错误经 engine 包映射为可读中文。
package chat_service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/engineendpoint"
	ierr "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/engine"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Chat service.Chat 接口实现
type Chat struct {
	chat          dao.Chat
	agent         dao.Agent
	pattern       service.LanguagePatternProvider
	creds         credential.Resolver
	engineBaseURL engineendpoint.Provider
}

// StreamInterruptedError 表示上游 SSE 未按协议完整结束。它只暴露稳定、安全的
// 客户端语义；底层读取错误仅写服务端日志，避免泄露传输或凭证细节。
type StreamInterruptedError struct{}

func (*StreamInterruptedError) Error() string { return "语言引擎连接中断，请重试" }

func (*StreamInterruptedError) StreamErrorCode() string { return "service.chat.stream_interrupted" }

// StreamRecoveryMode 明确终止错误的安全恢复方式，避免客户端按错误码或气泡形态猜测。
type StreamRecoveryMode string

const (
	StreamRecoveryNone           StreamRecoveryMode = "none"
	StreamRecoveryResend         StreamRecoveryMode = "resend"
	StreamRecoveryPersistedRetry StreamRecoveryMode = "persisted_retry"
)

// New 构造固定引擎地址的对话业务实例（Web 与单元测试兼容）。
func New(chat dao.Chat, agent dao.Agent, pattern service.LanguagePatternProvider, creds credential.Resolver, engineBaseURL string) *Chat {
	return NewDynamic(chat, agent, pattern, creds, engineendpoint.Static(engineBaseURL))
}

// NewDynamic 构造运行时读取最新地址的对话业务实例（桌面随机端口场景）。
func NewDynamic(chat dao.Chat, agent dao.Agent, pattern service.LanguagePatternProvider, creds credential.Resolver, engineBaseURL engineendpoint.Provider) *Chat {
	return &Chat{chat: chat, agent: agent, pattern: pattern, creds: creds, engineBaseURL: engineBaseURL}
}

func (s *Chat) validateChatAgentAccess(ctx context.Context, agentUID uuid.UUID) (*model.DaoAgent, *credential.ModelCredentials, ierr.Error) {
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		if err.IsType(ierr.ErrorTypeRecordNotFound) {
			return nil, nil, ierr.New(ierr.ErrorTypeRecordNotFound, "service.chat.agent_not_found", "道人不存在")
		}
		return nil, nil, err.Relation(ierr.ErrorServerInternalError("service.chat.create_take_agent"))
	}
	if agent.Status != "active" {
		return nil, nil, ierr.New(ierr.ErrorTypeInvalidRequest, "service.chat.agent_inactive", "道人已停用")
	}
	if s.creds == nil {
		return nil, nil, ierr.New(ierr.ErrorTypeInvalidRequest, "service.chat.model_unavailable", "道人使用的模型不可用")
	}
	credentials, resolveErr := s.creds.ResolveCredentials(ctx, agent.ModelName)
	if resolveErr != nil || credentials == nil || credentials.APIKey == "" {
		return nil, nil, ierr.New(ierr.ErrorTypeInvalidRequest, "service.chat.model_unavailable", "道人使用的模型不可用")
	}
	return agent, credentials, nil
}

func (s *Chat) validateChatAgent(ctx context.Context, agentUID uuid.UUID) (*model.DaoAgent, ierr.Error) {
	agent, _, err := s.validateChatAgentAccess(ctx, agentUID)
	return agent, err
}

// GetReadiness 分页遍历全部 active 道人并逐个复用正式凭证校验,产出后端权威的就绪名单。
// 单个道人模型不可用只使其缺席名单;仅道人列表读取失败才导致整体失败(5xx 安全文案)。
func (s *Chat) GetReadiness(ctx context.Context) (*service.ChatReadiness, ierr.Error) {
	readiness := &service.ChatReadiness{ReadyAgentIDs: []uuid.UUID{}}
	for page := 1; ; page++ {
		total, agents, err := s.agent.FindAgents(ctx, page, 100, "active")
		if err != nil {
			return nil, err.Relation(ierr.ErrorServerInternalError("service.chat.readiness_list"))
		}
		readiness.ActiveAgentCount = int(total)
		for _, agent := range agents {
			if _, _, verr := s.validateChatAgentAccess(ctx, agent.UUID); verr == nil {
				readiness.ReadyAgentIDs = append(readiness.ReadyAgentIDs, agent.UUID)
			}
		}
		if len(agents) == 0 || int64(page*100) >= total {
			break
		}
	}
	return readiness, nil
}

// CreateSession 创建 1v1 会话;agentUID 为道人对外 UUID
// 标题一律留空,由首个问答自动命名(群聊/单聊统一);group 会话改由 GroupService 路径另建
func (s *Chat) CreateSession(ctx context.Context, agentUID uuid.UUID) (*model.ChatSession, ierr.Error) {
	agent, err := s.validateChatAgent(ctx, agentUID)
	if err != nil {
		return nil, err
	}

	agentID := agent.ID
	session := &model.ChatSession{
		Type:    model.SessionTypeSingle,
		AgentID: &agentID,
		Title:   "", // 标题一律留空,由首个问答自动命名
	}
	if err := s.chat.SaveSession(ctx, session); err != nil {
		return nil, err.Relation(ierr.ErrorServerInternalError("service.chat.create_save"))
	}
	// 填充道人信息供响应 DTO 输出 agent_id(UUID)
	session.Agent = *agent

	zap.L().Info("[炼丹炉] 新的论道会话开启",
		zap.String("session_uuid", session.UUID.String()),
		zap.String("type", session.Type),
		zap.String("agent", agent.Name))
	return session, nil
}

// ListSessions 分页查询会话列表(agentUID 非零值时按道人过滤),按更新时间倒序
func (s *Chat) ListSessions(ctx context.Context, agentUID uuid.UUID, page int, size int) (int64, []*model.ChatSession, ierr.Error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}

	agentID := uint(0)
	if agentUID != uuid.Nil {
		agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
		if err != nil {
			return 0, nil, err.Relation(ierr.ErrorRecordNotFound("service.chat.list_take_agent"))
		}
		agentID = agent.ID
	}
	total, sessions, err := s.chat.FindSessions(ctx, agentID, page, size)
	if err != nil {
		return 0, nil, err
	}
	for _, session := range sessions {
		if session.Type != model.SessionTypeGroup {
			continue
		}
		members, memberErr := s.chat.FindMembers(ctx, session.ID)
		if memberErr != nil {
			return 0, nil, memberErr.Relation(ierr.ErrorServerInternalError("service.chat.list_members"))
		}
		session.Members = make([]model.SessionMember, 0, len(members))
		for _, member := range members {
			session.Members = append(session.Members, *member)
		}
	}
	return total, sessions, nil
}

// GetMessages 从最新消息向前分页，每页内部按时间正序呈现(page=1 为最新一页)
func (s *Chat) GetMessages(ctx context.Context, sessionUID uuid.UUID, page int, size int) (int64, []*model.ChatMessage, ierr.Error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}

	session, err := s.chat.TakeSessionByUUID(ctx, sessionUID)
	if err != nil {
		return 0, nil, err.Relation(ierr.ErrorRecordNotFound("service.chat.get_messages_take"))
	}
	return s.chat.FindMessages(ctx, session.ID, page, size)
}

// TakeLatestUserMessage 查询最新用户消息，供重试校验复用已持久化的用户回合。
func (s *Chat) TakeLatestUserMessage(ctx context.Context, sessionID uint) (*model.ChatMessage, ierr.Error) {
	return s.chat.TakeLatestUserMessage(ctx, sessionID)
}

// GetSessionAgentInfo 按会话 UUID 取会话(预加载道人),供 SSE 构建对话请求
func (s *Chat) GetSessionAgentInfo(ctx context.Context, sessionUID uuid.UUID) (*model.ChatSession, ierr.Error) {
	session, err := s.chat.TakeSessionByUUID(ctx, sessionUID)
	if err != nil {
		return nil, err.Relation(ierr.ErrorRecordNotFound("service.chat.get_session_agent_info"))
	}
	return session, nil
}

// AuthorizeSessionForStream 只用于单聊发送授权；GET 历史不会触发 active/model 校验。
func (s *Chat) AuthorizeSessionForStream(ctx context.Context, session *model.ChatSession) (*credential.ModelCredentials, ierr.Error) {
	if session == nil || session.AgentID == nil || session.Agent.UUID == uuid.Nil {
		return nil, ierr.New(ierr.ErrorTypeInvalidRequest, "service.chat.session_unavailable", "会话信息不可用")
	}
	agent, credentials, err := s.validateChatAgentAccess(ctx, session.Agent.UUID)
	if err != nil {
		return nil, err
	}
	session.Agent = *agent
	return credentials, nil
}

// GetOrBuildPattern 获取道人语言模式(委托 LanguagePatternProvider)
func (s *Chat) GetOrBuildPattern(ctx context.Context, agentID uint) (*model.LanguagePattern, ierr.Error) {
	return s.pattern.GetOrBuildPattern(ctx, agentID)
}

// ResolveCredentials 解析模型调用凭证(委托 credential.Resolver)
func (s *Chat) ResolveCredentials(ctx context.Context, modelName string) (*credential.ModelCredentials, ierr.Error) {
	creds, err := s.creds.ResolveCredentials(ctx, modelName)
	if err != nil {
		return nil, ierr.New(ierr.ErrorTypeServerInternalError, "service.chat.resolve_credentials", err.Error())
	}
	return creds, nil
}

// SaveMessage 写入消息并刷新所属会话 updated_at(sources 字段已废弃,不再写入)
func (s *Chat) SaveMessage(ctx context.Context, sessionID uint, role string, content string) (*model.ChatMessage, ierr.Error) {
	msg := &model.ChatMessage{
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	}
	if err := s.chat.SaveMessage(ctx, msg); err != nil {
		return nil, err.Relation(ierr.ErrorServerInternalError("service.chat.save_message"))
	}
	return msg, nil
}

// DeleteSession 删除会话(消息由 FK CASCADE 清理)
func (s *Chat) DeleteSession(ctx context.Context, sessionUID uuid.UUID) ierr.Error {
	session, err := s.chat.TakeSessionByUUID(ctx, sessionUID)
	if err != nil {
		return err.Relation(ierr.ErrorRecordNotFound("service.chat.delete_take"))
	}
	if err := s.chat.DeleteSession(ctx, session); err != nil {
		return err.Relation(ierr.ErrorServerInternalError("service.chat.delete"))
	}
	zap.L().Info("[炼丹炉] 论道会话已结束", zap.String("session_uuid", sessionUID.String()))
	return nil
}

// UpdateSessionTitle 更新会话标题
func (s *Chat) UpdateSessionTitle(ctx context.Context, sessionUID uuid.UUID, title string) ierr.Error {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > 30 {
		return ierr.New(ierr.ErrorTypeInvalidRequest, "service.chat.title_invalid", "标题需为 1-30 个字符")
	}
	session, err := s.chat.TakeSessionByUUID(ctx, sessionUID)
	if err != nil {
		return err.Relation(ierr.ErrorRecordNotFound("service.chat.update_title_take"))
	}
	if err := s.chat.UpdateSession(ctx, session, map[string]any{"title": title}); err != nil {
		return err.Relation(ierr.ErrorServerInternalError("service.chat.update_title"))
	}
	return nil
}

// ==================== SSE 流式对话 ====================

// StreamChat 调用语言引擎流式对话并逐块回调,返回完整内容与取消标记
//   - 使用 bufio.Reader.ReadBytes('\n') 解析 SSE,无 64KB 行限制
//   - ctx 取消时返回已累积的部分内容与 canceled=true,err 为 nil
//   - 引擎错误经 engine.MapEngineError 映射为可读中文描述;SSE error 事件直接透传其消息
//
// onChunk 每个内容片段回调一次(通常为转发到客户端 SSE)
func (s *Chat) StreamChat(ctx context.Context, messages []map[string]string, creds *credential.ModelCredentials, onChunk func(string)) (fullContent string, canceled bool, err error) {
	stream, callErr := s.callChatStream(ctx, messages, creds)
	if callErr != nil {
		if ctx.Err() != nil {
			return "", true, nil
		}
		return "", false, stderrors.New(engine.MapEngineError(callErr))
	}
	defer stream.Close()

	var full strings.Builder
	reader := bufio.NewReader(stream)
	for {
		line, readErr := reader.ReadBytes('\n')
		if ctx.Err() != nil {
			// 收到停止指令: 返回已累积内容
			return full.String(), true, nil
		}

		text := strings.TrimRight(string(line), "\r\n")
		if strings.HasPrefix(text, "data: ") {
			data := strings.TrimPrefix(text, "data: ")
			if data == "[DONE]" {
				return full.String(), false, nil
			}

			// 解析 SSE JSON(语言引擎直接输出 {"content": "..."} 格式,错误时为 {"error": "..."})
			var chunk struct {
				Content string `json:"content"`
				Error   string `json:"error"`
			}
			if jerr := json.Unmarshal([]byte(data), &chunk); jerr == nil {
				if chunk.Error != "" {
					return full.String(), false, stderrors.New(chunk.Error)
				}
				if chunk.Content != "" {
					full.WriteString(chunk.Content)
					if onChunk != nil {
						onChunk(chunk.Content)
					}
				}
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				return full.String(), false, &StreamInterruptedError{}
			}
			if ctx.Err() != nil || stderrors.Is(readErr, context.Canceled) {
				return full.String(), true, nil
			}
			zap.L().Warn("[炼丹炉] SSE 流读取异常", zap.Error(readErr))
			return full.String(), false, &StreamInterruptedError{}
		}
	}
}

// callChatCompletion 调用 Python 非流式对话接口(标题生成等短任务)
// 返回 content 字段;错误经 engine.MapEngineError 映射
func (s *Chat) callChatCompletion(ctx context.Context, messages []map[string]string, creds *credential.ModelCredentials) (string, error) {
	url := fmt.Sprintf("%s/api/v1/chat/completions", s.engineBaseURL())
	modelName := configuration.Configuration.LLM.DefaultModel
	if creds != nil && creds.Model != "" {
		modelName = creds.Model
	}
	reqBody := map[string]interface{}{"messages": messages, "model": modelName}
	if creds != nil {
		if creds.BaseURL != "" {
			reqBody["base_url"] = creds.BaseURL
		}
		if creds.APIKey != "" {
			reqBody["api_key"] = creds.APIKey
		}
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("构建对话请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", &engine.EngineError{Op: "语言引擎对话接口", StatusCode: 0, Body: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", &engine.EngineError{Op: "语言引擎对话接口", StatusCode: resp.StatusCode, Body: string(body)}
	}
	// Python 端走统一 BaseResponse 包络:{code, message, data:{content, model, usage}}
	var wrapper struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return "", fmt.Errorf("解析对话响应失败: %w", err)
	}
	return wrapper.Data.Content, nil
}

// callChatStream 调用 Python 语言引擎的流式对话接口(SSE),返回响应流
// messages 应已包含合成后的 system 消息;ctx 取消时上游 HTTP 请求随之中断(停止指令贯穿取消链)
// creds 为按请求传递的模型凭证;base_url/api_key 为空时 Python 回退自身环境变量(向后兼容)
func (s *Chat) callChatStream(ctx context.Context, messages []map[string]string, creds *credential.ModelCredentials) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/v1/chat/completions/stream", s.engineBaseURL())

	modelName := configuration.Configuration.LLM.DefaultModel
	if creds != nil && creds.Model != "" {
		modelName = creds.Model
	}

	reqBody := map[string]interface{}{
		"messages": messages,
		"model":    modelName,
	}
	if creds != nil {
		if creds.BaseURL != "" {
			reqBody["base_url"] = creds.BaseURL
		}
		if creds.APIKey != "" {
			reqBody["api_key"] = creds.APIKey
		}
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("构建流式对话请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用语言引擎流式对话接口失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &engine.EngineError{Op: "语言引擎流式接口", StatusCode: resp.StatusCode, Body: string(body)}
	}

	return resp.Body, nil
}

// CreateGroupSession 建群:成员≥2、去重、全部 active;title 置空待自动命名
func (s *Chat) CreateGroupSession(ctx context.Context, agentUIDs []uuid.UUID) (*model.ChatSession, ierr.Error) {
	// 去重(保序)
	seen := map[uuid.UUID]bool{}
	uids := make([]uuid.UUID, 0, len(agentUIDs))
	for _, u := range agentUIDs {
		if !seen[u] {
			seen[u] = true
			uids = append(uids, u)
		}
	}
	if len(uids) < 2 {
		return nil, ierr.New(ierr.ErrorTypeInvalidRequest, "service.chat.group_min_members", "群聊至少需要 2 位道人")
	}

	agents := make([]*model.DaoAgent, 0, len(uids))
	for _, u := range uids {
		a, err := s.validateChatAgent(ctx, u)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}

	session := &model.ChatSession{Type: model.SessionTypeGroup, Title: ""}
	members := make([]*model.SessionMember, 0, len(agents))
	for i, a := range agents {
		// 携带已验证道人,响应直接从成员取 UUID/昵称/状态,无需二次查询
		members = append(members, &model.SessionMember{AgentID: a.ID, SortOrder: i, Agent: *a})
	}
	if err := s.chat.SaveGroupSession(ctx, session, members); err != nil {
		return nil, err.Relation(ierr.ErrorServerInternalError("service.chat.group_save"))
	}
	for _, m := range members {
		session.Members = append(session.Members, *m)
	}
	zap.L().Info("[炼丹炉] 群聊开坛", zap.String("session_uuid", session.UUID.String()), zap.Int("members", len(members)))
	return session, nil
}

// ListMembers 列群成员(按发言顺序)
func (s *Chat) ListMembers(ctx context.Context, sessionUID uuid.UUID) ([]*model.SessionMember, ierr.Error) {
	session, err := s.chat.TakeSessionByUUID(ctx, sessionUID)
	if err != nil {
		return nil, err.Relation(ierr.ErrorRecordNotFound("service.chat.members_take"))
	}
	return s.chat.FindMembers(ctx, session.ID)
}

// AddMembers 邀请入群(已在群的静默跳过),落系统通知消息
func (s *Chat) AddMembers(ctx context.Context, sessionUID uuid.UUID, agentUIDs []uuid.UUID) ierr.Error {
	session, err := s.chat.TakeSessionByUUID(ctx, sessionUID)
	if err != nil {
		return err.Relation(ierr.ErrorRecordNotFound("service.chat.invite_take"))
	}
	if session.Type != model.SessionTypeGroup {
		return ierr.New(ierr.ErrorTypeInvalidRequest, "service.chat.invite_not_group", "仅群聊会话支持邀请成员")
	}
	existing, ferr := s.chat.FindMembers(ctx, session.ID)
	if ferr != nil {
		return ferr.Relation(ierr.ErrorServerInternalError("service.chat.invite_find"))
	}
	inGroup := map[uint]bool{}
	maxSort := -1
	for _, m := range existing {
		inGroup[m.AgentID] = true
		if m.SortOrder > maxSort {
			maxSort = m.SortOrder
		}
	}

	var newMembers []*model.SessionMember
	var names []string
	for _, u := range agentUIDs {
		a, aerr := s.agent.TakeAgentByUUID(ctx, u)
		if aerr != nil || a.Status != "active" {
			return ierr.New(ierr.ErrorTypeInvalidRequest, "service.chat.invite_member_invalid", "邀请的道人不存在或已停用")
		}
		if inGroup[a.ID] {
			continue
		}
		maxSort++
		newMembers = append(newMembers, &model.SessionMember{SessionID: session.ID, AgentID: a.ID, SortOrder: maxSort})
		names = append(names, a.Name)
		inGroup[a.ID] = true
	}
	if len(newMembers) == 0 {
		return nil
	}
	if err := s.chat.SaveMembers(ctx, newMembers); err != nil {
		return err.Relation(ierr.ErrorServerInternalError("service.chat.invite_save"))
	}
	// 系统通知(role=system,不进 LLM 历史,前端居中灰条)
	if _, serr := s.SaveMessage(ctx, session.ID, "system", fmt.Sprintf("你邀请了 %s 入群", strings.Join(names, "、"))); serr != nil {
		zap.L().Warn("[炼丹炉] 写入群通知失败", zap.Error(serr))
	}
	return nil
}

// RemoveMember 踢出群,落系统通知消息
func (s *Chat) RemoveMember(ctx context.Context, sessionUID uuid.UUID, agentUID uuid.UUID) ierr.Error {
	session, err := s.chat.TakeSessionByUUID(ctx, sessionUID)
	if err != nil {
		return err.Relation(ierr.ErrorRecordNotFound("service.chat.kick_take"))
	}
	agent, aerr := s.agent.TakeAgentByUUID(ctx, agentUID)
	if aerr != nil {
		return aerr.Relation(ierr.ErrorRecordNotFound("service.chat.kick_agent"))
	}
	if err := s.chat.DeleteMember(ctx, session.ID, agent.ID); err != nil {
		return err // DAO 已区分 not-found / internal
	}
	if _, serr := s.SaveMessage(ctx, session.ID, "system", fmt.Sprintf("%s 被移出群", agent.Name)); serr != nil {
		zap.L().Warn("[炼丹炉] 写入群通知失败", zap.Error(serr))
	}
	return nil
}

// SaveAgentMessage 写带道人归属与提及的消息(群聊编排器用)
func (s *Chat) SaveAgentMessage(ctx context.Context, sessionID uint, agentID uint, role string, content string, mentions model.JSONMap) (*model.ChatMessage, ierr.Error) {
	aid := agentID
	msg := &model.ChatMessage{
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		AgentID:   &aid,
		Mentions:  mentions,
	}
	if err := s.chat.SaveMessage(ctx, msg); err != nil {
		return nil, err.Relation(ierr.ErrorServerInternalError("service.chat.save_agent_message"))
	}
	return msg, nil
}
