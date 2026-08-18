// Package provider_service 供应商管理业务逻辑实现(新架构 internal 分层)
// 管理供应商配置:CRUD、凭证加解密、连接测试、预置模板;UUID 为唯一对外标识
// api_key 以 AES-GCM 加密存储,接口输出仅掩码形式,明文永不落日志
package provider_service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alchemy-furnace/server/internal/engineendpoint"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/engine"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ProviderService service.Provider 接口实现
type ProviderService struct {
	provider   dao.Provider
	model      dao.Model
	httpClient *http.Client
}

// New 构造供应商业务实例
// provider: 供应商 DAO;model: 模型 DAO(连接测试回退取首个启用模型)
func New(provider dao.Provider, model dao.Model) *ProviderService {
	return &ProviderService{
		provider:   provider,
		model:      model,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// ---------- 视图转换 ----------

// toView 转换为供应商富视图(解密仅用于生成掩码,明文不出现在任何输出中)
func (s *ProviderService) toView(p *model.LLMProvider, modelCount int64) *model.ProviderView {
	masked := ""
	if p.APIKeyEncrypted != "" {
		plain, err := credential.DecryptAPIKey(p.APIKeyEncrypted)
		if err != nil {
			zap.L().Warn("[炼丹炉] 供应商密钥解密失败，掩码降级显示", zap.Uint("provider_id", p.ID), zap.Error(err))
			masked = "****"
		} else {
			masked = credential.MaskAPIKey(plain)
		}
	}
	return &model.ProviderView{
		LLMProvider:  p,
		APIKeyMasked: masked,
		HasAPIKey:    p.APIKeyEncrypted != "",
		ModelCount:   modelCount,
	}
}

// ---------- 查询 ----------

// ListProviders 分页查询供应商列表(附带模型数量与掩码 api_key)
func (s *ProviderService) ListProviders(ctx context.Context, page, size int, enabled *bool) (int64, []*model.ProviderView, errors.Error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}
	if size > 200 {
		size = 200
	}

	total, providers, err := s.provider.FindProviders(ctx, page, size, enabled)
	if err != nil {
		return 0, nil, err.Relation(errors.ErrorServerInternalError("service.provider.list"))
	}

	views := make([]*model.ProviderView, 0, len(providers))
	for _, p := range providers {
		count, cerr := s.provider.CountModelsByProvider(ctx, p.ID)
		if cerr != nil {
			return 0, nil, cerr.Relation(errors.ErrorServerInternalError("service.provider.list_count"))
		}
		views = append(views, s.toView(p, count))
	}
	return total, views, nil
}

// GetProviderByUUID 按 UUID 获取供应商详情(掩码形式)
func (s *ProviderService) GetProviderByUUID(ctx context.Context, uid uuid.UUID) (*model.ProviderView, errors.Error) {
	p, err := s.provider.TakeProviderByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.provider.get"))
	}
	count, cerr := s.provider.CountModelsByProvider(ctx, p.ID)
	if cerr != nil {
		return nil, cerr.Relation(errors.ErrorServerInternalError("service.provider.get_count"))
	}
	return s.toView(p, count), nil
}

// ---------- 校验 ----------

func validateProviderName(name string) errors.Error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.provider.name.empty", "供应商标识不能为空")
	}
	if len(name) > 50 {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.provider.name.too_long", "供应商标识长度不能超过 50 个字符")
	}
	return nil
}

func validateProtocol(protocol string) errors.Error {
	if protocol != "openai-compatible" {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.provider.protocol.unsupported", "暂不支持的协议类型「%s」，当前仅支持 openai-compatible", protocol)
	}
	return nil
}

func validateBaseURL(raw string) errors.Error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.provider.base_url.invalid", "base_url 不是合法的 URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New(errors.ErrorTypeInvalidRequest, "service.provider.base_url.scheme", "base_url 仅支持 http/https 协议")
	}
	return nil
}

// ---------- 写入 ----------

