// Package service 对话业务逻辑层
// 处理会话管理和消息存储
// 流式对话的核心逻辑在 handler/chat_handler.go 的 WebSocket 中实现
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alchemy-furnace/server/dao"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/pkg/config"
	"go.uber.org/zap"
)

// ChatService 对话业务逻辑
type ChatService struct {
	engineBaseURL string
}

// NewChatService 创建对话业务实例
func NewChatService() *ChatService {
	return &ChatService{
		engineBaseURL: config.Get().PythonEngine.BaseURL,
	}
}

// CreateSession 创建新的对话会话
func (s *ChatService) CreateSession(req *model.CreateSessionRequest) (*model.ChatSession, error) {
	// 检查道人是否存在
	var agent model.DaoAgent
	if err := dao.GetDB().First(&agent, req.AgentID).Error; err != nil {
		return nil, fmt.Errorf("道人(id=%d)不存在: %w", req.AgentID, err)
	}

	title := req.Title
	if title == "" {
		title = fmt.Sprintf("与 %s 的论道", agent.Name)
	}

	session := model.ChatSession{
		AgentID: req.AgentID,
		Title:   title,
	}

	if err := dao.GetDB().Create(&session).Error; err != nil {
		return nil, fmt.Errorf("创建会话失败: %w", err)
	}

	// 预加载道人信息
	session.Agent = agent

	zap.L().Info("[炼丹炉] 新的论道会话开启",
		zap.Uint("session_id", session.ID),
		zap.String("title", session.Title),
		zap.String("agent", agent.Name))

	return &session, nil
}

// ListSessions 获取会话列表，支持分页
func (s *ChatService) ListSessions(page, pageSize int) ([]model.ChatSession, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var sessions []model.ChatSession
	var total int64

	if err := dao.GetDB().Model(&model.ChatSession{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询会话总数失败: %w", err)
	}

	offset := (page - 1) * pageSize
	if err := dao.GetDB().
		Preload("Agent").
		Order("updated_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&sessions).Error; err != nil {
		return nil, 0, fmt.Errorf("查询会话列表失败: %w", err)
	}

	return sessions, total, nil
}

// GetSession 根据 ID 获取会话详情
func (s *ChatService) GetSession(id uint) (*model.ChatSession, error) {
	var session model.ChatSession
	if err := dao.GetDB().Preload("Agent").Preload("Messages").First(&session, id).Error; err != nil {
		return nil, fmt.Errorf("查询会话(id=%d)失败: %w", id, err)
	}
	return &session, nil
}

// GetMessages 获取会话的消息历史，支持分页（按时间正序排列）
func (s *ChatService) GetMessages(sessionID uint, page, pageSize int) ([]model.ChatMessage, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200 // 消息查询允许更大的分页
	}

	var messages []model.ChatMessage
	var total int64

	db := dao.GetDB().Model(&model.ChatMessage{}).Where("session_id = ?", sessionID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询消息总数失败: %w", err)
	}

	offset := (page - 1) * pageSize
	if err := db.Order("created_at ASC").Offset(offset).Limit(pageSize).Find(&messages).Error; err != nil {
		return nil, 0, fmt.Errorf("查询消息列表失败: %w", err)
	}

	return messages, total, nil
}

// SaveMessage 保存消息到数据库
func (s *ChatService) SaveMessage(sessionID uint, role, content string, sources model.JSONMap) (*model.ChatMessage, error) {
	msg := model.ChatMessage{
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		Sources:   sources,
	}

	if err := dao.GetDB().Create(&msg).Error; err != nil {
		return nil, fmt.Errorf("保存消息失败: %w", err)
	}

	// 更新会话的 updated_at
	dao.GetDB().Model(&model.ChatSession{}).Where("id = ?", sessionID).Update("updated_at", time.Now())

	return &msg, nil
}

// CallChatStream 调用 Python 语言引擎的流式对话接口（SSE），返回响应流
// messages 应已包含合成后的 system 消息（由 LanguagePatternService 提供）
// 此函数由 handler 的 WebSocket 调用，获取 SSE 流后转发给客户端
func (s *ChatService) CallChatStream(messages []map[string]string, modelName string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/v1/chat/completions/stream", s.engineBaseURL)

	reqBody := map[string]interface{}{
		"messages": messages,
		"model":    modelName,
	}
	jsonBody, _ := json.Marshal(reqBody)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("调用语言引擎流式对话接口失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("语言引擎流式接口返回错误: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// GetSessionAgentInfo 获取会话关联的道人信息（ID 与模型名）
// 用于 WebSocket 聊天时构建对话请求
func (s *ChatService) GetSessionAgentInfo(sessionID uint) (agentID uint, modelName string, err error) {
	// 查询会话
	var session model.ChatSession
	if err := dao.GetDB().First(&session, sessionID).Error; err != nil {
		return 0, "", fmt.Errorf("会话(id=%d)不存在: %w", sessionID, err)
	}

	// 查询道人信息
	var agent model.DaoAgent
	if err := dao.GetDB().First(&agent, session.AgentID).Error; err != nil {
		return 0, "", fmt.Errorf("道人(id=%d)不存在: %w", session.AgentID, err)
	}

	modelName = agent.ModelName
	if modelName == "" {
		modelName = config.Get().LLM.DefaultModel
	}

	return session.AgentID, modelName, nil
}

// UpdateSessionTitle 更新会话标题
func (s *ChatService) UpdateSessionTitle(sessionID uint, title string) error {
	if err := dao.GetDB().Model(&model.ChatSession{}).Where("id = ?", sessionID).
		Update("title", title).Error; err != nil {
		return fmt.Errorf("更新会话标题失败: %w", err)
	}
	return nil
}

// DeleteSession 删除会话，级联删除所有消息
func (s *ChatService) DeleteSession(id uint) error {
	var session model.ChatSession
	if err := dao.GetDB().First(&session, id).Error; err != nil {
		return fmt.Errorf("会话(id=%d)不存在: %w", id, err)
	}

	if err := dao.GetDB().Delete(&session).Error; err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}

	zap.L().Info("[炼丹炉] 论道会话已结束", zap.Uint("session_id", id))
	return nil
}
