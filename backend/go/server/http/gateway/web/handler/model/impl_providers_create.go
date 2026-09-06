package model

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// CreateProviderRequest 创建供应商请求
type CreateProviderRequest struct {
	Name        string `json:"name" binding:"required,max=50"`          // 供应商标识(唯一)
	DisplayName string `json:"display_name" binding:"required,max=100"` // 显示名
	Protocol    string `json:"protocol" binding:"omitempty,max=50"`     // 协议类型,缺省 openai-compatible
	BaseURL     string `json:"base_url" binding:"required,max=255"`     // OpenAI 兼容接口地址
	APIKey      string `json:"api_key"`                                 // 明文 api_key(仅写入时传输)
	IsEnabled   *bool  `json:"is_enabled"`                              // 是否启用,缺省 true
	SortOrder   int    `json:"sort_order"`                              // 展示顺序
	Remark      string `json:"remark" binding:"max=255"`                // 备注
}

// CreateProvider 创建供应商
// POST /api/v1/providers
func (cls *Model) CreateProvider(c *gin.Context) (response.Code, any, error) {
	var body CreateProviderRequest
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}

	isEnabled := true
	if body.IsEnabled != nil {
		isEnabled = *body.IsEnabled
	}

	v, err := cls.provider.CreateProvider(contextutil.NewContextWithGin(c),
		body.Name, body.DisplayName, body.Protocol, body.BaseURL, body.APIKey,
		isEnabled, body.SortOrder, body.Remark)
	if err != nil {
		return 0, nil, err
	}
	return response.CodeCreated, toProviderResponse(v), nil
}
