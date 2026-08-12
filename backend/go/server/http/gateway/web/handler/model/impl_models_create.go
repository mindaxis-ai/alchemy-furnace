package model

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// CreateModelRequest 创建模型请求(供应商由嵌套路径 :uuid 提供)
type CreateModelRequest struct {
	Name        string  `json:"name" binding:"required,max=100"`         // 模型名(同供应商下唯一)
	DisplayName string  `json:"display_name" binding:"required,max=100"` // 显示名
	Temperature float64 `json:"temperature"`                             // 默认温度(0-2),0 值视为默认 0.7
	MaxTokens   int     `json:"max_tokens"`                              // 默认最大 token,0 值视为默认 4096
	IsEnabled   *bool   `json:"is_enabled"`                              // 是否启用,缺省 true
	IsDefault   bool    `json:"is_default"`                              // 是否默认模型(全表最多一个)
	IsSynthesis bool    `json:"is_synthesis"`                            // 是否合成专用模型(全表最多一个)
	IsFusion    bool    `json:"is_fusion"`                               // 是否金丹融合专用模型(全表最多一个)
	SortOrder   int     `json:"sort_order"`                              // 展示顺序
}

// CreateModel 在供应商下创建模型
// POST /api/v1/providers/:uuid/models
func (cls *Model) CreateModel(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}

	var body CreateModelRequest
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}

	isEnabled := true
	if body.IsEnabled != nil {
		isEnabled = *body.IsEnabled
	}

	v, serr := cls.model.CreateModel(contextutil.NewContextWithGin(c), uid,
		body.Name, body.DisplayName, body.Temperature, body.MaxTokens,
		isEnabled, body.IsDefault, body.IsSynthesis, body.IsFusion, body.SortOrder)
	if serr != nil {
		return 0, nil, serr
	}
	return response.CodeCreated, toModelResponse(v), nil
}
