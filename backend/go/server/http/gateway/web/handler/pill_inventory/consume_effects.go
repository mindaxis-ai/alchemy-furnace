// 道人服用与能力编排路由：服用 / 能力列表 / 全量编排 / 移除能力
// 能力对外标识为 AgentPillEffect.UUID（effect_id），服用响应回传该标识；
// 全量编排 PUT 的提交集必须等于活跃集，乐观锁由 expected_effects_revision 承担。
package pill_inventory

import (
	"fmt"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// consumeBody 服用金丹请求体
type consumeBody struct {
	ItemID    string  `json:"item_id" binding:"required"`
	Weight    float64 `json:"weight"`
	SortOrder *int    `json:"sort_order"`
}

// ConsumePill 服用金丹：available→consumed_by_agent + 能力快照（幂等）
// POST /api/v1/agents/:id/consume
func (h *Handler) ConsumePill(c *gin.Context) (response.Code, any, error) {
	opID, err := idempotencyKey(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	agentID, err := pathUUID(c, "uuid")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	var body consumeBody
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}
	itemID, err := uuid.Parse(body.ItemID)
	if err != nil {
		return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest,
			"handler.pill_inventory.uuid_parse", "item_id 不是合法 UUID")
	}
	if err := validateWeight("weight", body.Weight); err != nil {
		return response.InvalidParams, nil, err
	}
	sortOrder := 0
	if body.SortOrder != nil {
		if err := validateSortOrder("sort_order", *body.SortOrder); err != nil {
			return response.InvalidParams, nil, err
		}
		sortOrder = *body.SortOrder
	}
	result, err := h.inventory.Consume(ctx(c), service.ConsumePillRequest{
		OperationID: opID,
		AgentID:     agentID,
		ItemID:      itemID,
		Weight:      body.Weight,
		SortOrder:   sortOrder,
	})
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, operationResultOut(result), nil
}

// effectOut 能力输出（UUID 在模型上是 json:"-"）
type effectOut struct {
	ID         uuid.UUID     `json:"id"`
	Name       string        `json:"name"`
	Schema     model.JSONMap `json:"schema"`
	Weight     float64       `json:"weight"`
	SortOrder  int           `json:"sort_order"`
	ItemID     uuid.UUID     `json:"item_id"`     // 来源金丹实例（消耗后仍指向原实例）
	RevisionID uuid.UUID     `json:"revision_id"` // 来源丹方版本（不可变）
	CreatedAt  time.Time     `json:"created_at"`
	RemovedAt  *time.Time    `json:"removed_at,omitempty"`
}

// ListEffects 道人活跃能力列表（按 sort_order 升序；含 effects_revision 供 PUT 乐观锁）
// GET /api/v1/agents/:id/effects
func (h *Handler) ListEffects(c *gin.Context) (response.Code, any, error) {
	agentID, err := pathUUID(c, "uuid")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	effects, revision, err := h.agent.ListEffects(ctx(c), agentID)
	if err != nil {
		return 0, nil, err
	}
	out := make([]effectOut, 0, len(effects))
	for _, ef := range effects {
		out = append(out, effectOut{
			ID:         ef.Effect.UUID,
			Name:       ef.Effect.NameSnapshot,
			Schema:     ef.Effect.SchemaSnapshot,
			Weight:     ef.Effect.Weight,
			SortOrder:  ef.Effect.SortOrder,
			ItemID:     ef.ItemUUID,
			RevisionID: ef.RevisionUUID,
			CreatedAt:  ef.Effect.CreatedAt,
			RemovedAt:  ef.Effect.RemovedAt,
		})
	}
	return response.Ok, map[string]any{"effects_revision": revision, "effects": out}, nil
}

// updateEffectsBody 全量编排提交体
type updateEffectsBody struct {
	ExpectedEffectsRevision *int          `json:"expected_effects_revision" binding:"required"`
	Effects                 []effectInput `json:"effects" binding:"required"`
}

// effectInput 编排条目（effect_id 为能力对外标识）
type effectInput struct {
	EffectID  string  `json:"effect_id" binding:"required"`
	Weight    float64 `json:"weight"`
	SortOrder int     `json:"sort_order"`
}

// UpdateEffects 全量编排：提交集必须等于活跃集（缺失/重复/外部 → 409），
// expected_effects_revision 乐观锁过期同 409；成功返回更新后的道人与能力列表
// PUT /api/v1/agents/:id/effects
func (h *Handler) UpdateEffects(c *gin.Context) (response.Code, any, error) {
	agentID, err := pathUUID(c, "uuid")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	var body updateEffectsBody
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}
	items := make([]service.EffectUpdateItem, 0, len(body.Effects))
	seen := make(map[uuid.UUID]struct{}, len(body.Effects))
	for i, in := range body.Effects {
		effectID, err := uuid.Parse(in.EffectID)
		if err != nil {
			return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest,
				"handler.pill_inventory.uuid_parse", "effects[%d].effect_id 不是合法 UUID", i)
		}
		if _, dup := seen[effectID]; dup {
			return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest,
				"handler.pill_inventory.duplicate_effect", "effects 中 %s 重复", in.EffectID)
		}
		seen[effectID] = struct{}{}
		if err := validateWeight(fmt.Sprintf("effects[%d].weight", i), in.Weight); err != nil {
			return response.InvalidParams, nil, err
		}
		if err := validateSortOrder(fmt.Sprintf("effects[%d].sort_order", i), in.SortOrder); err != nil {
			return response.InvalidParams, nil, err
		}
		items = append(items, service.EffectUpdateItem{EffectID: effectID, Weight: in.Weight, SortOrder: in.SortOrder})
	}
	agent, err := h.agent.UpdateEffects(ctx(c), agentID, *body.ExpectedEffectsRevision, items)
	if err != nil {
		return 0, nil, err
	}
	// 重读能力列表（与提交同源，保证展示与提交一致）
	effects, _, err := h.agent.ListEffects(ctx(c), agentID)
	if err != nil {
		return 0, nil, err
	}
	out := make([]effectOut, 0, len(effects))
	for _, ef := range effects {
		out = append(out, effectOut{
			ID:         ef.Effect.UUID,
			Name:       ef.Effect.NameSnapshot,
			Schema:     ef.Effect.SchemaSnapshot,
			Weight:     ef.Effect.Weight,
			SortOrder:  ef.Effect.SortOrder,
			ItemID:     ef.ItemUUID,
			RevisionID: ef.RevisionUUID,
			CreatedAt:  ef.Effect.CreatedAt,
		})
	}
	return response.Ok, map[string]any{
		"effects_revision": agent.EffectsRevision,
		"effects":          out,
	}, nil
}

// RemoveEffect 显式移除能力（按能力 UUID；软删保留历史，原实例不返还）
// POST /api/v1/agents/:id/effects/:effect_id/remove
func (h *Handler) RemoveEffect(c *gin.Context) (response.Code, any, error) {
	opID, err := idempotencyKey(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	agentID, err := pathUUID(c, "uuid")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	effectID, err := pathUUID(c, "effect_id")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	if err := h.agent.RemoveEffect(ctx(c), agentID, effectID); err != nil {
		return 0, nil, err
	}
	return response.Ok, map[string]any{"operation_id": opID.String(), "effect_id": effectID.String()}, nil
}
