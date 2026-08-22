// Package pill_service 金丹业务逻辑实现(新架构 internal 分层)
// 处理金丹(语言模式/人格特质技能包)的增删改查;skill_schema 校验规则平移自旧 service
package pill_service

import (
	"context"
	"encoding/json"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Pill service.Pill 接口实现
type Pill struct {
	pill dao.Pill
}

// New 构造金丹业务实例
func New(pill dao.Pill) *Pill {
	return &Pill{pill: pill}
}

// ListPills 分页查询金丹列表
func (s *Pill) ListPills(ctx context.Context, page int, size int, keyword string, isBuiltin *bool) (int64, []*model.ElixirPill, errors.Error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	return s.pill.FindPills(ctx, page, size, keyword, isBuiltin)
}

// GetPillByUUID 按 UUID 获取金丹详情
func (s *Pill) GetPillByUUID(ctx context.Context, uid uuid.UUID) (*model.ElixirPill, errors.Error) {
	pill, err := s.pill.TakePillByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.pill.get_by_uuid"))
	}
	return pill, nil
}

// CreatePill 创建金丹
func (s *Pill) CreatePill(ctx context.Context, name string, description string, skillSchema model.JSONMap, tags model.JSONList, author string, version string) (*model.ElixirPill, errors.Error) {
	if err := validateSkillSchema(skillSchema); err != nil {
		return nil, err
	}

	if version == "" {
		version = "1.0.0"
	}
	if tags == nil {
		tags = model.JSONList{}
	}

	pill := &model.ElixirPill{
		Name:        name,
		Description: description,
		SkillSchema: skillSchema,
		Tags:        tags,
		Author:      author,
		Version:     version,
		IsBuiltin:   false,
	}
	if err := s.pill.SavePill(ctx, pill); err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.pill.create"))
	}

	zap.L().Info("[炼丹炉] 金丹炼成", zap.String("name", pill.Name), zap.String("uuid", pill.UUID.String()))
	return pill, nil
}

// UpdatePill 更新金丹并失效相关道人语言模式缓存
func (s *Pill) UpdatePill(ctx context.Context, uid uuid.UUID, name *string, description *string, skillSchema model.JSONMap, tags model.JSONList, author *string, version *string) (*model.ElixirPill, errors.Error) {
	pill, err := s.pill.TakePillByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.pill.update_take"))
	}
	if pill.IsBuiltin {
		return nil, errors.ErrorConflict("service.pill.builtin_readonly", "内置金丹为只读示例，请制作副本后再编辑")
	}

	updates := map[string]any{}
	if name != nil && *name != "" {
		updates["name"] = *name
	}
	if description != nil {
		updates["description"] = *description
	}
	if skillSchema != nil {
		if verr := validateSkillSchema(skillSchema); verr != nil {
			return nil, verr
		}
		updates["skill_schema"] = skillSchema
	}
	if tags != nil {
		updates["tags"] = tags
	}
	if author != nil {
		updates["author"] = *author
	}
	if version != nil && *version != "" {
		updates["version"] = *version
	}

	if len(updates) > 0 {
		if err := s.pill.UpdatePill(ctx, pill, updates); err != nil {
			return nil, err.Relation(errors.ErrorServerInternalError("service.pill.update"))
		}
		s.invalidateByPill(ctx, pill)
	}

	// 重新查询获取更新后的数据
	fresh, err := s.pill.TakePillByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.pill.update_retake"))
	}

	zap.L().Info("[炼丹炉] 金丹信息已更新", zap.String("uuid", uid.String()), zap.String("name", fresh.Name))
	return fresh, nil
}

// DeletePill 删除金丹,级联删除服用记录并失效相关道人语言模式缓存
func (s *Pill) DeletePill(ctx context.Context, uid uuid.UUID) errors.Error {
	pill, err := s.pill.TakePillByUUID(ctx, uid)
	if err != nil {
		return err.Relation(errors.ErrorRecordNotFound("service.pill.delete_take"))
	}
	if pill.IsBuiltin {
		return errors.ErrorConflict("service.pill.builtin_readonly", "内置金丹为只读示例，不可删除")
	}

	// 先失效缓存(必须在删除服用记录之前,否则找不到受影响道人)
	s.invalidateByPill(ctx, pill)

	if err := s.pill.DeletePill(ctx, pill); err != nil {
		return err.Relation(errors.ErrorServerInternalError("service.pill.delete"))
	}

	zap.L().Info("[炼丹炉] 金丹已销毁", zap.String("uuid", uid.String()), zap.String("name", pill.Name))
	return nil
}

