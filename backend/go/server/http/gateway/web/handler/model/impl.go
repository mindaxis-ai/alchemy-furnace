// Package model 供应商与模型管理 HTTP 处理器(新网关;对齐 Luna-CY 模板 handler 分包风格)
// 路由: /api/v1/providers, /api/v1/models;路径参数 :uuid 为对外唯一标识
// 包名 model 与 GORM 模型包同名,故模型包别名 gmodel
package model

import (
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	gmodel "github.com/alchemy-furnace/server/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Model 供应商与模型管理处理器
type Model struct {
	provider service.Provider
	model    service.Model
}

// New 构造处理器
func New(provider service.Provider, model service.Model) *Model {
	return &Model{provider: provider, model: model}
}

// ---------- 响应 DTO ----------

// ProviderResponse 供应商响应 DTO:id 输出 UUID 字符串,api_key 永不明文返回(仅掩码)
type ProviderResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	DisplayName  string    `json:"display_name"`
	Protocol     string    `json:"protocol"`
	BaseURL      string    `json:"base_url"`
	APIKeyMasked string    `json:"api_key_masked"`
	HasAPIKey    bool      `json:"has_api_key"`
	IsEnabled    bool      `json:"is_enabled"`
	SortOrder    int       `json:"sort_order"`
	Remark       string    `json:"remark"`
	ModelCount   int64     `json:"model_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ModelResponse 模型响应 DTO:id/provider_id 输出 UUID 字符串
type ModelResponse struct {
	ID                  string    `json:"id"`
	ProviderID          string    `json:"provider_id"`
	Name                string    `json:"name"`
	DisplayName         string    `json:"display_name"`
	ProviderName        string    `json:"provider_name"`
	ProviderDisplayName string    `json:"provider_display_name"`
	Temperature         float64   `json:"temperature"`
	MaxTokens           int       `json:"max_tokens"`
	IsEnabled           bool      `json:"is_enabled"`
	IsDefault           bool      `json:"is_default"`
	IsSynthesis         bool      `json:"is_synthesis"`
	IsFusion            bool      `json:"is_fusion"`
	SortOrder           int       `json:"sort_order"`
	ReferencedBy        int64     `json:"referenced_by"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// toProviderResponse 供应商富视图 -> 对外 DTO
func toProviderResponse(v *gmodel.ProviderView) *ProviderResponse {
	return &ProviderResponse{
		ID:           v.UUID.String(),
		Name:         v.Name,
		DisplayName:  v.DisplayName,
		Protocol:     v.Protocol,
		BaseURL:      v.BaseURL,
		APIKeyMasked: v.APIKeyMasked,
		HasAPIKey:    v.HasAPIKey,
		IsEnabled:    v.IsEnabled,
		SortOrder:    v.SortOrder,
		Remark:       v.Remark,
		ModelCount:   v.ModelCount,
		CreatedAt:    v.CreatedAt,
		UpdatedAt:    v.UpdatedAt,
	}
}

// toProviderResponseList 批量转换
func toProviderResponseList(views []*gmodel.ProviderView) []*ProviderResponse {
	list := make([]*ProviderResponse, 0, len(views))
	for _, v := range views {
		list = append(list, toProviderResponse(v))
	}
	return list
}

// toModelResponse 模型富视图 -> 对外 DTO
func toModelResponse(v *gmodel.ModelView) *ModelResponse {
	return &ModelResponse{
		ID:                  v.UUID.String(),
		ProviderID:          v.Provider.UUID.String(),
		Name:                v.Name,
		DisplayName:         v.DisplayName,
		ProviderName:        v.Provider.Name,
		ProviderDisplayName: v.Provider.DisplayName,
		Temperature:         v.Temperature,
		MaxTokens:           v.MaxTokens,
		IsEnabled:           v.IsEnabled,
		IsDefault:           v.IsDefault,
		IsSynthesis:         v.IsSynthesis,
		IsFusion:            v.IsFusion,
		SortOrder:           v.SortOrder,
		ReferencedBy:        v.ReferencedBy,
		CreatedAt:           v.CreatedAt,
		UpdatedAt:           v.UpdatedAt,
	}
}

// toModelResponseList 批量转换
func toModelResponseList(views []*gmodel.ModelView) []*ModelResponse {
	list := make([]*ModelResponse, 0, len(views))
	for _, v := range views {
		list = append(list, toModelResponse(v))
	}
	return list
}

// ---------- 路径参数解析 ----------

// parseUUID 解析 :uuid 路径参数;非法形态返回 400
func parseUUID(c *gin.Context) (uuid.UUID, errors.Error) {
	uid, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		return uuid.Nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.model.uuid_parse", "ID格式不正确")
	}
	return uid, nil
}
