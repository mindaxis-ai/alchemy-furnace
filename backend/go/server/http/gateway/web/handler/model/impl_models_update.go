package model

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
)

// UpdateModelRequest 更新模型请求(指针字段区分「未传」与「置空/置零」)
// is_default/is_synthesis/is_fusion 置 true 时事务内清除其他记录
type UpdateModelRequest struct {
	Name        *string  `json:"name" binding:"omitempty,max=100"`
	DisplayName *string  `json:"display_name" binding:"omitempty,max=100"`
	Temperature *float64 `json:"temperature"`
	MaxTokens   *int     `json:"max_tokens"`
	IsEnabled   *bool    `json:"is_enabled"`
	IsDefault   *bool    `json:"is_default"`
	IsSynthesis *bool    `json:"is_synthesis"`
	IsFusion    *bool    `json:"is_fusion"`
	SortOrder   *int     `json:"sort_order"`
}

// UpdateModel 更新模型
// PUT /api/v1/models/:uuid
func (cls *Model) UpdateModel(c *gin.Context) (response.Code, any, error) {
	uid, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}

	var body UpdateModelRequest
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}

	v, serr := cls.model.UpdateModel(contextutil.NewContextWithGin(c), uid,
		body.Name, body.DisplayName, body.Temperature, body.MaxTokens,
		body.IsEnabled, body.IsDefault, body.IsSynthesis, body.IsFusion, body.SortOrder)
	if serr != nil {
		return 0, nil, serr
	}
	return response.Ok, toModelResponse(v), nil
}