// CreateProvider 创建供应商配置
func (s *ProviderService) CreateProvider(ctx context.Context, name, displayName, protocol, baseURL, apiKey string, isEnabled bool, sortOrder int, remark string) (*model.ProviderView, errors.Error) {
	name = strings.TrimSpace(name)
	if err := validateProviderName(name); err != nil {
		return nil, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.provider.display_name.empty", "显示名不能为空")
	}
	if err := validateBaseURL(baseURL); err != nil {
		return nil, err
	}
	protocol = strings.TrimSpace(protocol)
	if protocol == "" {
		protocol = "openai-compatible"
	}
	if err := validateProtocol(protocol); err != nil {
		return nil, err
	}

	exists, err := s.provider.CountProvidersByName(ctx, name, 0)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.provider.create_name_check"))
	}
	if exists > 0 {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.provider.name.exists", "供应商标识「%s」已存在", name)
	}

	encrypted, eerr := credential.EncryptAPIKey(apiKey)
	if eerr != nil {
		if eerr == credential.ErrNoSecret {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.provider.encrypt.no_secret", "未配置 MODEL_KEY_SECRET 环境变量，无法存储 API Key")
		}
		return nil, errors.ErrorServerInternalError("service.provider.encrypt")
	}

	p := &model.LLMProvider{
		Name:            name,
		DisplayName:     displayName,
		Protocol:        protocol,
		BaseURL:         baseURL,
		APIKeyEncrypted: encrypted,
		IsEnabled:       isEnabled,
		SortOrder:       sortOrder,
		Remark:          remark,
	}
	if err := s.provider.SaveProvider(ctx, p); err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.provider.create"))
	}

	zap.L().Info("[炼丹炉] 新供应商入炉",
		zap.String("uuid", p.UUID.String()),
		zap.String("name", p.Name),
		zap.String("protocol", p.Protocol))
	return s.toView(p, 0), nil
}

// UpdateProvider 按 UUID 部分更新供应商
// apiKey: nil=不修改,空字符串=清除密钥,非空=重新加密存储;更新后其下所有模型立即生效
func (s *ProviderService) UpdateProvider(ctx context.Context, uid uuid.UUID, name, displayName, protocol, baseURL, apiKey *string, isEnabled *bool, sortOrder *int, remark *string) (*model.ProviderView, errors.Error) {
	p, err := s.provider.TakeProviderByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.provider.update_take"))
	}

	updates := map[string]any{}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if err := validateProviderName(trimmed); err != nil {
			return nil, err
		}
		exists, cerr := s.provider.CountProvidersByName(ctx, trimmed, p.ID)
		if cerr != nil {
			return nil, cerr.Relation(errors.ErrorServerInternalError("service.provider.update_name_check"))
		}
		if exists > 0 {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.provider.name.exists", "供应商标识「%s」已存在", trimmed)
		}
		updates["name"] = trimmed
	}
	if displayName != nil {
		trimmed := strings.TrimSpace(*displayName)
		if trimmed == "" {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.provider.display_name.empty", "显示名不能为空")
		}
		updates["display_name"] = trimmed
	}
	if protocol != nil {
		if err := validateProtocol(*protocol); err != nil {
			return nil, err
		}
		updates["protocol"] = *protocol
	}
	if baseURL != nil {
		if err := validateBaseURL(*baseURL); err != nil {
			return nil, err
		}
		updates["base_url"] = *baseURL
	}
	if apiKey != nil {
		encrypted, eerr := credential.EncryptAPIKey(*apiKey)
		if eerr != nil {
			if eerr == credential.ErrNoSecret {
				return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.provider.encrypt.no_secret", "未配置 MODEL_KEY_SECRET 环境变量，无法存储 API Key")
			}
			return nil, errors.ErrorServerInternalError("service.provider.encrypt")
		}
		updates["api_key_encrypted"] = encrypted // 空字符串即清除密钥
	}
	if isEnabled != nil {
		updates["is_enabled"] = *isEnabled
	}
	if sortOrder != nil {
		updates["sort_order"] = *sortOrder
	}
	if remark != nil {
		updates["remark"] = *remark
	}

	if len(updates) > 0 {
		if err := s.provider.UpdateProvider(ctx, p, updates); err != nil {
			return nil, err.Relation(errors.ErrorServerInternalError("service.provider.update"))
		}
	}

	fresh, err := s.provider.TakeProviderByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorServerInternalError("service.provider.update_retake"))
	}
	count, cerr := s.provider.CountModelsByProvider(ctx, fresh.ID)
	if cerr != nil {
		return nil, cerr.Relation(errors.ErrorServerInternalError("service.provider.update_count"))
	}

	zap.L().Info("[炼丹炉] 供应商配置已更新", zap.String("uuid", uid.String()), zap.String("name", fresh.Name))
	return s.toView(fresh, count), nil
}

