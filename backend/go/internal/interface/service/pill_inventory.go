// PillInventory 丹方与消耗性金丹库存服务接口（金丹消耗品重构）
// 契约见 docs/superpowers/plans/2026-08-31-pill-recipes-consumables.md §2.2
// 所有写操作走幂等包装：OperationID 即全局幂等键（handler 从 Idempotency-Key 读入）
package service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// RecipeDraft 丹方草稿（保存/编辑共用）
type RecipeDraft struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	SkillSchema  model.JSONMap  `json:"skill_schema"`
	Tags         model.JSONList `json:"tags"`
	Author       string         `json:"author"`
	VersionLabel string         `json:"version_label"`
}

// SaveRecipeRequest 保存丹方请求；CraftOne=true 时同事务炼出一枚
type SaveRecipeRequest struct {
	OperationID uuid.UUID
	Draft       RecipeDraft
	CraftOne    bool
}

// CraftPillRequest 按不可变版本炼制一枚请求
type CraftPillRequest struct {
	OperationID uuid.UUID
	RevisionID  uuid.UUID
}

// UpdateRecipeRequest 编辑丹方生成新版本请求；ExpectedRevisionID 提交竞争检查
type UpdateRecipeRequest struct {
	OperationID        uuid.UUID
	RecipeID           uuid.UUID
	ExpectedRevisionID uuid.UUID
	Draft              RecipeDraft
}

// ArchiveRecipeRequest 归档丹方请求（停止新炼制，不删历史）
type ArchiveRecipeRequest struct {
	OperationID uuid.UUID
	RecipeID    uuid.UUID
}

// DiscardItemRequest 弃置金丹实例请求（available→discarded 终态）
type DiscardItemRequest struct {
	OperationID uuid.UUID
	ItemID      uuid.UUID
}

// ConsumePillRequest 服用金丹请求（任务 3 实现）
type ConsumePillRequest struct {
	OperationID uuid.UUID
	AgentID     uuid.UUID
	ItemID      uuid.UUID
	Weight      float64
	SortOrder   int
}

// PreviewFusionRequest 融合预览请求（任务 4 实现）
type PreviewFusionRequest struct {
	ItemIDs           []uuid.UUID
	ExcludeOperatorID string
}

// ConfirmFusionRequest 融合确认请求（任务 4 实现）
type ConfirmFusionRequest struct {
	OperationID uuid.UUID
	PreviewID   uuid.UUID
	Name        string
	Description string
}

// PillOperationResult 库存写操作结果（成功操作的 ResultJSON 契约）
type PillOperationResult struct {
	OperationID     uuid.UUID   `json:"operation_id"`
	RecipeID        *uuid.UUID  `json:"recipe_id,omitempty"`
	RevisionID      *uuid.UUID  `json:"revision_id,omitempty"`
	ItemIDs         []uuid.UUID `json:"item_ids,omitempty"`
	EffectID        *uuid.UUID  `json:"effect_id,omitempty"`
	ConsumedItemIDs []uuid.UUID `json:"consumed_item_ids,omitempty"`
}

// PillInventory 金丹消耗品库存接口
type PillInventory interface {
	// SaveRecipe 保存丹方；可同事务炼出一枚；幂等
	SaveRecipe(context.Context, SaveRecipeRequest) (*PillOperationResult, errors.Error)
	// CraftOne 按不可变版本炼制一枚；归档丹方拒绝；幂等
	CraftOne(context.Context, CraftPillRequest) (*PillOperationResult, errors.Error)
	// Consume 服用一枚并生成能力快照；幂等（任务 3 实现）
	Consume(context.Context, ConsumePillRequest) (*PillOperationResult, errors.Error)
	// ConfirmFusion 原子确认融合：扣全部材料并产出；幂等（任务 4 实现）
	ConfirmFusion(context.Context, ConfirmFusionRequest) (*PillOperationResult, errors.Error)
	// GetOperation 读取已提交操作结果（断线恢复用）
	GetOperation(context.Context, uuid.UUID) (*PillOperationResult, errors.Error)

	// UpdateRecipe 编辑丹方生成新版本；ExpectedRevisionID 竞争检查；幂等（任务 5 暴露）
	UpdateRecipe(context.Context, UpdateRecipeRequest) (*PillOperationResult, errors.Error)
	// ArchiveRecipe 归档丹方（停止新炼制，不删历史）；幂等
	ArchiveRecipe(context.Context, ArchiveRecipeRequest) errors.Error
	// DiscardItem 弃置金丹实例（available→discarded 终态）；幂等
	DiscardItem(context.Context, DiscardItemRequest) errors.Error

	// ListRecipes 丹方分页；includeArchived 含归档；返回每丹方可用实例数（键为丹方 UUID 文本）
	ListRecipes(ctx context.Context, page, size int, keyword string, includeArchived bool) (int64, []RecipeListItem, map[string]int64, errors.Error)
	// GetRecipe 丹方详情（含当前版本内容；任意状态可读）
	GetRecipe(ctx context.Context, uid uuid.UUID) (*model.PillRecipe, *model.PillRecipeRevision, errors.Error)
	// GetRecipeRevision 读指定版本；归属校验：版本必须属于该丹方，否则 404
	GetRecipeRevision(ctx context.Context, recipeID, revisionID uuid.UUID) (*model.PillRecipeRevision, errors.Error)
	// ListItems 可用库存分页；recipeID 非空时按丹方过滤；每项含来源丹方/版本对外标识
	ListItems(ctx context.Context, page, size int, recipeID *uuid.UUID) (int64, []ItemListItem, errors.Error)
	// GetItem 按 UUID 读金丹实例（任意状态可读，含来源丹方与版本内容；已消耗/弃置展示去向）
	GetItem(ctx context.Context, uid uuid.UUID) (*ItemDetail, errors.Error)
}

// RecipeListItem 丹方列表项（补当前版本名称与对外 UUID；模型上的 UUID 是 json:"-"）
type RecipeListItem struct {
	PillRecipe          model.PillRecipe
	Name                string
	CurrentRevisionUUID uuid.UUID
	// 当前版本序号（任务 6 丹方入口显示「版本 vN」）
	Revision int
}

// ItemListItem 库存列表项（来源丹方/版本对外标识 + 名称，任务 5）
type ItemListItem struct {
	Item         model.PillItem
	RecipeUUID   uuid.UUID
	RevisionUUID uuid.UUID
	RecipeName   string
	Revision     int
}

// ItemDetail 金丹实例详情（任意状态，含来源丹方与版本内容，展示去向用）
type ItemDetail struct {
	Item     model.PillItem
	Recipe   model.PillRecipe
	Revision model.PillRecipeRevision
}
