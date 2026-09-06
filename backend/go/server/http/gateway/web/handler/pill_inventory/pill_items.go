// 金丹库存路由：可用库存分页 / 实例详情（任意状态，展示去向）/ 弃置终态
package pill_inventory

import (
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// itemListOut 库存列表项（可用实例；主键统一为 uuid 文本）
type itemListOut struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`        // 来源丹方当前版本名称
	State      string    `json:"state"`       // available（本列表恒为 available）
	RecipeID   string    `json:"recipe_id"`   // 来源丹方对外标识
	RevisionID string    `json:"revision_id"` // 来源不可变版本对外标识
	Revision   int       `json:"revision"`    // 版本号
	CreatedAt  time.Time `json:"created_at"`
}

// itemDetailOut 实例详情（任意状态可读；已消耗/弃置展示去向）
type itemDetailOut struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Tags         model.JSONList `json:"tags"`  // 来源版本标签(hero spotlight 丹性行)
	State        string         `json:"state"` // available / consumed_by_agent / consumed_by_fusion / discarded
	RecipeID     string         `json:"recipe_id"`
	RevisionID   string         `json:"revision_id"`
	Revision     int            `json:"revision"`
	VersionLabel string         `json:"version_label"`
	ArchivedAt   *time.Time     `json:"archived_at,omitempty"` // 来源丹方已归档时展示
	ConsumedAt   *time.Time     `json:"consumed_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// ListPillItems 可用库存分页；recipe_id 非空时按丹方过滤
// GET /api/v1/pill-items?page=&size=&recipe_id=
func (h *Handler) ListPillItems(c *gin.Context) (response.Code, any, error) {
	var recipeID *uuid.UUID
	if raw := c.Query("recipe_id"); raw != "" {
		uid, err := uuid.Parse(raw)
		if err != nil {
			return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest,
				"handler.pill_inventory.uuid_parse", "recipe_id 不是合法 UUID")
		}
		recipeID = &uid
	}
	total, items, err := h.inventory.ListItems(ctx(c), parseIntDefault(c.Query("page"), 1), parseIntDefault(c.Query("size"), 20), recipeID)
	if err != nil {
		return 0, nil, err
	}
	out := make([]itemListOut, 0, len(items))
	for _, it := range items {
		out = append(out, itemListOut{
			ID:         it.Item.PillItemID,
			Name:       it.RecipeName,
			State:      string(it.Item.State),
			RecipeID:   it.RecipeUUID.String(),
			RevisionID: it.RevisionUUID.String(),
			Revision:   it.Revision,
			CreatedAt:  it.Item.CreatedAt,
		})
	}
	return response.Ok, map[string]any{"total": total, "items": out}, nil
}

// GetPillItem 实例详情（任意状态；已消耗/弃置展示状态去向）
// GET /api/v1/pill-items/:id
func (h *Handler) GetPillItem(c *gin.Context) (response.Code, any, error) {
	id, err := pathUUID(c, "id")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	detail, err := h.inventory.GetItem(ctx(c), id)
	if err != nil {
		return 0, nil, err
	}
	var archivedAt *time.Time
	if detail.Recipe.ArchivedAt != nil {
		archivedAt = detail.Recipe.ArchivedAt
	}
	return response.Ok, itemDetailOut{
		ID:           detail.Item.PillItemID,
		Name:         detail.Revision.Name,
		Description:  detail.Revision.Description,
		Tags:         detail.Revision.Tags,
		State:        string(detail.Item.State),
		RecipeID:     detail.Recipe.PillRecipeID,
		RevisionID:   detail.Revision.PillRecipeRevisionID,
		Revision:     detail.Revision.Revision,
		VersionLabel: detail.Revision.VersionLabel,
		ArchivedAt:   archivedAt,
		ConsumedAt:   detail.Item.ConsumedAt,
		CreatedAt:    detail.Item.CreatedAt,
	}, nil
}

// DiscardItem 弃置金丹：available→discarded 终态（显式确认，不物理删除；幂等）
// POST /api/v1/pill-items/:id/discard
func (h *Handler) DiscardItem(c *gin.Context) (response.Code, any, error) {
	opID, err := idempotencyKey(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	id, err := pathUUID(c, "id")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	if err := h.inventory.DiscardItem(ctx(c), service.DiscardItemRequest{OperationID: opID, ItemID: id}); err != nil {
		return 0, nil, err
	}
	return response.Ok, map[string]any{"operation_id": opID.String()}, nil
}
