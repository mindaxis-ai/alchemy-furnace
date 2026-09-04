// 丹方：保存 / 版本管理（v2 编辑）/ 归档 / 列表
// 产品规则：丹方永久保留；编辑产生新版本且不影响旧金丹/能力；归档只停新炼制。
package pill_inventory_service

import (
	"context"
	"encoding/json"
	stderrors "errors"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// saveRecipeHash SaveRecipe 的标准化负载哈希（kind + 全字段，CraftOne 参与）
func saveRecipeHash(req service.SaveRecipeRequest) string {
	return payloadHash("save_recipe",
		req.Draft.Name, req.Draft.Description,
		canonicalJSON(req.Draft.SkillSchema), canonicalJSON(req.Draft.Tags),
		req.Draft.Author, req.Draft.VersionLabel,
		boolStr(req.CraftOne))
}

// SaveRecipe 保存丹方；CraftOne=true 同事务炼出一枚（幂等）
func (s *Inventory) SaveRecipe(ctx context.Context, req service.SaveRecipeRequest) (*service.PillOperationResult, errors.Error) {
	if err := validateRecipeDraft(req.Draft); err != nil {
		return nil, err
	}
	return s.runOperation(ctx, req.OperationID, "save_recipe", saveRecipeHash(req),
		func(tx *gorm.DB, op *model.PillOperation) (*service.PillOperationResult, error) {
			recipe := &model.PillRecipe{CreatedAt: s.now()}
			if err := dao.CreatePillRecipe(tx, recipe); err != nil {
				return nil, err
			}
			rev := &model.PillRecipeRevision{
				RecipeID:     recipe.UUID.String(),
				Revision:     1,
				Name:         req.Draft.Name,
				Description:  req.Draft.Description,
				SkillSchema:  deepCopySchema(req.Draft.SkillSchema),
				Tags:         deepCopyList(req.Draft.Tags),
				Author:       req.Draft.Author,
				VersionLabel: orDefault(req.Draft.VersionLabel, "1.0.0"),
				CreatedAt:    s.now(),
			}
			if err := dao.CreatePillRecipeRevision(tx, rev); err != nil {
				return nil, err
			}
			if err := dao.SetPillRecipeCurrentRevision(tx, recipe.ID, rev.UUID.String()); err != nil {
				return nil, err
			}
			res := &service.PillOperationResult{
				OperationID: op.UUID,
				RecipeID:    &recipe.UUID,
				RevisionID:  &rev.UUID,
			}
			if req.CraftOne {
				item := &model.PillItem{
					RecipeRevisionID:  rev.UUID.String(),
					State:             model.PillAvailable,
					OriginOperationID: op.UUID.String(),
					OriginIndex:       0,
					CreatedAt:         s.now(),
				}
				if err := dao.CreatePillItem(tx, item); err != nil {
					return nil, err
				}
				res.ItemIDs = []uuid.UUID{item.UUID}
			}
			return res, nil
		})
}

// updateRecipeHash UpdateRecipe 的标准化负载哈希
func updateRecipeHash(req service.UpdateRecipeRequest) string {
	return payloadHash("update_recipe",
		req.RecipeID.String(), req.ExpectedRevisionID.String(),
		req.Draft.Name, req.Draft.Description,
		canonicalJSON(req.Draft.SkillSchema), canonicalJSON(req.Draft.Tags),
		req.Draft.Author, req.Draft.VersionLabel)
}

// UpdateRecipe 编辑丹方生成新版本（v2）：v1 不可变、旧实例仍引用 v1；
// expected_revision_id 必须匹配当前版本，竞争编辑返回 409 recipe.revision_conflict
func (s *Inventory) UpdateRecipe(ctx context.Context, req service.UpdateRecipeRequest) (*service.PillOperationResult, errors.Error) {
	if req.RecipeID == uuid.Nil || req.ExpectedRevisionID == uuid.Nil {
		return nil, errors.New(errors.ErrorTypeInvalidRequest, "pill.invalid_request", "缺少丹方或版本标识")
	}
	if err := validateRecipeDraft(req.Draft); err != nil {
		return nil, err
	}
	return s.runOperation(ctx, req.OperationID, "update_recipe", updateRecipeHash(req),
		func(tx *gorm.DB, op *model.PillOperation) (*service.PillOperationResult, error) {
			recipe, err := dao.PillRecipeByUUID(tx, req.RecipeID)
			if stderrors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.ErrorRecordNotFound("recipe.not_found")
			}
			if err != nil {
				return nil, err
			}
			if recipe.CurrentRevisionID == nil {
				return nil, errors.ErrorServerInternalError("recipe.invalid_state")
			}
			current, err := dao.PillRecipeRevisionByID(tx, *recipe.CurrentRevisionID)
			if err != nil {
				return nil, err
			}
			if current.UUID != req.ExpectedRevisionID {
				return nil, errors.New(errors.ErrorTypeConflict, "recipe.revision_conflict",
					"丹方已被他人更新，请刷新后重试")
			}
			rev := &model.PillRecipeRevision{
				RecipeID:     recipe.UUID.String(),
				Revision:     current.Revision + 1,
				Name:         req.Draft.Name,
				Description:  req.Draft.Description,
				SkillSchema:  deepCopySchema(req.Draft.SkillSchema),
				Tags:         deepCopyList(req.Draft.Tags),
				Author:       req.Draft.Author,
				VersionLabel: orDefault(req.Draft.VersionLabel, current.VersionLabel),
				CreatedAt:    s.now(),
			}
			if err := dao.CreatePillRecipeRevision(tx, rev); err != nil {
				return nil, err
			}
			if err := dao.SetPillRecipeCurrentRevision(tx, recipe.ID, rev.UUID.String()); err != nil {
				return nil, err
			}
			return &service.PillOperationResult{
				OperationID: op.UUID,
				RecipeID:    &recipe.UUID,
				RevisionID:  &rev.UUID,
			}, nil
		})
}

