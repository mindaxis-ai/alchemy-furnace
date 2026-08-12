// Package fusion 金丹融合 HTTP 处理器(新网关;对齐 Luna-CY 模板 handler 分包风格)
// 路由: /api/v1/fusion/fuse
package fusion

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Fusion 金丹融合处理器
type Fusion struct {
	fusion service.Fusion
}

// New 构造金丹融合处理器
func New(fusion service.Fusion) *Fusion {
	return &Fusion{fusion: fusion}
}

// FuseRequest 金丹融合请求
type FuseRequest struct {
	PillUUIDs         []string `json:"pill_uuids" binding:"required,min=2"` // 原料金丹 UUID(至少 2 枚)
	ExcludeOperatorID string   `json:"exclude_operator_id"`                 // 重试时要排除的算子 id(可选)
}

// FuseResponse 金丹融合响应
type FuseResponse struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	SkillSchema model.JSONMap          `json:"skill_schema"`
	Operator    synthesis.FuseOperator `json:"operator"`
	Model       string                 `json:"model"`
	Degraded    bool                   `json:"degraded"`
}

// Fuse 金丹融合预览(不落库)
// POST /api/v1/fusion/fuse
func (cls *Fusion) Fuse(c *gin.Context) (response.Code, any, error) {
	var body FuseRequest
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}

	// UUID 边界在此解析,非法返回 400(与 trial.parsePillInputs 对齐)
	uids := make([]uuid.UUID, 0, len(body.PillUUIDs))
	for i, s := range body.PillUUIDs {
		uid, err := uuid.Parse(s)
		if err != nil {
			return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.fusion.uuid_parse", "第%d颗金丹ID格式不正确", i+1)
		}
		uids = append(uids, uid)
	}

	result, err := cls.fusion.Fuse(contextutil.NewContextWithGin(c), uids, body.ExcludeOperatorID)
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, FuseResponse{
		Name:        result.Name,
		Description: result.Description,
		SkillSchema: result.SkillSchema,
		Operator:    result.Operator,
		Model:       result.Model,
		Degraded:    result.Degraded,
	}, nil
}
