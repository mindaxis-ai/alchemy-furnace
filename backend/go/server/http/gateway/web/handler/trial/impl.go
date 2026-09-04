// Package trial 试丹 HTTP 处理器(新网关;对齐 Luna-CY 模板 handler 分包风格)
// 提供无需创建道人即可临时组合「基础性格 + 金丹」快速预览效果的接口
// 路由: /api/v1/trial/synthesis, /api/v1/trial/chat
package trial

import (
	"strings"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// Trial 试丹处理器
type Trial struct {
	trial service.Trial
}

// New 构造试丹处理器
func New(trial service.Trial) *Trial {
	return &Trial{trial: trial}
}

// ---------- 请求/响应 DTO ----------

// PillInput 试丹请求中的单颗金丹引用(任务 5 消耗品重构后)
// 二选一: recipe_id(+revision_id 指定版本)/ 草稿(name+skill_schema)。
// 试丹是模拟: 不消耗金丹、不写 AgentPillEffect。
type PillInput struct {
	RecipeID    string        `json:"recipe_id"`
	RevisionID  string        `json:"revision_id"`
	Name        string        `json:"name"`
	SkillSchema model.JSONMap `json:"skill_schema"`
	Weight      float64       `json:"weight"`
	SortOrder   int           `json:"sort_order"`
}

// SynthesizeResponse 试丹合成预览响应
type SynthesizeResponse struct {
	SystemPrompt   string                   `json:"system_prompt"`
	EmergenceRules model.JSONList           `json:"emergence_rules"`
	InnerTensions  []synthesis.InnerTension `json:"inner_tensions"`
	Fingerprint    string                   `json:"fingerprint"`
	Model          string                   `json:"model"`
}

// ---------- 解析工具 ----------

// parsePillInputs 将请求金丹引用解析为业务层输入(目标互斥与 UUID 边界在此解析,非法返回 400)
func parsePillInputs(inputs []PillInput) ([]service.TrialPillInput, errors.Error) {
	result := make([]service.TrialPillInput, 0, len(inputs))
	for i, in := range inputs {
		// 版本必须依附丹方(先于目标唯一性检查,给出更精确的错误)
		if in.RevisionID != "" && in.RecipeID == "" {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.trial.revision_requires_recipe",
				"第%d颗金丹: 指定版本必须携带所属丹方 recipe_id", i+1)
		}
		hasRecipe, hasDraft := in.RecipeID != "", in.Name != "" || in.SkillSchema != nil
		targetCount := 0
		for _, has := range []bool{hasRecipe, hasDraft} {
			if has {
				targetCount++
			}
		}
		if targetCount != 1 {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.trial.input_target",
				"第%d颗金丹必须且只能提供 recipe_id(+revision_id) 或草稿之一", i+1)
		}

		item := service.TrialPillInput{Weight: in.Weight, SortOrder: in.SortOrder}
		if hasRecipe {
			rid, err := uuid.Parse(in.RecipeID)
			if err != nil {
				return nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.trial.recipe_id_parse", "第%d颗丹方ID格式不正确", i+1)
			}
			item.RecipeID = rid
			if in.RevisionID != "" {
				vid, err := uuid.Parse(in.RevisionID)
				if err != nil {
					return nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.trial.revision_id_parse", "第%d颗金丹版本ID格式不正确", i+1)
				}
				item.RevisionID = vid
			}
		} else {
			name := strings.TrimSpace(in.Name)
			if name == "" || in.SkillSchema == nil {
				return nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.trial.draft_invalid",
					"第%d颗金丹草稿必须包含名称与 skill_schema", i+1)
			}
			item.Draft = &service.TrialPillDraft{Name: name, SkillSchema: in.SkillSchema}
		}
		result = append(result, item)
	}
	return result, nil
}
