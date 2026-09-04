package distillation_service

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/distillation"
	appErrors "github.com/alchemy-furnace/server/internal/errors"
	iservice "github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

type Service struct {
	client     distillation.Client
	credential credential.Resolver
	inventory  iservice.PillInventory
}

func New(client distillation.Client, resolver credential.Resolver, inventory iservice.PillInventory) *Service {
	return &Service{client: client, credential: resolver, inventory: inventory}
}

func (s *Service) Distill(ctx context.Context, subject, brief, locale string) (*distillation.Response, appErrors.Error) {
	subject, brief = strings.TrimSpace(subject), strings.TrimSpace(brief)
	if len([]rune(subject)) < 2 || len([]rune(brief)) < 4 {
		return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.distillation.input", "请填写人物或主题，以及至少一句炼制目标")
	}
	if locale != "en" {
		locale = "zh-CN"
	}
	creds, err := s.credential.ResolveSynthesisCredentials(ctx)
	if err != nil {
		return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.distillation.credentials", err.Error())
	}
	result, err := s.client.Distill(ctx, subject, brief, locale, creds)
	if err != nil {
		var remote *distillation.RemoteError
		if stderrors.As(err, &remote) {
			// 透传远端语义: error_code=远端 code,data={stage,retryable,details}。
			// 可重试的远端 503 映射为 ErrorTypeServiceUnavailable(HTTP 503),
			// 不被通用 Wrapper 隐藏成"服务器内部错误"。
			errorType := appErrors.ErrorTypeInvalidRequest
			switch {
			case remote.Status == http.StatusServiceUnavailable:
				errorType = appErrors.ErrorTypeServiceUnavailable
			case remote.Status >= http.StatusInternalServerError:
				errorType = appErrors.ErrorTypeServerInternalError
			}
			return nil, appErrors.NewWithData(errorType, remote.Code, map[string]any{
				"stage":     remote.Stage,
				"retryable": remote.Retryable,
				"details":   remote.Details,
			}, remote.Message)
		}
		return nil, appErrors.New(appErrors.ErrorTypeServerInternalError, "service.distillation.call", err.Error())
	}
	return result, nil
}

// SkillExport 只读导出: 服务端重校验(格式/目标唯一性/字段长度/slug/来源协议/敏感内容),
// 绝不接收或透传 API Key。
// 目标解析(任务 5 消耗品重构后): recipe_id 导出丹方当前版本;recipe_id+revision_id
// 导出指定版本(归属校验,版本必须属于该丹方);skill 结构化模式重校验后透传。
// 接口不删除、不修改丹方;远端可重试错误映射为 503,内容错误映射为 400。
func (s *Service) SkillExport(ctx context.Context, input *distillation.SkillExportInput) (*distillation.ExportResult, appErrors.Error) {
	if input == nil {
		return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.skill_export.input", "缺少导出请求")
	}
	format := strings.TrimSpace(input.Format)
	if format != "codex" && format != "claude" {
		return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.skill_export.format", "format 必须是 codex 或 claude")
	}
	hasRecipe, hasSkill := strings.TrimSpace(input.RecipeID) != "", input.Skill != nil
	targetCount := 0
	for _, has := range []bool{hasRecipe, hasSkill} {
		if has {
			targetCount++
		}
	}
	if targetCount != 1 {
		return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.skill_export.target",
			"必须且只能提供 recipe_id(+revision_id) 或 skill 之一")
	}

	skill := input.Skill
	if hasRecipe {
		rev, aerr := s.resolveExportRevision(ctx, input)
		if aerr != nil {
			return nil, aerr
		}
		skill = projectExportRevision(rev)
	}

	if err := distillation.ValidateExportable(skill); err != nil {
		var v *distillation.ExportValidationError
		if stderrors.As(err, &v) {
			return nil, appErrors.NewWithData(appErrors.ErrorTypeInvalidRequest, "service.skill_export.invalid",
				map[string]any{"field": v.Field, "reason": v.Reason}, err.Error())
		}
		return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.skill_export.invalid", err.Error())
	}

	result, err := s.client.SkillExport(ctx, skill, format)
	if err != nil {
		var remote *distillation.RemoteError
		if stderrors.As(err, &remote) {
			// 透传远端语义: error_code=远端 code,data={stage,retryable,details};
			// 可重试的远端 503 映射为 ErrorTypeServiceUnavailable(HTTP 503)
			errorType := appErrors.ErrorTypeInvalidRequest
			switch {
			case remote.Status == http.StatusServiceUnavailable:
				errorType = appErrors.ErrorTypeServiceUnavailable
			case remote.Status >= http.StatusInternalServerError:
				errorType = appErrors.ErrorTypeServerInternalError
			}
			return nil, appErrors.NewWithData(errorType, remote.Code, map[string]any{
				"stage":     remote.Stage,
				"retryable": remote.Retryable,
				"details":   remote.Details,
			}, remote.Message)
		}
		return nil, appErrors.New(appErrors.ErrorTypeServerInternalError, "service.skill_export.call", err.Error())
	}
	return result, nil
}

// resolveExportRevision 按 recipe_id(+revision_id) 解析导出目标版本:
// 仅 recipe_id → 当前版本;带 revision_id → 指定版本(归属校验,不属于该丹方 → 404)。
func (s *Service) resolveExportRevision(ctx context.Context, input *distillation.SkillExportInput) (*model.PillRecipeRevision, appErrors.Error) {
	recipeUUID, err := uuid.Parse(strings.TrimSpace(input.RecipeID))
	if err != nil {
		return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.skill_export.recipe_id", "非法丹方 ID")
	}
	if strings.TrimSpace(input.RevisionID) == "" {
		return s.currentExportRevision(ctx, recipeUUID)
	}
	revisionUUID, err := uuid.Parse(strings.TrimSpace(input.RevisionID))
	if err != nil {
		return nil, appErrors.New(appErrors.ErrorTypeInvalidRequest, "service.skill_export.revision_id", "非法版本 ID")
	}
	return s.inventory.GetRecipeRevision(ctx, recipeUUID, revisionUUID)
}

// currentExportRevision 读丹方当前版本内容(任意状态可读,归档丹方也可导出)
func (s *Service) currentExportRevision(ctx context.Context, recipeUUID uuid.UUID) (*model.PillRecipeRevision, appErrors.Error) {
	_, rev, aerr := s.inventory.GetRecipe(ctx, recipeUUID)
	if aerr != nil {
		return nil, aerr
	}
	return rev, nil
}

// projectExportRevision 只投影导出所需的规范化字段,绝不序列化整行数据库记录
func projectExportRevision(rev *model.PillRecipeRevision) *distillation.ExportableSkill {
	return &distillation.ExportableSkill{
		Name:          rev.Name,
		Description:   rev.Description,
		SkillSchema:   rev.SkillSchema,
		Tags:          jsonListToStrings(rev.Tags),
		Sources:       make([]distillation.Source, 0), // 空来源必须发 [] 而非 null(Pydantic sources: List 拒绝 null)
		GeneratedAt:   rev.CreatedAt.UTC().Format(time.RFC3339),
		EvidenceLevel: "limited",
	}
}

// jsonListToStrings model.JSONList(interface{} 列表)投影为导出模型使用的字符串列表
func jsonListToStrings(list model.JSONList) []string {
	result := make([]string, 0, len(list))
	for _, item := range list {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}
