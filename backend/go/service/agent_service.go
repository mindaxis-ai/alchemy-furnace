// Package service 道人业务逻辑层
// 处理道人的增删改查，以及道人与金丹的绑定（服用/解除）关系
// 道人是 AI 对话代理，每个道人拥有独特的性格描述（系统提示词）
package service

import (
	"fmt"

	"github.com/alchemy-furnace/server/dao"
	"github.com/alchemy-furnace/server/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AgentService 道人业务逻辑
type AgentService struct {
	patterns *LanguagePatternService
}

// NewAgentService 创建道人业务实例
func NewAgentService() *AgentService {
	return &AgentService{
		patterns: NewLanguagePatternService(),
	}
}

// ListAgents 获取道人列表，支持分页和状态过滤
func (s *AgentService) ListAgents(page, pageSize int, status string) ([]model.DaoAgent, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var agents []model.DaoAgent
	var total int64

	db := dao.GetDB().Model(&model.DaoAgent{})

	// 按状态过滤
	if status != "" {
		db = db.Where("status = ?", status)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询道人总数失败: %w", err)
	}

	offset := (page - 1) * pageSize
	if err := db.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&agents).Error; err != nil {
		return nil, 0, fmt.Errorf("查询道人列表失败: %w", err)
	}

	return agents, total, nil
}

// GetAgent 根据 ID 获取道人详情，包含已服用的金丹列表（按服用顺序）与语言模式缓存
func (s *AgentService) GetAgent(id uint) (*model.DaoAgent, error) {
	var agent model.DaoAgent
	if err := dao.GetDB().
		Preload("AgentPills", func(db *gorm.DB) *gorm.DB {
			return db.Order("agent_pills.sort_order ASC, agent_pills.id ASC")
		}).
		Preload("AgentPills.Pill").
		Preload("LanguagePattern").
		First(&agent, id).Error; err != nil {
		return nil, fmt.Errorf("查询道人(id=%d)失败: %w", id, err)
	}
	return &agent, nil
}

// CreateAgent 创建新的道人
func (s *AgentService) CreateAgent(req *model.CreateAgentRequest) (*model.DaoAgent, error) {
	agent := model.DaoAgent{
		Name:        req.Name,
		Avatar:      req.Avatar,
		Personality: req.Personality,
		ModelName:   req.ModelName,
		Status:      "active",
	}
	// 如果未指定模型，使用默认模型
	if agent.ModelName == "" {
		agent.ModelName = "gpt-4o"
	}

	if err := dao.GetDB().Create(&agent).Error; err != nil {
		return nil, fmt.Errorf("创建道人失败: %w", err)
	}

	zap.L().Info("[炼丹炉] 新道人下山历练", zap.String("name", agent.Name), zap.Uint("id", agent.ID))
	return &agent, nil
}

// UpdateAgent 更新道人信息
func (s *AgentService) UpdateAgent(id uint, req *model.UpdateAgentRequest) (*model.DaoAgent, error) {
	var agent model.DaoAgent
	if err := dao.GetDB().First(&agent, id).Error; err != nil {
		return nil, fmt.Errorf("道人(id=%d)不存在: %w", id, err)
	}

	// 只更新非空字段
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Avatar != "" {
		updates["avatar"] = req.Avatar
	}
	if req.Personality != "" {
		updates["personality"] = req.Personality
	}
	if req.ModelName != "" {
		updates["model_name"] = req.ModelName
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}

	if len(updates) == 0 {
		return &agent, nil // 无更新内容
	}

	if err := dao.GetDB().Model(&agent).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("更新道人(id=%d)失败: %w", id, err)
	}

	// 基础性格变化时，失效语言模式缓存
	if _, ok := updates["personality"]; ok {
		if err := s.patterns.InvalidatePattern(id); err != nil {
			zap.L().Warn("[炼丹炉] 失效语言模式缓存失败", zap.Uint("agent_id", id), zap.Error(err))
		}
	}

	// 重新查询
	if err := dao.GetDB().First(&agent, id).Error; err != nil {
		return nil, err
	}

	zap.L().Info("[炼丹炉] 道人信息已更新", zap.Uint("id", agent.ID), zap.String("name", agent.Name))
	return &agent, nil
}

// DeleteAgent 删除道人，级联删除服用记录和会话
func (s *AgentService) DeleteAgent(id uint) error {
	var agent model.DaoAgent
	if err := dao.GetDB().First(&agent, id).Error; err != nil {
		return fmt.Errorf("道人(id=%d)不存在: %w", id, err)
	}

	if err := dao.GetDB().Delete(&agent).Error; err != nil {
		return fmt.Errorf("删除道人失败: %w", err)
	}

	zap.L().Info("[炼丹炉] 道人已归隐", zap.Uint("id", id), zap.String("name", agent.Name))
	return nil
}

// ---------- 服用金丹（Agent-Pill 绑定） ----------

