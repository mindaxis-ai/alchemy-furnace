// Package model_service 模型管理业务逻辑实现(新架构 internal 分层)
// 管理 LLM 模型配置:归属供应商的模型 CRUD、两级凭证解析链
// 凭证(base_url/api_key)由供应商持有,模型仅声明模型名与生成参数;UUID 为唯一对外标识
package model_service

import (
	"context"
	"strings"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	iservice "github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ModelService service.Model 接口实现
type ModelService struct {
	model    dao.Model
	provider dao.Provider
}

// New 构造模型业务实例
func New(model dao.Model, provider dao.Provider) *ModelService {
	return &ModelService{model: model, provider: provider}
}

// ---------- 视图转换 ----------

func (s *ModelService) toView(m *model.LLMModel, referencedBy int64) *model.ModelView {
	return &model.ModelView{LLMModel: m, ReferencedBy: referencedBy}
}

// ---------- 校验 ----------

func validateName(name string) errors.Error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.model.name.empty", "模型名不能为空")
	}
	if len(name) > 100 {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.model.name.too_long", "模型名长度不能超过 100 个字符")
	}
	return nil
}

func validateParams(temperature float64, maxTokens int) errors.Error {
	if temperature < 0 || temperature > 2 {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.model.temperature_range", "temperature 必须在 0-2 之间")
	}
	if maxTokens < 1 || maxTokens > 128000 {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.model.max_tokens_range", "max_tokens 必须在 1-128000 之间")
	}
	return nil
}

// ---------- 查询 ----------

// ListModelsByProvider 分页查询指定供应商下的模型列表(含引用计数富视图)
func (s *ModelService) ListModelsByProvider(ctx context.Context, providerUID uuid.UUID, page, size int) ([]*model.ModelView, errors.Error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	if size > 500 {
		size = 500
	}

	p, err := s.provider.TakeProviderByUUID(ctx, providerUID)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.model.list_take_provider"))
	}

	_, models, err := s.model.FindModelsByProvider(ctx, p.ID, page, size)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.model.list"))
	}

	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.Name)
	}
	counts, cerr := s.model.CountAgentReferencesByNames(ctx, names)
	if cerr != nil {
		return nil, cerr.Relation(errors.ErrorServerInternalError("service.model.list_refs"))
	}

	views := make([]*model.ModelView, 0, len(models))
	for _, m := range models {
		m.Provider = *p
		views = append(views, s.toView(m, counts[m.Name]))
	}
	return views, nil
}

// GetModelByUUID 按 UUID 获取模型详情(含引用计数)
func (s *ModelService) GetModelByUUID(ctx context.Context, uid uuid.UUID) (*model.ModelView, errors.Error) {
	m, err := s.model.TakeModelByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.model.get"))
	}
	count, cerr := s.model.CountAgentReferencesByName(ctx, m.Name)
	if cerr != nil {
		return nil, cerr.Relation(errors.ErrorServerInternalError("service.model.get_count"))
	}
	return s.toView(m, count), nil
}

// ---------- 写入 ----------

// CreateModel 在指定供应商下创建模型;供应商停用返回 ErrorInvalidRequest
// is_default / is_synthesis / is_fusion 为 true 时事务内清除其他记录的同字段
func (s *ModelService) CreateModel(ctx context.Context, providerUID uuid.UUID, name, displayName string, temperature float64, maxTokens int, isEnabled, isDefault, isSynthesis, isFusion bool, sortOrder int) (*model.ModelView, errors.Error) {
	p, err := s.provider.TakeProviderByUUID(ctx, providerUID)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.model.create_take_provider"))
	}
	if !p.IsEnabled {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.model.provider_disabled", "供应商「%s」已停用，请先启用后再添加模型", p.DisplayName)
	}

	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return nil, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.model.display_name.empty", "显示名不能为空")
	}

	if temperature == 0 {
		temperature = 0.7
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}
	if err := validateParams(temperature, maxTokens); err != nil {
		return nil, err
	}

	exists, err := s.model.ModelNameExistsInProvider(ctx, p.ID, name, 0)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.model.create_name_check"))
	}
	if exists {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.model.name.exists", "该供应商下模型名「%s」已存在", name)
	}

	m := &model.LLMModel{
		ProviderID:  p.ID,
		Name:        name,
		DisplayName: displayName,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		IsEnabled:   isEnabled,
		IsDefault:   isDefault,
		IsSynthesis: isSynthesis,
		IsFusion:    isFusion,
		SortOrder:   sortOrder,
	}
	if err := s.model.SaveModel(ctx, m); err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.model.create"))
	}

	m.Provider = *p
	zap.L().Info("[炼丹炉] 新模型入炉",
		zap.String("uuid", m.UUID.String()),
		zap.String("name", m.Name),
		zap.String("provider", p.Name))
	return s.toView(m, 0), nil
}

