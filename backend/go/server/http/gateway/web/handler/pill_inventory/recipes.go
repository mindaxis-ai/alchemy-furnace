// 丹方路由：列表 / 创建 / 详情 / 版本 / 编辑 / 归档 / 炼制
// 写操作均走幂等包装（Idempotency-Key 必填）；craft 即按不可变版本单炼。
package pill_inventory

import (
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/request"
	"github.com/alchemy-furnace/server/server/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// recipeListOut 丹方列表项（UUID 在模型上是 json:"-"，此处显式携带；主键统一为 uuid 文本）
type recipeListOut struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	CurrentRevisionID string     `json:"current_revision_id"`
	ArchivedAt        *time.Time `json:"archived_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	AvailableCount    int64      `json:"available_count"` // 可用金丹实例数（GROUP BY 聚合）
	Revision          int        `json:"revision"`        // 当前版本序号（任务 6 丹方入口显示「版本 vN」）
}

// recipeDetailOut 丹方详情（含当前版本内容）
type recipeDetailOut struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	SkillSchema       model.JSONMap  `json:"skill_schema"`
	Tags              model.JSONList `json:"tags"`
	Author            string         `json:"author"`
	VersionLabel      string         `json:"version_label"`
	Revision          int            `json:"revision"`
	CurrentRevisionID string         `json:"current_revision_id"`
	ArchivedAt        *time.Time     `json:"archived_at,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}

// revisionOut 不可变版本输出
type revisionOut struct {
	ID           string         `json:"id"`
	Revision     int            `json:"revision"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	SkillSchema  model.JSONMap  `json:"skill_schema"`
	Tags         model.JSONList `json:"tags"`
	Author       string         `json:"author"`
	VersionLabel string         `json:"version_label"`
	CreatedAt    time.Time      `json:"created_at"`
}

// saveRecipeBody 创建丹方（可选同事务炼制一枚）
type saveRecipeBody struct {
	Name         string         `json:"name" binding:"required"`
	Description  string         `json:"description"`
	SkillSchema  model.JSONMap  `json:"skill_schema"`
	Tags         model.JSONList `json:"tags"`
	Author       string         `json:"author"`
	VersionLabel string         `json:"version_label"`
	CraftOne     bool           `json:"craft_one"`
}

// updateRecipeBody 编辑丹方生成新版本（expected_revision_id 提交竞争检查）
type updateRecipeBody struct {
	ExpectedRevisionID string         `json:"expected_revision_id" binding:"required"`
	Name               string         `json:"name" binding:"required"`
	Description        string         `json:"description"`
	SkillSchema        model.JSONMap  `json:"skill_schema"`
	Tags               model.JSONList `json:"tags"`
	Author             string         `json:"author"`
	VersionLabel       string         `json:"version_label"`
}

// craftBody 按不可变版本炼制一枚
type craftBody struct {
	RevisionID string `json:"revision_id" binding:"required"`
}

// ListRecipes 丹方分页
// GET /api/v1/recipes?page=&size=&keyword=&include_archived=
func (h *Handler) ListRecipes(c *gin.Context) (response.Code, any, error) {
	page := parseIntDefault(c.Query("page"), 1)
	size := parseIntDefault(c.Query("size"), 20)
	includeArchived := c.Query("include_archived") == "true"

	total, recipes, counts, err := h.inventory.ListRecipes(ctx(c), page, size, c.Query("keyword"), includeArchived)
	if err != nil {
		return 0, nil, err
	}
	out := make([]recipeListOut, 0, len(recipes))
	for _, item := range recipes {
		out = append(out, recipeListOut{
			ID:                item.PillRecipe.PillRecipeID,
			Name:              item.Name,
			CurrentRevisionID: item.CurrentRevisionUUID.String(),
			ArchivedAt:        item.PillRecipe.ArchivedAt,
			CreatedAt:         item.PillRecipe.CreatedAt,
			AvailableCount:    counts[item.PillRecipe.PillRecipeID],
			Revision:          item.Revision,
		})
	}
	return response.Ok, map[string]any{"total": total, "items": out}, nil
}

// SaveRecipe 创建丹方；craft_one=true 同事务炼制一枚
// POST /api/v1/recipes
func (h *Handler) SaveRecipe(c *gin.Context) (response.Code, any, error) {
	opID, err := idempotencyKey(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	var body saveRecipeBody
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}
	result, err := h.inventory.SaveRecipe(ctx(c), service.SaveRecipeRequest{
		OperationID: opID,
		Draft: service.RecipeDraft{
			Name:         body.Name,
			Description:  body.Description,
			SkillSchema:  body.SkillSchema,
			Tags:         body.Tags,
			Author:       body.Author,
			VersionLabel: body.VersionLabel,
		},
		CraftOne: body.CraftOne,
	})
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, operationResultOut(result), nil
}

// GetRecipe 丹方详情（任意状态可读）
// GET /api/v1/recipes/:id
func (h *Handler) GetRecipe(c *gin.Context) (response.Code, any, error) {
	recipeID, err := pathUUID(c, "id")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	recipe, rev, err := h.inventory.GetRecipe(ctx(c), recipeID)
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, recipeDetailFrom(recipe, rev), nil
}

// GetRecipeRevision 读指定不可变版本（归属校验：版本必须属于该丹方）
// GET /api/v1/recipes/:id/revisions/:revision_id
func (h *Handler) GetRecipeRevision(c *gin.Context) (response.Code, any, error) {
	recipeID, err := pathUUID(c, "id")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	revisionID, err := pathUUID(c, "revision_id")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	rev, err := h.inventory.GetRecipeRevision(ctx(c), recipeID, revisionID)
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, revisionFrom(rev), nil
}

// UpdateRecipe 编辑丹方生成新版本；expected_revision_id 必须匹配当前版本
// POST /api/v1/recipes/:id/revisions
func (h *Handler) UpdateRecipe(c *gin.Context) (response.Code, any, error) {
	opID, err := idempotencyKey(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	recipeID, err := pathUUID(c, "id")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	var body updateRecipeBody
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}
	expectedID, err := uuid.Parse(body.ExpectedRevisionID)
	if err != nil {
		return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest,
			"handler.pill_inventory.uuid_parse", "expected_revision_id 不是合法 UUID")
	}
	result, err := h.inventory.UpdateRecipe(ctx(c), service.UpdateRecipeRequest{
		OperationID:        opID,
		RecipeID:           recipeID,
		ExpectedRevisionID: expectedID,
		Draft: service.RecipeDraft{
			Name:         body.Name,
			Description:  body.Description,
			SkillSchema:  body.SkillSchema,
			Tags:         body.Tags,
			Author:       body.Author,
			VersionLabel: body.VersionLabel,
		},
	})
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, operationResultOut(result), nil
}

// ArchiveRecipe 归档丹方（停止新炼制，不删历史；幂等）
// POST /api/v1/recipes/:id/archive
func (h *Handler) ArchiveRecipe(c *gin.Context) (response.Code, any, error) {
	opID, err := idempotencyKey(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	recipeID, err := pathUUID(c, "id")
	if err != nil {
		return response.InvalidParams, nil, err
	}
	if err := h.inventory.ArchiveRecipe(ctx(c), service.ArchiveRecipeRequest{
		OperationID: opID,
		RecipeID:    recipeID,
	}); err != nil {
		return 0, nil, err
	}
	return response.Ok, map[string]any{"operation_id": opID.String()}, nil
}

// CraftPill 按不可变版本炼制一枚；归档丹方拒绝
// POST /api/v1/recipes/:id/craft
func (h *Handler) CraftPill(c *gin.Context) (response.Code, any, error) {
	opID, err := idempotencyKey(c)
	if err != nil {
		return response.InvalidParams, nil, err
	}
	// 路径中的丹方 ID 仅参与路由定位（craft 归属校验由版本唯一性承担）
	if _, err := pathUUID(c, "id"); err != nil {
		return response.InvalidParams, nil, err
	}
	var body craftBody
	if err := request.ShouldBindJSON(c, &body); err != nil {
		return response.InvalidParams, nil, err
	}
	revisionID, err := uuid.Parse(body.RevisionID)
	if err != nil {
		return response.InvalidParams, nil, errors.New(errors.ErrorTypeInvalidRequest,
			"handler.pill_inventory.uuid_parse", "revision_id 不是合法 UUID")
	}
	result, err := h.inventory.CraftOne(ctx(c), service.CraftPillRequest{
		OperationID: opID,
		RevisionID:  revisionID,
	})
	if err != nil {
		return 0, nil, err
	}
	return response.Ok, operationResultOut(result), nil
}

// ---------- DTO 组装 ----------

// operationResultOut 幂等写操作统一输出（operation_id 与请求头同值）
func operationResultOut(r *service.PillOperationResult) map[string]any {
	out := map[string]any{"operation_id": r.OperationID.String()}
	if r.RecipeID != nil {
		out["recipe_id"] = r.RecipeID.String()
	}
	if r.RevisionID != nil {
		out["revision_id"] = r.RevisionID.String()
	}
	if len(r.ItemIDs) > 0 {
		ids := make([]string, 0, len(r.ItemIDs))
		for _, uid := range r.ItemIDs {
			ids = append(ids, uid.String())
		}
		out["item_ids"] = ids
	}
	if r.EffectID != nil {
		out["effect_id"] = r.EffectID.String()
	}
	if len(r.ConsumedItemIDs) > 0 {
		ids := make([]string, 0, len(r.ConsumedItemIDs))
		for _, uid := range r.ConsumedItemIDs {
			ids = append(ids, uid.String())
		}
		out["consumed_item_ids"] = ids
	}
	return out
}

func recipeDetailFrom(recipe *model.PillRecipe, rev *model.PillRecipeRevision) recipeDetailOut {
	out := recipeDetailOut{
		ID:           recipe.PillRecipeID,
		Name:         rev.Name,
		Description:  rev.Description,
		SkillSchema:  rev.SkillSchema,
		Tags:         rev.Tags,
		Author:       rev.Author,
		VersionLabel: rev.VersionLabel,
		Revision:     rev.Revision,
		ArchivedAt:   recipe.ArchivedAt,
		CreatedAt:    recipe.CreatedAt,
	}
	if recipe.CurrentRevisionID != nil {
		out.CurrentRevisionID = rev.PillRecipeRevisionID
	}
	return out
}

func revisionFrom(rev *model.PillRecipeRevision) revisionOut {
	return revisionOut{
		ID:           rev.PillRecipeRevisionID,
		Revision:     rev.Revision,
		Name:         rev.Name,
		Description:  rev.Description,
		SkillSchema:  rev.SkillSchema,
		Tags:         rev.Tags,
		Author:       rev.Author,
		VersionLabel: rev.VersionLabel,
		CreatedAt:    rev.CreatedAt,
	}
}

// parseIntDefault 解析正整数参数，非法/缺失回退默认值
func parseIntDefault(raw string, def int) int {
	if raw == "" {
		return def
	}
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
		if n > 1e6 {
			return def
		}
	}
	if n < 1 {
		return def
	}
	return n
}
