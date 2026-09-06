// 金丹实例：炼制 / 库存查询 / 弃置终态
// 产品规则：服用/弃置为终态但实例永不可删（去向展示）；弃置不写 consumed_at。
package pill_inventory_service

import (
	"context"
	stderrors "errors"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CraftOne 按不可变版本炼制一枚；归档丹方拒绝；幂等
func (s *Inventory) CraftOne(ctx context.Context, req service.CraftPillRequest) (*service.PillOperationResult, errors.Error) {
	if req.RevisionID == uuid.Nil {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "pill.invalid_request", "缺少版本标识")
	}
	return s.runOperation(ctx, req.OperationID, "craft_one", payloadHash("craft_one", req.RevisionID.String()),
		func(tx *gorm.DB, op *model.PillOperation) (*service.PillOperationResult, error) {
			rev, err := dao.PillRecipeRevisionByUUID(tx, req.RevisionID)
			if stderrors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.ErrorRecordNotFound("recipe.not_found")
			}
			if err != nil {
				return nil, err
			}
			recipe, err := dao.PillRecipeByID(tx, rev.RecipeID)
			if err != nil {
				return nil, err
			}
			if recipe.ArchivedAt != nil {
				return nil, errors.New(errors.ErrorTypeConflict, "recipe.archived",
					"丹方已归档，禁止新炼制")
			}
			item := &model.PillItem{
				RecipeRevisionID:  rev.PillRecipeRevisionID,
				State:             model.PillAvailable,
				OriginOperationID: op.PillOperationID,
				OriginIndex:       0,
				CreatedAt:         s.now(),
			}
			if err := dao.CreatePillItem(tx, item); err != nil {
				return nil, err
			}
			return &service.PillOperationResult{
				OperationID: req.OperationID,
				RecipeID:    uuidPtr(recipe.PillRecipeID),
				RevisionID:  uuidPtr(rev.PillRecipeRevisionID),
				ItemIDs:     []uuid.UUID{uuidVal(item.PillItemID)},
			}, nil
		})
}

// ListItems 可用库存分页；recipeID 非空时按丹方过滤；
// 每项组装来源丹方/版本对外标识与名称（UUID 在模型上是 json:"-"）
func (s *Inventory) ListItems(ctx context.Context, page, size int, recipeID *uuid.UUID) (int64, []service.ItemListItem, errors.Error) {
	var recipeUID *string
	if recipeID != nil {
		recipe, err := dao.PillRecipeByUUID(s.db, *recipeID)
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil, errors.ErrorRecordNotFound("recipe.not_found")
		}
		if err != nil {
			return 0, nil, errors.ErrorServerInternalError("pill.items_query_failed")
		}
		uid := recipe.PillRecipeID
		recipeUID = &uid
	}
	total, items, err := dao.ListAvailablePillItems(s.db, page, size, recipeUID)
	if err != nil {
		return 0, nil, errors.ErrorServerInternalError("pill.items_query_failed")
	}
	revIDs := make([]string, 0, len(items))
	for _, it := range items {
		revIDs = append(revIDs, it.RecipeRevisionID)
	}
	revs, err := dao.PillRecipeRevisionsByIDs(s.db, revIDs)
	if err != nil {
		return 0, nil, errors.ErrorServerInternalError("pill.items_query_failed")
	}
	recipeIDs := make([]string, 0, len(revs))
	for _, r := range revs {
		recipeIDs = append(recipeIDs, r.RecipeID)
	}
	recipes, err := dao.PillRecipesByIDs(s.db, recipeIDs)
	if err != nil {
		return 0, nil, errors.ErrorServerInternalError("pill.items_query_failed")
	}
	out := make([]service.ItemListItem, 0, len(items))
	for _, it := range items {
		rv, okRev := revs[it.RecipeRevisionID]
		if !okRev {
			return 0, nil, errors.ErrorServerInternalError("pill.items_query_failed")
		}
		item := service.ItemListItem{Item: it, RevisionUUID: uuidVal(rv.PillRecipeRevisionID), Revision: rv.Revision, RecipeName: rv.Name}
		if recipe, ok := recipes[rv.RecipeID]; ok {
			item.RecipeUUID = uuidVal(recipe.PillRecipeID)
		}
		out = append(out, item)
	}
	return total, out, nil
}

// GetItem 按 UUID 读金丹实例（任意状态可读，含来源丹方与版本内容；已消耗/弃置展示去向）
func (s *Inventory) GetItem(ctx context.Context, id uuid.UUID) (*service.ItemDetail, errors.Error) {
	if id == uuid.Nil {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "pill.invalid_request", "缺少实例标识")
	}
	item, err := dao.PillItemByUUID(s.db, id)
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.ErrorRecordNotFound("pill.not_found")
	}
	if err != nil {
		return nil, errors.ErrorServerInternalError("pill.item_query_failed")
	}
	rev, err := dao.PillRecipeRevisionByID(s.db, item.RecipeRevisionID)
	if err != nil {
		return nil, errors.ErrorServerInternalError("pill.revision_query_failed")
	}
	recipe, err := dao.PillRecipeByID(s.db, rev.RecipeID)
	if err != nil {
		return nil, errors.ErrorServerInternalError("pill.recipe_query_failed")
	}
	return &service.ItemDetail{Item: *item, Revision: *rev, Recipe: *recipe}, nil
}

// DiscardItem 弃置金丹：available→discarded 终态；重复/已消耗 → 409 pill.not_available
func (s *Inventory) DiscardItem(ctx context.Context, req service.DiscardItemRequest) errors.Error {
	if req.ItemID == uuid.Nil {
		return errors.New(errors.ErrorTypeInvalidRequest, "pill.invalid_request", "缺少实例标识")
	}
	_, err := s.runOperation(ctx, req.OperationID, "discard_item", payloadHash("discard_item", req.ItemID.String()),
		func(tx *gorm.DB, op *model.PillOperation) (*service.PillOperationResult, error) {
			item, err := dao.PillItemByUUID(tx, req.ItemID)
			if stderrors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.ErrorRecordNotFound("pill.not_found")
			}
			if err != nil {
				return nil, err
			}
			// CAS：只有 available 可转 discarded；竞争/重复触发返回 false
			ok, err := dao.DiscardPillItemCAS(tx, item.PillItemID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errors.New(errors.ErrorTypeConflict, "pill.not_available",
					"金丹不可弃置（已被服用或已弃置）")
			}
			return &service.PillOperationResult{OperationID: req.OperationID}, nil
		})
	return err
}
