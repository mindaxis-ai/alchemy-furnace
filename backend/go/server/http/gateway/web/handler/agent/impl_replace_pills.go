package agent

import (
	"github.com/alchemy-furnace/server/internal/context/contextutil"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ReplacePillsItem 完整服丹编排水(金丹对外 UUID + 权重)
type ReplacePillsItem struct {
	PillID string  `json:"pill_id" binding:"required"` // 金丹 UUID
	Weight float64 `json:"weight" binding:"required"`  // 剂量/权重
}

// ReplacePillsRequest 完整服丹编排请求;pills 为空数组表示清空全部服用关系
type ReplacePillsRequest struct {
	Pills []ReplacePillsItem `json:"pills"`
}

// ReplacePills 用完整服丹编排一次性替换道人服用关系(原子)
// PUT /api/v1/agents/:uuid/pills
// handler 只做绑定与 UUID 转换;顺序/权重区间/重复/存在性校验在 service
func (cls *Agent) ReplacePills(c *gin.Context) (response.Code, any, error) {
	agentUID, err := parseUUID(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}

	var body ReplacePillsRequest
	if berr := request.ShouldBindJSON(c, &body); berr != nil {
		return response.InvalidParams, nil, berr
	}

	items := make([]service.PillCompositionItem, 0, len(body.Pills))
	for _, p := range body.Pills {
		pillUID, perr := uuid.Parse(p.PillID)
		if perr != nil {
			return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest, "handler.agent.replace_pills_pill_uuid", "金丹ID格式不正确")
		}
		items = append(items, service.PillCompositionItem{PillUUID: pillUID, Weight: p.Weight})
	}

	detail, serr := cls.agent.ReplacePillComposition(contextutil.NewContextWithGin(c), agentUID, items)
	if serr != nil {
		return response.CodeReplacePillsFailed, nil, serr
	}
	return response.Ok, toDetailResponse(detail), nil
}
