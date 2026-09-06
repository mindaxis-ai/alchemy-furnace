// 融合两阶段路由：预览（模型调用在事务外）→ 确认（原子消耗全部材料并产出）
// 预览不要求幂等键（非写操作）；确认必须携带 Idempotency-Key（全局幂等）。
package pill_inventory

import (
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// previewBody 融合预览请求体（两阶段第一阶段）
type previewBody struct {
	ItemIDs           []string `json:"item_ids" binding:"required,min=2"`
	ExcludeOperatorID string   `json:"exclude_operator_id"`
}

// PreviewFusion 融合预览：校验材料 → 模型生成（事务外）→ 持久化预览（15 分钟 TTL）
// POST /api/v1/fusion/previews
func (h *Handler) PreviewFusion(c *gin.Context) (response.Code, any, error) {
	var body previewBody
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}
	itemIDs := make([]uuid.UUID, 0, len(body.ItemIDs))
	for i, s := range body.ItemIDs {
		uid, err := uuid.Parse(s)
		if err != nil {
			return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest,
				"handler.pill_inventory.uuid_parse", "item_ids[%d] 不是合法 UUID", i)
		}
		itemIDs = append(itemIDs, uid)
	}
	result, err := h.fusion.PreviewFusion(ctx(c), service.PreviewFusionRequest{
		ItemIDs:           itemIDs,
		ExcludeOperatorID: body.ExcludeOperatorID,
	})
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, map[string]any{
		"preview_id":   result.PreviewID,
		"expires_at":   result.ExpiresAt,
		"name":         result.Name,
		"description":  result.Description,
		"skill_schema": result.SkillSchema,
		"operator":     result.Operator,
		"model":        result.Model,
		"degraded":     result.Degraded,
	}, nil
}

// confirmBody 融合确认请求体（两阶段第二阶段）
type confirmBody struct {
	PreviewID   string `json:"preview_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// ConfirmFusion 原子确认融合：扣全部材料（available→consumed_by_fusion）并产出新金丹（幂等）
// POST /api/v1/fusion/confirm
func (h *Handler) ConfirmFusion(c *gin.Context) (response.Code, any, error) {
	opID, err := idempotencyKey(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	var body confirmBody
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}
	previewID, err := uuid.Parse(body.PreviewID)
	if err != nil {
		return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest,
			"handler.pill_inventory.uuid_parse", "preview_id 不是合法 UUID")
	}
	result, err := h.inventory.ConfirmFusion(ctx(c), service.ConfirmFusionRequest{
		OperationID: opID,
		PreviewID:   previewID,
		Name:        body.Name,
		Description: body.Description,
	})
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, operationResultOut(result), nil
}