// ArchiveRecipe 归档丹方：停止新炼制，不删历史（幂等：已归档重复成功）
func (s *Inventory) ArchiveRecipe(ctx context.Context, req service.ArchiveRecipeRequest) errors.Error {
	if req.RecipeID == uuid.Nil {
		return errors.New(errors.ErrorTypeInvalidRequest, "pill.invalid_request", "缺少丹方标识")
	}
	hash := payloadHash("archive_recipe", req.RecipeID.String())
	_, err := s.runOperation(ctx, req.OperationID, "archive_recipe", hash,
		func(tx *gorm.DB, op *model.PillOperation) (*service.PillOperationResult, error) {
			recipe, err := dao.PillRecipeByUUID(tx, req.RecipeID)
			if stderrors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.ErrorRecordNotFound("recipe.not_found")
			}
			if err != nil {
				return nil, err
			}
			if recipe.ArchivedAt == nil {
				if err := dao.SetPillRecipeArchived(tx, recipe.ID, s.now()); err != nil {
					return nil, err
				}
			}
			return &service.PillOperationResult{OperationID: op.UUID, RecipeID: &recipe.UUID}, nil
		})
	return err
}

// ListRecipes 丹方分页；每丹方附带当前版本名称与可用实例数量
// （名称批量查版本表组装，UUID 在模型上是 json:"-" 不可直接对外输出）
func (s *Inventory) ListRecipes(ctx context.Context, page, size int, keyword string, includeArchived bool) (int64, []service.RecipeListItem, map[string]int64, errors.Error) {
	total, recipes, err := dao.ListPillRecipesPaged(s.db, page, size, keyword, includeArchived)
	if err != nil {
		return 0, nil, nil, errors.ErrorServerInternalError("recipe.list_failed")
	}
	revIDs := make([]string, 0, len(recipes))
	for _, r := range recipes {
		if r.CurrentRevisionID != nil {
			revIDs = append(revIDs, *r.CurrentRevisionID)
		}
	}
	revs, err := dao.PillRecipeRevisionsByIDs(s.db, revIDs)
	if err != nil {
		return 0, nil, nil, errors.ErrorServerInternalError("recipe.list_failed")
	}
	items := make([]service.RecipeListItem, 0, len(recipes))
	for _, r := range recipes {
		name := ""
		revision := 0
		var revUUID uuid.UUID
		if r.CurrentRevisionID != nil {
			if rev, ok := revs[*r.CurrentRevisionID]; ok {
				name = rev.Name
				revision = rev.Revision
				revUUID = rev.UUID
			}
		}
		items = append(items, service.RecipeListItem{PillRecipe: r, Name: name, CurrentRevisionUUID: revUUID, Revision: revision})
	}
	counts, err := dao.AvailablePillCountByRecipe(s.db)
	if err != nil {
		return 0, nil, nil, errors.ErrorServerInternalError("recipe.count_failed")
	}
	return total, items, counts, nil
}

