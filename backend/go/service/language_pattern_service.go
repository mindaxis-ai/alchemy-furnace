// Package service 语言模式缓存业务逻辑层
// 负责缓存/失效/重建每个道人合成后的系统提示词与涌现规则
// 缓存以 source_fingerprint 判断命中：性格 + 排序后的金丹 + 权重
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/alchemy-furnace/server/dao"
	"github.com/alchemy-furnace/server/model"
	"go.uber.org/zap"
)

// LanguagePatternService 语言模式缓存业务逻辑
type LanguagePatternService struct {
	synthesis *SynthesisClient
}

// NewLanguagePatternService 创建语言模式缓存业务实例
func NewLanguagePatternService() *LanguagePatternService {
	return &LanguagePatternService{
		synthesis: NewSynthesisClient(),
	}
}

// loadAgentWithPills 加载道人及其已服用金丹（含权重/顺序），按 sort_order 排序
func (s *LanguagePatternService) loadAgentWithPills(agentID uint) (*model.DaoAgent, []model.AgentPill, error) {
	var agent model.DaoAgent
	if err := dao.GetDB().First(&agent, agentID).Error; err != nil {
		return nil, nil, fmt.Errorf("道人(id=%d)不存在: %w", agentID, err)
	}

	var agentPills []model.AgentPill
	if err := dao.GetDB().Preload("Pill").
		Where("agent_id = ?", agentID).
		Order("sort_order ASC, id ASC").
		Find(&agentPills).Error; err != nil {
		return nil, nil, fmt.Errorf("获取道人服用记录失败: %w", err)
	}

	return &agent, agentPills, nil
}

// computeFingerprint 计算来源指纹：SHA256(personality + 排序后的 pills(id+updated_at+weight) )
func computeFingerprint(agent *model.DaoAgent, agentPills []model.AgentPill) (string, error) {
	type pillFingerprint struct {
		ID        uint    `json:"id"`
		Name      string  `json:"name"`
		Weight    float64 `json:"weight"`
		SortOrder int     `json:"sort_order"`
		UpdatedAt string  `json:"updated_at"`
	}

	parts := make([]pillFingerprint, 0, len(agentPills))
	for _, ap := range agentPills {
		parts = append(parts, pillFingerprint{
			ID:        ap.PillID,
			Name:      ap.Pill.Name,
			Weight:    ap.Weight,
			SortOrder: ap.SortOrder,
			UpdatedAt: ap.Pill.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		})
	}
	// 确保稳定排序（按 sort_order 再按 id）
	sort.SliceStable(parts, func(i, j int) bool {
		if parts[i].SortOrder != parts[j].SortOrder {
			return parts[i].SortOrder < parts[j].SortOrder
		}
		return parts[i].ID < parts[j].ID
	})

	payload := map[string]interface{}{
		"personality": agent.Personality,
		"pills":       parts,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化指纹来源失败: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// InvalidatePattern 将道人的语言模式缓存标记为失效
func (s *LanguagePatternService) InvalidatePattern(agentID uint) error {
	result := dao.GetDB().Model(&model.LanguagePattern{}).
		Where("agent_id = ?", agentID).
		Update("is_valid", false)
	if result.Error != nil {
		return fmt.Errorf("失效语言模式缓存失败: %w", result.Error)
	}
	return nil
}

// InvalidateByPillID 失效所有服用了指定金丹的道人的缓存（金丹内容更新时调用）
func (s *LanguagePatternService) InvalidateByPillID(pillID uint) error {
	var agentIDs []uint
	if err := dao.GetDB().Model(&model.AgentPill{}).
		Where("pill_id = ?", pillID).
		Pluck("agent_id", &agentIDs).Error; err != nil {
		return fmt.Errorf("查询服用金丹(id=%d)的道人失败: %w", pillID, err)
	}
	if len(agentIDs) == 0 {
		return nil
	}
	if err := dao.GetDB().Model(&model.LanguagePattern{}).
		Where("agent_id IN ?", agentIDs).
		Update("is_valid", false).Error; err != nil {
		return fmt.Errorf("批量失效语言模式缓存失败: %w", err)
	}
	zap.L().Info("[炼丹炉] 金丹变化，已失效相关语言模式缓存",
		zap.Uint("pill_id", pillID),
		zap.Int("affected_agents", len(agentIDs)))
	return nil
}

// GetOrBuildPattern 获取道人的语言模式：若缓存命中（is_valid 且指纹一致）直接返回；
// 否则调用 Python 合成引擎重建并写回缓存
func (s *LanguagePatternService) GetOrBuildPattern(agentID uint) (*model.LanguagePattern, error) {
	agent, agentPills, err := s.loadAgentWithPills(agentID)
	if err != nil {
		return nil, err
	}

	fingerprint, err := computeFingerprint(agent, agentPills)
	if err != nil {
		return nil, err
	}

	// 查询现有缓存
	var pattern model.LanguagePattern
	findErr := dao.GetDB().Where("agent_id = ?", agentID).First(&pattern).Error
	if findErr == nil && pattern.IsValid && pattern.SourceFingerprint == fingerprint {
		return &pattern, nil // 缓存命中
	}

	// 缓存未命中，调用 Python 合成引擎
	pills := make([]SynthesisPillInput, 0, len(agentPills))
	for _, ap := range agentPills {
		pills = append(pills, SynthesisPillInput{
			ID:          ap.PillID,
			Name:        ap.Pill.Name,
			Weight:      ap.Weight,
			SortOrder:   ap.SortOrder,
			SkillSchema: ap.Pill.SkillSchema,
		})
	}

	resp, err := s.synthesis.Combine(agent.Personality, pills)
	if err != nil {
		// 合成失败时，若存在旧缓存则降级返回旧缓存（标记失效但可用）
		if findErr == nil {
			zap.L().Warn("[炼丹炉] 语言模式合成失败，降级使用旧缓存",
				zap.Uint("agent_id", agentID), zap.Error(err))
			return &pattern, nil
		}
		return nil, err
	}

	// 写回缓存（upsert）
	tensionsJSON, _ := json.Marshal(resp.InnerTensions)
	innerTensions := model.JSONList{}
	if len(resp.InnerTensions) > 0 {
		var list model.JSONList
		if err := json.Unmarshal(tensionsJSON, &list); err == nil {
			innerTensions = list
		}
	}
	emergenceRules := resp.EmergenceRules
	if emergenceRules == nil {
		emergenceRules = model.JSONList{}
	}

	if findErr == nil {
		// 更新已有记录
		pattern.SystemPrompt = resp.SystemPrompt
		pattern.EmergenceRules = emergenceRules
		pattern.InnerTensions = innerTensions
		pattern.SourceFingerprint = fingerprint
		pattern.IsValid = true
		if err := dao.GetDB().Save(&pattern).Error; err != nil {
			return nil, fmt.Errorf("更新语言模式缓存失败: %w", err)
		}
	} else {
		// 创建新记录
		pattern = model.LanguagePattern{
			AgentID:           agentID,
			SystemPrompt:      resp.SystemPrompt,
			EmergenceRules:    emergenceRules,
			InnerTensions:     innerTensions,
			SourceFingerprint: fingerprint,
			IsValid:           true,
		}
		if err := dao.GetDB().Create(&pattern).Error; err != nil {
			return nil, fmt.Errorf("写入语言模式缓存失败: %w", err)
		}
	}

	zap.L().Info("[炼丹炉] 语言模式合成完成",
		zap.Uint("agent_id", agentID),
		zap.Int("pill_count", len(pills)),
		zap.Int("tension_count", len(resp.InnerTensions)))

	return &pattern, nil
}
