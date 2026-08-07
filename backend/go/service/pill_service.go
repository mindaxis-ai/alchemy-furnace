// Package service 金丹业务逻辑层
// 处理金丹（语言模式/人格特质技能包）的增删改查
// 金丹基于 nuwa-skill 结构，skill_schema 存储于 JSONB
package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/alchemy-furnace/server/dao"
	"github.com/alchemy-furnace/server/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ErrInvalidSkillSchema 表示 skill_schema 校验失败（缺少 expression_dna 或数组超限）
var ErrInvalidSkillSchema = errors.New("skill_schema 校验失败")

// validateSkillSchema 校验 nuwa-skill 结构化内容：
// - expression_dna 必须存在且为对象
// - mental_models 长度 0-20
// - example_dialogues 长度 0-10
func validateSkillSchema(schema model.JSONMap) error {
	if len(schema) == 0 {
		return fmt.Errorf("%w: skill_schema 不能为空", ErrInvalidSkillSchema)
	}

	dna, ok := schema["expression_dna"]
	if !ok || dna == nil {
		return fmt.Errorf("%w: 缺少 expression_dna", ErrInvalidSkillSchema)
	}
	if dnaMap, ok := dna.(map[string]interface{}); !ok || len(dnaMap) == 0 {
		return fmt.Errorf("%w: expression_dna 必须为非空对象", ErrInvalidSkillSchema)
	}

	if models, ok := schema["mental_models"]; ok && models != nil {
		list, ok := models.([]interface{})
		if !ok {
			return fmt.Errorf("%w: mental_models 必须为数组", ErrInvalidSkillSchema)
		}
		if len(list) > 20 {
			return fmt.Errorf("%w: mental_models 长度不能超过 20", ErrInvalidSkillSchema)
		}
	}

	if dialogues, ok := schema["example_dialogues"]; ok && dialogues != nil {
		list, ok := dialogues.([]interface{})
		if !ok {
			return fmt.Errorf("%w: example_dialogues 必须为数组", ErrInvalidSkillSchema)
		}
		if len(list) > 10 {
			return fmt.Errorf("%w: example_dialogues 长度不能超过 10", ErrInvalidSkillSchema)
		}
	}

	return nil
}

// PillService 金丹业务逻辑
type PillService struct {
	patterns *LanguagePatternService
}

// NewPillService 创建金丹业务实例
func NewPillService() *PillService {
	return &PillService{
		patterns: NewLanguagePatternService(),
	}
}

