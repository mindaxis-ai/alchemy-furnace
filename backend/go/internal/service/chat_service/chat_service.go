// Package chat_service 对话业务逻辑实现(新架构 internal 分层)
// 处理会话管理、消息存储与 SSE 流式对话;对外以 UUID 标识会话,内部联结用自增 ID。
// 流式对话调用 Python 语言引擎 /api/v1/chat/completions/stream(SSE),错误经 engine 包映射为可读中文。
package chat_service

import (
	"bufio"
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/configuration"
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
	engineBaseURL string
}

// New 构造对话业务实例
func New(chat dao.Chat, agent dao.Agent, pattern service.LanguagePatternProvider, creds credential.Resolver, engineBaseURL string) *Chat {
	return &Chat{chat: chat, agent: agent, pattern: pattern, creds: creds, engineBaseURL: engineBaseURL}
}

// CreateSession 创建 1v1 会话;agentUID 为道人对外 UUID
// 标题一律留空,由首个问答自动命名(群聊/单聊统一);group 会话改由 GroupService 路径另建
func (s *Chat) CreateSession(ctx context.Context, agentUID uuid.UUID) (*model.ChatSession, ierr.Error) {
	agent, err := s.agent.TakeAgentByUUID(ctx, agentUID)
	if err != nil {
		return nil, err.Relation(ierr.ErrorRecordNotFound("service.chat.create_take_agent"))
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
	return s.chat.FindSessions(ctx, agentID, page, size)
}

// GetMessages 分页查询会话消息历史(按时间正序),sessionUID 为会话对外 UUID
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

// GetSessionAgentInfo 按会话 UUID 取会话(预加载道人),供 SSE 构建对话请求
func (s *Chat) GetSessionAgentInfo(ctx context.Context, sessionUID uuid.UUID) (*model.ChatSession, ierr.Error) {
	session, err := s.chat.TakeSessionByUUID(ctx, sessionUID)
	if err != nil {
		return nil, err.Relation(ierr.ErrorRecordNotFound("service.chat.get_session_agent_info"))
	}
	return session, nil
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
				// 流正常结束(未收到 [DONE] 也按完成处理)
				return full.String(), false, nil
			}
			if ctx.Err() != nil || stderrors.Is(readErr, context.Canceled) {
				return full.String(), true, nil
			}
			var netErr net.Error
			if stderrors.As(readErr, &netErr) && netErr.Timeout() {
				return full.String(), false, stderrors.New("语言引擎响应超时，请稍后重试")
			}
			zap.L().Warn("[炼丹炉] SSE 流读取异常", zap.Error(readErr))
			return full.String(), false, nil
		}
	}
}

// callChatStream 调用 Python 语言引擎的流式对话接口(SSE),返回响应流
// messages 应已包含合成后的 system 消息;ctx 取消时上游 HTTP 请求随之中断(停止指令贯穿取消链)
// creds 为按请求传递的模型凭证;base_url/api_key 为空时 Python 回退自身环境变量(向后兼容)
func (s *Chat) callChatStream(ctx context.Context, messages []map[string]string, creds *credential.ModelCredentials) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/v1/chat/completions/stream", s.engineBaseURL)

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