// UpdateModel 按 UUID 部分更新模型;is_default/is_synthesis/is_fusion 为 true 时事务内清除其他记录
func (s *ModelService) UpdateModel(ctx context.Context, uid uuid.UUID, name, displayName *string, temperature *float64, maxTokens *int, isEnabled, isDefault, isSynthesis, isFusion *bool, sortOrder *int) (*model.ModelView, errors.Error) {
	m, err := s.model.TakeModelByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.model.update_take"))
	}

	updates := map[string]any{}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if err := validateName(trimmed); err != nil {
			return nil, err
		}
		exists, cerr := s.model.ModelNameExistsInProvider(ctx, m.ProviderID, trimmed, m.ID)
		if cerr != nil {
			return nil, cerr.Relation(errors.ErrorServerInternalError("service.model.update_name_check"))
		}
		if exists {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.model.name.exists", "该供应商下模型名「%s」已存在", trimmed)
		}
		updates["name"] = trimmed
	}
	if displayName != nil {
		trimmed := strings.TrimSpace(*displayName)
		if trimmed == "" {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.model.display_name.empty", "显示名不能为空")
		}
		updates["display_name"] = trimmed
	}

	// 校验生效后的温度/max_tokens 范围
	effTemp := m.Temperature
	if temperature != nil {
		effTemp = *temperature
	}
	effMax := m.MaxTokens
	if maxTokens != nil {
		effMax = *maxTokens
	}
	if err := validateParams(effTemp, effMax); err != nil {
		return nil, err
	}

	if temperature != nil {
		updates["temperature"] = *temperature
	}
	if maxTokens != nil {
		updates["max_tokens"] = *maxTokens
	}
	if isEnabled != nil {
		updates["is_enabled"] = *isEnabled
	}
	if isDefault != nil {
		updates["is_default"] = *isDefault // bool 值,DAO 事务内据此清除其他记录
	}
	if isSynthesis != nil {
		updates["is_synthesis"] = *isSynthesis
	}
	if isFusion != nil {
		updates["is_fusion"] = *isFusion
	}
	if sortOrder != nil {
		updates["sort_order"] = *sortOrder
	}

	if len(updates) > 0 {
		if err := s.model.UpdateModel(ctx, m, updates); err != nil {
			return nil, err.Relation(errors.ErrorServerInternalError("service.model.update"))
		}
	}

	fresh, err := s.model.TakeModelByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.model.update_retake"))
	}
	count, cerr := s.model.CountAgentReferencesByName(ctx, fresh.Name)
	if cerr != nil {
		return nil, cerr.Relation(errors.ErrorServerInternalError("service.model.update_count"))
	}

	zap.L().Info("[炼丹炉] 模型配置已更新", zap.String("uuid", uid.String()), zap.String("name", fresh.Name))
	return s.toView(fresh, count), nil
}

// DeleteModel 按 UUID 删除模型;被道人引用时返回 409(携带 referenced_by)
func (s *ModelService) DeleteModel(ctx context.Context, uid uuid.UUID) errors.Error {
	m, err := s.model.TakeModelByUUID(ctx, uid)
	if err != nil {
		return err.Relation(errors.ErrorRecordNotFound("service.model.delete_take"))
	}

	count, cerr := s.model.CountAgentReferencesByName(ctx, m.Name)
	if cerr != nil {
		return cerr.Relation(errors.ErrorServerInternalError("service.model.delete_count"))
	}
	if count > 0 {
		return errors.ErrorConflictWithData(
			"service.model.delete_referenced",
			map[string]any{"referenced_by": count},
			"该模型仍被 %d 个道人引用，无法删除",
			count,
		)
	}

	if err := s.model.DeleteModel(ctx, m); err != nil {
		return err.Relation(errors.ErrorServerInternalError("service.model.delete"))
	}

	zap.L().Info("[炼丹炉] 模型配置已移除", zap.String("uuid", uid.String()), zap.String("name", m.Name))
	return nil
}

// Options 已启用供应商下的已启用模型精简列表(道人表单下拉)
func (s *ModelService) Options(ctx context.Context) ([]model.LLMModelOption, errors.Error) {
	return s.model.FindEnabledOptions(ctx)
}

// ---------- 两级凭证解析链(model -> provider -> 解密) ----------

// resolveCredentials 由已启用模型解析完整调用凭证:校验供应商启用 -> 解密 api_key
func (s *ModelService) resolveCredentials(ctx context.Context, m *model.LLMModel) (*credential.ModelCredentials, errors.Error) {
	if !m.Provider.IsEnabled {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.model.provider_disabled", "该模型所属供应商已停用")
	}
	apiKey, err := credential.DecryptAPIKey(m.Provider.APIKeyEncrypted)
	if err != nil {
		return nil, errors.ErrorServerInternalError("service.model.decrypt")
	}
	return &credential.ModelCredentials{Model: m.Name, BaseURL: m.Provider.BaseURL, APIKey: apiKey}, nil
}