// ListPills 获取金丹列表，支持分页、关键字搜索与内置过滤
func (s *PillService) ListPills(page, pageSize int, keyword string, isBuiltin *bool) ([]model.ElixirPill, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var pills []model.ElixirPill
	var total int64

	db := dao.GetDB().Model(&model.ElixirPill{})

	// 关键字搜索（名称或描述）
	if keyword != "" {
		db = db.Where("name LIKE ? OR description LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	// 内置金丹过滤
	if isBuiltin != nil {
		db = db.Where("is_builtin = ?", *isBuiltin)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询金丹总数失败: %w", err)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := db.Order("is_builtin DESC, updated_at DESC").Offset(offset).Limit(pageSize).Find(&pills).Error; err != nil {
		return nil, 0, fmt.Errorf("查询金丹列表失败: %w", err)
	}

	return pills, total, nil
}

// GetPill 根据 ID 获取金丹详情
func (s *PillService) GetPill(id uint) (*model.ElixirPill, error) {
	var pill model.ElixirPill
	if err := dao.GetDB().First(&pill, id).Error; err != nil {
		return nil, fmt.Errorf("查询金丹(id=%d)失败: %w", id, err)
	}
	return &pill, nil
}

// CreatePill 创建新的金丹
// skill_schema 必填且必须包含 expression_dna；tags/author/version 可选
func (s *PillService) CreatePill(req *model.CreatePillRequest) (*model.ElixirPill, error) {
	if err := validateSkillSchema(req.SkillSchema); err != nil {
		return nil, err
	}

	version := req.Version
	if version == "" {
		version = "1.0.0"
	}
	tags := req.Tags
	if tags == nil {
		tags = model.JSONList{}
	}

	pill := model.ElixirPill{
		Name:        req.Name,
		Description: req.Description,
		SkillSchema: req.SkillSchema,
		Tags:        tags,
		Author:      req.Author,
		Version:     version,
		IsBuiltin:   false,
	}

	if err := dao.GetDB().Create(&pill).Error; err != nil {
		return nil, fmt.Errorf("创建金丹失败: %w", err)
	}

	zap.L().Info("[炼丹炉] 金丹炼成", zap.String("name", pill.Name), zap.Uint("id", pill.ID))
	return &pill, nil
}

// UpdatePill 更新金丹信息，并失效所有服用该金丹的道人的语言模式缓存
func (s *PillService) UpdatePill(id uint, req *model.UpdatePillRequest) (*model.ElixirPill, error) {
	var pill model.ElixirPill
	if err := dao.GetDB().First(&pill, id).Error; err != nil {
		return nil, fmt.Errorf("金丹(id=%d)不存在: %w", id, err)
	}

	// 只更新非空字段
	updates := map[string]interface{}{"updated_at": time.Now()}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.SkillSchema != nil {
		if err := validateSkillSchema(req.SkillSchema); err != nil {
			return nil, err
		}
		updates["skill_schema"] = req.SkillSchema
	}
	if req.Tags != nil {
		updates["tags"] = req.Tags
	}
	if req.Author != "" {
		updates["author"] = req.Author
	}
	if req.Version != "" {
		updates["version"] = req.Version
	}

	if err := dao.GetDB().Model(&pill).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("更新金丹(id=%d)失败: %w", id, err)
	}

	// 失效服用该金丹的道人的语言模式缓存
	if err := s.patterns.InvalidateByPillID(id); err != nil {
		zap.L().Warn("[炼丹炉] 失效语言模式缓存失败", zap.Uint("pill_id", id), zap.Error(err))
	}

	// 重新查询获取更新后的数据
	if err := dao.GetDB().First(&pill, id).Error; err != nil {
		return nil, fmt.Errorf("查询更新后的金丹失败: %w", err)
	}

	zap.L().Info("[炼丹炉] 金丹信息已更新", zap.Uint("id", pill.ID), zap.String("name", pill.Name))
	return &pill, nil
}

// DeletePill 删除金丹，级联删除服用记录并失效相关道人的语言模式缓存
func (s *PillService) DeletePill(id uint) error {
	db := dao.GetDB()

	// 先获取金丹信息，确认存在
	var pill model.ElixirPill
	if err := db.First(&pill, id).Error; err != nil {
		return fmt.Errorf("金丹(id=%d)不存在: %w", id, err)
	}

	// 先失效服用该金丹的道人的语言模式缓存
	// 注意：必须在删除服用记录之前执行，否则无法找到受影响的道人
	if err := s.patterns.InvalidateByPillID(id); err != nil {
		zap.L().Warn("[炼丹炉] 失效语言模式缓存失败", zap.Uint("pill_id", id), zap.Error(err))
	}

	// 在事务中删除数据库记录（服用记录、金丹），Qdrant 向量已随架构移除，无需清理
	if err := dao.Transaction(func(tx *gorm.DB) error {
		// 删除服用记录
		if err := tx.Where("pill_id = ?", id).Delete(&model.AgentPill{}).Error; err != nil {
			return fmt.Errorf("删除服用记录失败: %w", err)
		}

		// 删除金丹本身
		if err := tx.Delete(&pill).Error; err != nil {
			return fmt.Errorf("删除金丹失败: %w", err)
		}

		return nil
	}); err != nil {
		return err
	}

	zap.L().Info("[炼丹炉] 金丹已销毁", zap.Uint("id", id), zap.String("name", pill.Name))
	return nil
}