// GetRecipe 丹方详情（含当前版本内容；任意状态可读，已归档也可读）
func (s *Inventory) GetRecipe(ctx context.Context, recipeUUID uuid.UUID) (*model.PillRecipe, *model.PillRecipeRevision, errors.Error) {
	recipe, err := dao.PillRecipeByUUID(s.db, recipeUUID)
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, errors.ErrorRecordNotFound("recipe.not_found")
	}
	if err != nil {
		return nil, nil, errors.ErrorServerInternalError("recipe.get_failed")
	}
	if recipe.CurrentRevisionID == nil {
		return nil, nil, errors.ErrorServerInternalError("recipe.invalid_state")
	}
	rev, err := dao.PillRecipeRevisionByID(s.db, *recipe.CurrentRevisionID)
	if err != nil {
		return nil, nil, errors.ErrorServerInternalError("recipe.get_failed")
	}
	return recipe, rev, nil
}

// GetRecipeRevision 读指定不可变版本；归属校验：版本必须属于该丹方，否则 404
// （防止跨丹方枚举/猜测他人版本）
func (s *Inventory) GetRecipeRevision(ctx context.Context, recipeUUID, revisionUUID uuid.UUID) (*model.PillRecipeRevision, errors.Error) {
	recipe, err := dao.PillRecipeByUUID(s.db, recipeUUID)
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.ErrorRecordNotFound("recipe.not_found")
	}
	if err != nil {
		return nil, errors.ErrorServerInternalError("recipe.get_failed")
	}
	rev, err := dao.PillRecipeRevisionByUUID(s.db, revisionUUID)
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.ErrorRecordNotFound("recipe.revision_not_found")
	}
	if err != nil {
		return nil, errors.ErrorServerInternalError("recipe.get_failed")
	}
	if rev.RecipeID != recipe.UUID.String() {
		return nil, errors.ErrorRecordNotFound("recipe.revision_not_found")
	}
	return rev, nil
}

// ---------- 小工具 ----------

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// deepCopySchema 深拷贝能力内容：不保存指向请求方可变 map 的共享对象（§3.2 快照要求）
func deepCopySchema(src model.JSONMap) model.JSONMap {
	if src == nil {
		return model.JSONMap{}
	}
	raw, err := json.Marshal(src)
	if err != nil {
		out := model.JSONMap{}
		for k, v := range src {
			out[k] = v
		}
		return out
	}
	var out model.JSONMap
	if err := json.Unmarshal(raw, &out); err != nil {
		return model.JSONMap{}
	}
	return out
}

func deepCopyList(src model.JSONList) model.JSONList {
	if src == nil {
		return model.JSONList{}
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return append(model.JSONList(nil), src...)
	}
	var out model.JSONList
	if err := json.Unmarshal(raw, &out); err != nil {
		return model.JSONList{}
	}
	return out
}
