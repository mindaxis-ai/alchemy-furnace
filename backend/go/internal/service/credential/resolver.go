// 模型调用凭证解析:供应商/模型凭证的两级解析链(model -> provider -> 解密 api_key)
// 对话服务、试炼服务共用;解析失败由调用方决定降级策略(试丹回退 nil,对话返回错误)
package credential

import (
	"context"
	"errors"
	"fmt"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Resolver 模型调用凭证解析接口
type Resolver interface {
	// ResolveCredentials 解析指定模型名的调用凭证(model -> provider -> 解密)
	// name 为空时解析默认模型;未登记模型回退仅含模型名的空凭证(Python 回退环境变量)
	ResolveCredentials(ctx context.Context, name string) (*ModelCredentials, error)

	// ResolveSynthesisCredentials 解析语言模式合成专用模型凭证(is_synthesis 优先,回退 is_default)
	ResolveSynthesisCredentials(ctx context.Context) (*ModelCredentials, error)

	// ResolveFusionCredentials 解析金丹融合专用模型凭证(is_fusion 优先;找不到返回明确错误,不静默回退 is_default)
	ResolveFusionCredentials(ctx context.Context) (*ModelCredentials, error)
}

// ModelResolver credential.Resolver 实现:直接经 internal/dao 查询模型/供应商并解密 api_key
type ModelResolver struct{}

// NewResolver 构造凭证解析器
func NewResolver() *ModelResolver {
	return &ModelResolver{}
}

// ResolveCredentials 解析指定模型名的调用凭证
//   - 同名模型存在于多个供应商时,取 sort_order,id 最小者并记录 warning
//   - 找到已启用模型:返回供应商凭证(base_url + 解密 api_key)
//   - 找到但全部已停用:返回明确错误「该模型已停用，请更换模型」
//   - 未找到:返回仅含模型名的空凭证(Python 回退环境变量配置,向后兼容)并记录警告
//   - name 为空:解析默认模型
func (r *ModelResolver) ResolveCredentials(ctx context.Context, name string) (*ModelCredentials, error) {
	// 007-demo-mode: 演示模式无 DB,Python 端走 DemoChatProvider,凭证无意义
	if configuration.IsDemo() {
		return &ModelCredentials{Model: name}, nil
	}

	if name == "" {
		return r.ResolveDefaultCredentials(ctx)
	}

	var models []model.LLMModel
	if err := dao.GetDB().WithContext(ctx).Where("name = ?", name).
		Order("sort_order ASC, id ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("查询模型配置失败: %w", err)
	}
	if len(models) == 0 {
		// 向后兼容:模型表无此记录时透传模型名,Python 回退环境变量凭证
		zap.L().Warn("[炼丹炉] 模型未在模型管理中登记，回退环境变量凭证", zap.String("model", name))
		return &ModelCredentials{Model: name}, nil
	}

	// 同名模型可能跨供应商存在多个:取第一个已启用者
	var selected *model.LLMModel
	enabledCount := 0
	for i := range models {
		if models[i].IsEnabled {
			enabledCount++
			if selected == nil {
				selected = &models[i]
			}
		}
	}
	if selected == nil {
		return nil, errors.New("该模型已停用，请更换模型")
	}
	if enabledCount > 1 {
		zap.L().Warn("[炼丹炉] 同名模型存在于多个供应商，按 sort_order,id 取第一个",
			zap.String("model", name),
			zap.Uint("selected_id", selected.ID),
			zap.Uint("provider_id", selected.ProviderID))
	}

	return r.resolveModelCredentials(ctx, selected)
}

// ResolveDefaultCredentials 解析默认模型凭证;无默认模型时回退环境变量配置
func (r *ModelResolver) ResolveDefaultCredentials(ctx context.Context) (*ModelCredentials, error) {
	cfg := configuration.Configuration

	var m model.LLMModel
	err := dao.GetDB().WithContext(ctx).Where("is_default = ? AND is_enabled = ?", true, true).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		zap.L().Warn("[炼丹炉] 未配置默认模型，回退环境变量凭证", zap.String("model", cfg.LLM.DefaultModel))
		return &ModelCredentials{Model: cfg.LLM.DefaultModel}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询默认模型配置失败: %w", err)
	}

	return r.resolveModelCredentials(ctx, &m)
}

// ResolveSynthesisCredentials 解析语言模式合成专用模型凭证:is_synthesis 优先,回退 is_default
func (r *ModelResolver) ResolveSynthesisCredentials(ctx context.Context) (*ModelCredentials, error) {
	// 007-demo-mode: 演示模式无 DB,返回空凭证(Python 端走 DemoSynthesisProvider)
	if configuration.IsDemo() {
		return &ModelCredentials{}, nil
	}

	var m model.LLMModel
	err := dao.GetDB().WithContext(ctx).Where("is_synthesis = ? AND is_enabled = ?", true, true).First(&m).Error
	switch {
	case err == nil:
		return r.resolveModelCredentials(ctx, &m)
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 无合成专用模型,回退默认模型
		return r.ResolveDefaultCredentials(ctx)
	default:
		return nil, fmt.Errorf("查询合成模型配置失败: %w", err)
	}
}

// ResolveFusionCredentials 解析金丹融合专用模型凭证:is_fusion 优先,找不到则返回明确错误(不兜底道人默认模型)
func (r *ModelResolver) ResolveFusionCredentials(ctx context.Context) (*ModelCredentials, error) {
	// 007-demo-mode: 演示模式无 DB,返回空凭证(Python 端走 DemoFusionProvider)
	if configuration.IsDemo() {
		return &ModelCredentials{}, nil
	}

	var m model.LLMModel
	err := dao.GetDB().WithContext(ctx).Where("is_fusion = ? AND is_enabled = ?", true, true).First(&m).Error
	switch {
	case err == nil:
		return r.resolveModelCredentials(ctx, &m)
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 硬性报错:不静默回退道人默认模型,引导用户去设置中配置融合专用
		return nil, errors.New("未配置金丹融合专用模型，请前往「设置 → 模型管理」勾选「用于金丹融合」")
	default:
		return nil, fmt.Errorf("查询融合模型配置失败: %w", err)
	}
}

// resolveModelCredentials 由已启用模型解析完整调用凭证:加载供应商 -> 校验启用 -> 解密 api_key
func (r *ModelResolver) resolveModelCredentials(ctx context.Context, m *model.LLMModel) (*ModelCredentials, error) {
	var p model.LLMProvider
	if err := dao.GetDB().WithContext(ctx).First(&p, m.ProviderID).Error; err != nil {
		return nil, fmt.Errorf("查询模型所属供应商配置失败: %w", err)
	}
	if !p.IsEnabled {
		return nil, errors.New("该模型所属供应商已停用")
	}

	apiKey, err := DecryptAPIKey(p.APIKeyEncrypted)
	if err != nil {
		return nil, err
	}
	return &ModelCredentials{Model: m.Name, BaseURL: p.BaseURL, APIKey: apiKey}, nil
}