// ClonePill 深复制金丹为自定义副本:复制全部元数据与完整 schema/tags,
// 但 UUID 全新、is_builtin=false、名称追加" 副本";副本 JSON 与原丹不共享任何引用
func (s *Pill) ClonePill(ctx context.Context, uid uuid.UUID) (*model.ElixirPill, errors.Error) {
	pill, err := s.pill.TakePillByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.pill.clone_take"))
	}

	schema, cerr := deepCopyMap(pill.SkillSchema)
	if cerr != nil {
		return nil, errors.ErrorServerInternalError("service.pill.clone_schema")
	}
	tags, cerr := deepCopyList(pill.Tags)
	if cerr != nil {
		return nil, errors.ErrorServerInternalError("service.pill.clone_tags")
	}

	clone := &model.ElixirPill{
		UUID:        uuid.New(),
		Name:        pill.Name + " 副本",
		Description: pill.Description,
		SkillSchema: schema,
		Tags:        tags,
		Author:      pill.Author,
		Version:     pill.Version,
		IsBuiltin:   false,
	}
	if err := s.pill.SavePill(ctx, clone); err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.pill.clone"))
	}

	zap.L().Info("[炼丹炉] 金丹副本已制作", zap.String("source_uuid", uid.String()), zap.String("clone_uuid", clone.UUID.String()))
	return clone, nil
}

// deepCopyMap/deepCopyList 经 JSON 往返深复制,保证副本与原丹不共享嵌套引用
// (schema/tags 入库即 JSON 序列化,内容必然 JSON 兼容)
func deepCopyMap(src model.JSONMap) (model.JSONMap, error) {
	raw, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	var out model.JSONMap
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func deepCopyList(src model.JSONList) (model.JSONList, error) {
	raw, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	var out model.JSONList
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// invalidateByPill 失效服用该金丹的全部道人缓存;失败仅告警不阻塞主流程
func (s *Pill) invalidateByPill(ctx context.Context, pill *model.ElixirPill) {
	agentIDs, err := s.pill.FindAgentIDsByPillID(ctx, pill.ID)
	if err != nil {
		zap.L().Warn("[炼丹炉] 查询服用金丹的道人失败", zap.String("pill_uuid", pill.UUID.String()), zap.Error(err))
		return
	}
	if err := s.pill.InvalidateLanguagePatternsByAgentIDs(ctx, agentIDs); err != nil {
		zap.L().Warn("[炼丹炉] 失效语言模式缓存失败", zap.String("pill_uuid", pill.UUID.String()), zap.Error(err))
		return
	}
	if len(agentIDs) > 0 {
		zap.L().Info("[炼丹炉] 金丹变化,已失效相关语言模式缓存",
			zap.String("pill_uuid", pill.UUID.String()),
			zap.Int("affected_agents", len(agentIDs)))
	}
}

// validateSkillSchema 校验 nuwa-skill 结构化内容:
// expression_dna 必须存在且为非空对象;mental_models 长度 0-20;example_dialogues 长度 0-10
func validateSkillSchema(schema model.JSONMap) errors.Error {
	if len(schema) == 0 {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.pill.schema.empty", "skill_schema 校验失败: skill_schema 不能为空")
	}

	dna, ok := schema["expression_dna"]
	if !ok || dna == nil {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.pill.schema.dna_missing", "skill_schema 校验失败: 缺少 expression_dna")
	}
	if dnaMap, ok := dna.(map[string]interface{}); !ok || len(dnaMap) == 0 {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.pill.schema.dna_invalid", "skill_schema 校验失败: expression_dna 必须为非空对象")
	}

	if models, ok := schema["mental_models"]; ok && models != nil {
		list, ok := models.([]interface{})
		if !ok {
			return errors.New(errors.ErrorTypeInvalidRequest, "service.pill.schema.models_type", "skill_schema 校验失败: mental_models 必须为数组")
		}
		if len(list) > 20 {
			return errors.New(errors.ErrorTypeInvalidRequest, "service.pill.schema.models_len", "skill_schema 校验失败: mental_models 长度不能超过 20")
		}
	}

	if dialogues, ok := schema["example_dialogues"]; ok && dialogues != nil {
		list, ok := dialogues.([]interface{})
		if !ok {
			return errors.New(errors.ErrorTypeInvalidRequest, "service.pill.schema.dialogues_type", "skill_schema 校验失败: example_dialogues 必须为数组")
		}
		if len(list) > 10 {
			return errors.New(errors.ErrorTypeInvalidRequest, "service.pill.schema.dialogues_len", "skill_schema 校验失败: example_dialogues 长度不能超过 10")
		}
	}

	return nil
}