// ResolveCredentials 解析对话模型调用凭证;name 空时取默认模型
//   - 同名模型存在于多个供应商时,取 sort_order,id 最小者并记录 warning
//   - 找到已启用模型:返回供应商凭证
//   - 找到但全部停用:返回明确错误
//   - 未找到:返回仅含模型名的空凭证(Python 回退环境变量),向后兼容
func (s *ModelService) ResolveCredentials(ctx context.Context, name string) (*credential.ModelCredentials, errors.Error) {
	if name == "" {
		return s.ResolveDefaultCredentials(ctx)
	}

	models, err := s.model.FindModelsByName(ctx, name)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.model.resolve_find"))
	}
	if len(models) == 0 {
		// 向后兼容:模型表无此记录时透传模型名,Python 回退环境变量凭证
		zap.L().Warn("[炼丹炉] 模型未在模型管理中登记，回退环境变量凭证", zap.String("model", name))
		return &credential.ModelCredentials{Model: name}, nil
	}

	var selected *model.LLMModel
	enabledCount := 0
	for i := range models {
		if models[i].IsEnabled {
			enabledCount++
			if selected == nil {
				selected = models[i]
			}
		}
	}
	if selected == nil {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.model.disabled", "该道人使用的模型已停用，请更换模型")
	}
	if enabledCount > 1 {
		zap.L().Warn("[炼丹炉] 同名模型存在于多个供应商，按 sort_order,id 取第一个",
			zap.String("model", name),
			zap.Uint("selected_id", selected.ID),
			zap.Uint("provider_id", selected.ProviderID))
	}

	return s.resolveCredentials(ctx, selected)
}

// ResolveDefaultCredentials 解析默认模型凭证;无默认模型时回退环境变量配置
func (s *ModelService) ResolveDefaultCredentials(ctx context.Context) (*credential.ModelCredentials, errors.Error) {
	m, err := s.model.TakeDefaultEnabled(ctx)
	if err != nil {
		if err.IsType(errors.ErrorTypeRecordNotFound) {
			zap.L().Warn("[炼丹炉] 未配置默认模型，回退环境变量凭证", zap.String("model", configuration.Configuration.LLM.DefaultModel))
			return &credential.ModelCredentials{Model: configuration.Configuration.LLM.DefaultModel}, nil
		}
		return nil, err.Relation(errors.ErrorServerInternalError("service.model.resolve_default"))
	}
	return s.resolveCredentials(ctx, m)
}

// ResolveSynthesisCredentials 解析合成专用模型凭证;无则回退默认模型
func (s *ModelService) ResolveSynthesisCredentials(ctx context.Context) (*credential.ModelCredentials, errors.Error) {
	m, err := s.model.TakeSynthesisEnabled(ctx)
	if err != nil {
		if err.IsType(errors.ErrorTypeRecordNotFound) {
			return s.ResolveDefaultCredentials(ctx)
		}
		return nil, err.Relation(errors.ErrorServerInternalError("service.model.resolve_synthesis"))
	}
	return s.resolveCredentials(ctx, m)
}

// ResolveFusionCredentials 解析金丹融合专用模型凭证;未配置则返回 ErrorInvalidRequest(不兜底道人默认模型)
func (s *ModelService) ResolveFusionCredentials(ctx context.Context) (*credential.ModelCredentials, errors.Error) {
	m, err := s.model.TakeFusionEnabled(ctx)
	if err != nil {
		if err.IsType(errors.ErrorTypeRecordNotFound) {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.model.resolve_fusion",
				"未配置金丹融合专用模型，请前往「设置 → 模型管理」勾选「用于金丹融合」")
		}
		return nil, err.Relation(errors.ErrorServerInternalError("service.model.resolve_fusion"))
	}
	return s.resolveCredentials(ctx, m)
}

// GetFusionModelConfig 返回当前已配置的融合专用模型详情(供 /fusion 页面 banner 展示)
// 未配置时返回 Configured=false 的空结构,供前端走「未配置」提示分支
func (s *ModelService) GetFusionModelConfig(ctx context.Context) (*iservice.FusionModelConfig, errors.Error) {
	m, err := s.model.TakeFusionEnabled(ctx)
	if err != nil {
		if err.IsType(errors.ErrorTypeRecordNotFound) {
			// 未配置不属于错误,返回 Configured=false 的空结构让前端走警告分支
			return &iservice.FusionModelConfig{Configured: false}, nil
		}
		return nil, err.Relation(errors.ErrorServerInternalError("service.model.get_fusion_config"))
	}
	// 预加载 provider 已在 TakeFusionEnabled 内完成
	return &iservice.FusionModelConfig{
		Configured:          true,
		ModelName:           m.Name,
		ModelDisplayName:    m.DisplayName,
		ProviderName:        m.Provider.Name,
		ProviderDisplayName: m.Provider.DisplayName,
	}, nil
}