// BindPill 道人服用金丹，建立绑定关系（含权重与服用顺序）
func (s *AgentService) BindPill(agentID, pillID uint, weight float64, sortOrder int) error {
	// 检查道人是否存在
	var agent model.DaoAgent
	if err := dao.GetDB().First(&agent, agentID).Error; err != nil {
		return fmt.Errorf("道人(id=%d)不存在: %w", agentID, err)
	}

	// 检查金丹是否存在
	var pill model.ElixirPill
	if err := dao.GetDB().First(&pill, pillID).Error; err != nil {
		return fmt.Errorf("金丹(id=%d)不存在: %w", pillID, err)
	}

	// 检查是否已绑定
	var existing model.AgentPill
	result := dao.GetDB().Where("agent_id = ? AND pill_id = ?", agentID, pillID).First(&existing)
	if result.Error == nil {
		return fmt.Errorf("道人已经服用过这枚金丹了")
	}

	// 默认权重
	if weight <= 0 {
		weight = 1.0
	}
	// 默认排在最后
	if sortOrder < 0 {
		sortOrder = 0
	}
	if sortOrder == 0 {
		var maxOrder int
		dao.GetDB().Model(&model.AgentPill{}).
			Where("agent_id = ?", agentID).
			Select("COALESCE(MAX(sort_order), 0)").
			Scan(&maxOrder)
		sortOrder = maxOrder + 1
	}

	// 创建绑定记录
	agentPill := model.AgentPill{
		AgentID:   agentID,
		PillID:    pillID,
		Weight:    weight,
		SortOrder: sortOrder,
	}
	if err := dao.GetDB().Create(&agentPill).Error; err != nil {
		return fmt.Errorf("服用金丹失败: %w", err)
	}

	// 失效语言模式缓存
	if err := s.patterns.InvalidatePattern(agentID); err != nil {
		zap.L().Warn("[炼丹炉] 失效语言模式缓存失败", zap.Uint("agent_id", agentID), zap.Error(err))
	}

	zap.L().Info("[炼丹炉] 道人服用金丹",
		zap.Uint("agent_id", agentID),
		zap.Uint("pill_id", pillID),
		zap.Float64("weight", weight),
		zap.Int("sort_order", sortOrder),
		zap.String("agent_name", agent.Name),
		zap.String("pill_name", pill.Name))
	return nil
}

// UpdateAgentPill 更新服用记录（权重/顺序），并失效缓存
func (s *AgentService) UpdateAgentPill(agentID, pillID uint, weight float64, sortOrder int) error {
	var existing model.AgentPill
	if err := dao.GetDB().Where("agent_id = ? AND pill_id = ?", agentID, pillID).First(&existing).Error; err != nil {
		return fmt.Errorf("道人未服用这枚金丹: %w", err)
	}

	updates := map[string]interface{}{}
	if weight > 0 {
		updates["weight"] = weight
	}
	if sortOrder >= 0 {
		updates["sort_order"] = sortOrder
	}
	if len(updates) == 0 {
		return nil
	}
	if err := dao.GetDB().Model(&existing).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新服用记录失败: %w", err)
	}

	if err := s.patterns.InvalidatePattern(agentID); err != nil {
		zap.L().Warn("[炼丹炉] 失效语言模式缓存失败", zap.Uint("agent_id", agentID), zap.Error(err))
	}
	return nil
}

// UnbindPill 道人解除金丹绑定
func (s *AgentService) UnbindPill(agentID, pillID uint) error {
	result := dao.GetDB().Where("agent_id = ? AND pill_id = ?", agentID, pillID).Delete(&model.AgentPill{})
	if result.Error != nil {
		return fmt.Errorf("解除绑定失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("道人未服用这枚金丹")
	}

	// 失效语言模式缓存
	if err := s.patterns.InvalidatePattern(agentID); err != nil {
		zap.L().Warn("[炼丹炉] 失效语言模式缓存失败", zap.Uint("agent_id", agentID), zap.Error(err))
	}

	zap.L().Info("[炼丹炉] 道人解除金丹绑定", zap.Uint("agent_id", agentID), zap.Uint("pill_id", pillID))
	return nil
}

// ListAgentPills 获取道人已服用的金丹列表
func (s *AgentService) ListAgentPills(agentID uint) ([]model.ElixirPill, error) {
	// 检查道人是否存在
	var agent model.DaoAgent
	if err := dao.GetDB().First(&agent, agentID).Error; err != nil {
		return nil, fmt.Errorf("道人(id=%d)不存在: %w", agentID, err)
	}

	// 查询已服用的金丹（按服用顺序）
	var pills []model.ElixirPill
	err := dao.GetDB().Table("elixir_pills").
		Select("elixir_pills.*").
		Joins("JOIN agent_pills ON agent_pills.pill_id = elixir_pills.id").
		Where("agent_pills.agent_id = ?", agentID).
		Order("agent_pills.sort_order ASC, agent_pills.id ASC").
		Find(&pills).Error
	if err != nil {
		return nil, fmt.Errorf("查询道人服用记录失败: %w", err)
	}

	return pills, nil
}

// GetAgentPillIDs 获取道人已服用金丹的 ID 列表
func (s *AgentService) GetAgentPillIDs(agentID uint) ([]uint, error) {
	var pillIDs []uint
	err := dao.GetDB().Model(&model.AgentPill{}).
		Where("agent_id = ?", agentID).
		Order("sort_order ASC, id ASC").
		Pluck("pill_id", &pillIDs).Error
	if err != nil {
		return nil, fmt.Errorf("获取道人金丹ID列表失败: %w", err)
	}
	return pillIDs, nil
}