// DeleteProvider 按 UUID 删除供应商;下有模型时返回 409(携带 model_count)
func (s *ProviderService) DeleteProvider(ctx context.Context, uid uuid.UUID) errors.Error {
	p, err := s.provider.TakeProviderByUUID(ctx, uid)
	if err != nil {
		return err.Relation(errors.ErrorRecordNotFound("service.provider.delete_take"))
	}

	count, cerr := s.provider.CountModelsByProvider(ctx, p.ID)
	if cerr != nil {
		return cerr.Relation(errors.ErrorServerInternalError("service.provider.delete_count"))
	}
	if count > 0 {
		return errors.ErrorConflictWithData(
			"service.provider.delete_has_models",
			map[string]any{"model_count": count},
			"该供应商下仍有 %d 个模型，无法删除",
			count,
		)
	}

	if err := s.provider.DeleteProvider(ctx, p); err != nil {
		return err.Relation(errors.ErrorServerInternalError("service.provider.delete"))
	}

	zap.L().Info("[炼丹炉] 供应商配置已移除", zap.String("uuid", uid.String()), zap.String("name", p.Name))
	return nil
}

// TestConnection 以供应商凭证发起一次最小 LLM 调用(max_tokens=1),测量延迟
// modelName 空时回退该供应商下第一个启用模型;失败结果包裹在 TestConnectionResult 中返回(非 errors.Error)
func (s *ProviderService) TestConnection(ctx context.Context, uid uuid.UUID, modelName string) (*model.TestConnectionResult, errors.Error) {
	p, err := s.provider.TakeProviderByUUID(ctx, uid)
	if err != nil {
		return nil, err.Relation(errors.ErrorRecordNotFound("service.provider.test_take"))
	}

	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		m, merr := s.model.FindFirstEnabledModelByProvider(ctx, p.ID)
		if merr != nil {
			return nil, errors.New(errors.ErrorTypeInvalidRequest, "service.provider.test.no_model", "请先为该供应商添加已启用模型")
		}
		modelName = m.Name
	}

	apiKey, derr := credential.DecryptAPIKey(p.APIKeyEncrypted)
	if derr != nil {
		return nil, errors.ErrorServerInternalError("service.provider.test_decrypt")
	}

	reqBody := map[string]interface{}{
		"model":      modelName,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	}
	if p.BaseURL != "" {
		reqBody["base_url"] = p.BaseURL
	}
	if apiKey != "" {
		reqBody["api_key"] = apiKey
	}
	jsonBody, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("%s/api/v1/chat/completions", engineendpoint.Current())
	req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if rerr != nil {
		return nil, errors.ErrorServerInternalError("service.provider.test_build_req")
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, doErr := s.httpClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if doErr != nil {
		return &model.TestConnectionResult{Success: false, LatencyMs: latency, Error: engine.MapEngineError(doErr)}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &model.TestConnectionResult{
			Success:   false,
			LatencyMs: latency,
			Error:     engine.MapEngineError(&engine.EngineError{Op: "供应商连接测试", StatusCode: resp.StatusCode, Body: string(body)}),
		}, nil
	}

	return &model.TestConnectionResult{Success: true, LatencyMs: latency, Error: ""}, nil
}

// Templates 返回预置供应商模板清单(静态常量,前后端单一数据源)
func (s *ProviderService) Templates() []model.ProviderTemplate {
	return model.ProviderTemplates
}
