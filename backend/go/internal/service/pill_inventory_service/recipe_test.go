// 任务 2 测试：丹方保存/版本管理/炼制/库存查询 + 幂等包装
// 覆盖 plan 契约：SaveRecipe 幂等、同 key 改参 409、v2 编辑 v1 不变、
// 归档禁炼制、弃置终态、GROUP BY 聚合数量、GetOperation 断线恢复
package pill_inventory_service

import (
	"context"

	"github.com/alchemy-furnace/server/internal/interface/service"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// inventoryModels 任务 2/3 服务测试需要的模型全集（与 dao.pillInventoryModels 对齐）
func inventoryModels() []any {
	return []any{
		&model.PillRecipe{}, &model.PillRecipeRevision{}, &model.PillItem{},
		&model.AgentPillEffect{}, &model.PillOperation{}, &model.FusionPreview{},
		&model.PillStarterGrant{},
		// 任务 3：服用事务操作道人（EffectsRevision 递增 + 缓存失效）
		&model.DaoAgent{}, &model.LanguagePattern{},
	}
}

// openInventoryDB 服务层测试夹具：临时文件 SQLite + 外键 + 单连接
func openInventoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	return openInventoryDBAt(t, filepath.Join(t.TempDir(), "inventory.db"))
}

// openInventoryDBAt 打开指定路径的 SQLite（任务 4 竞态测试：两个连接指向同一文件）
func openInventoryDBAt(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = raw.Close() })
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(inventoryModels()...); err != nil {
		t.Fatal(err)
	}
	return db
}

func newTestSvc(t *testing.T) (*Inventory, *gorm.DB) {
	t.Helper()
	db := openInventoryDB(t)
	fixed := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	return New(db, func() time.Time { return fixed }), db
}

// minSchema 合法最小 schema + 未知字段（往返不丢失验证用）
func minSchema() model.JSONMap {
	return model.JSONMap{
		"expression_dna":   map[string]any{"sentence_length": "mixed"},
		"future_extension": map[string]any{"keep": true, "nested": []any{"a", "b"}},
	}
}

func draftOf(name string, schema model.JSONMap) service.RecipeDraft {
	return service.RecipeDraft{Name: name, Description: "desc", SkillSchema: schema, VersionLabel: "1.0.0"}
}

// ---------- SaveRecipe：0 枚 / 1 枚 / 幂等 / 改参 409 ----------

// TestSaveRecipeCraftFalseProducesNoItems 保存丹方不炼制：0 枚；未知字段往返不丢失
func TestSaveRecipeCraftFalseProducesNoItems(t *testing.T) {
	svc, db := newTestSvc(t)
	key := uuid.New()
	req := service.SaveRecipeRequest{OperationID: key, Draft: draftOf("测试丹方", minSchema())}

	first, err := svc.SaveRecipe(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveRecipe: %v", err)
	}
	if first.RecipeID == nil || first.RevisionID == nil {
		t.Fatalf("应返回丹方与版本: %+v", first)
	}
	if len(first.ItemIDs) != 0 {
		t.Fatalf("craft=false 不应产出实例: %+v", first)
	}
	var items int64
	if err := db.Model(&model.PillItem{}).Count(&items).Error; err != nil {
		t.Fatal(err)
	}
	if items != 0 {
		t.Fatalf("pill_items=%d, want 0", items)
	}

	// 版本内容往返：未知字段完整保留
	var rev model.PillRecipeRevision
	if err := db.First(&rev).Error; err != nil {
		t.Fatal(err)
	}
	if _, ok := rev.SkillSchema["future_extension"]; !ok {
		t.Fatalf("未知字段丢失: %+v", rev.SkillSchema)
	}
	if v, _ := rev.SkillSchema["future_extension"].(map[string]any)["keep"]; v != true {
		t.Fatalf("嵌套未知字段丢失: %+v", rev.SkillSchema["future_extension"])
	}
}

// TestSaveRecipeCraftTrueProducesOneItem craft=true 同事务炼出一枚可用实例
func TestSaveRecipeCraftTrueProducesOneItem(t *testing.T) {
	svc, db := newTestSvc(t)
	req := service.SaveRecipeRequest{OperationID: uuid.New(), CraftOne: true, Draft: draftOf("炼制丹", minSchema())}

	res, err := svc.SaveRecipe(context.Background(), req)
	if err != nil {
		t.Fatalf("SaveRecipe: %v", err)
	}
	if len(res.ItemIDs) != 1 {
		t.Fatalf("ItemIDs=%v, want 1 枚", res.ItemIDs)
	}
	var item model.PillItem
	if err := db.Where("pill_item_id = ?", res.ItemIDs[0].String()).First(&item).Error; err != nil {
		t.Fatalf("实例未落库: %v", err)
	}
	if item.State != model.PillAvailable {
		t.Fatalf("state=%s, want available", item.State)
	}
	var rev model.PillRecipeRevision
	if err := db.First(&rev).Error; err != nil {
		t.Fatal(err)
	}
	if item.RecipeRevisionID != rev.PillRecipeRevisionID {
		t.Fatalf("实例未引用新版本: item=%s rev=%s", item.RecipeRevisionID, rev.PillRecipeRevisionID)
	}
}

// TestSaveRecipeIdempotentSameKey 同 key 重试返回相同结果（幂等）
func TestSaveRecipeIdempotentSameKey(t *testing.T) {
	svc, _ := newTestSvc(t)
	key := uuid.New()
	req := service.SaveRecipeRequest{OperationID: key, CraftOne: true, Draft: draftOf("幂等丹", minSchema())}

	first, err := svc.SaveRecipe(context.Background(), req)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.SaveRecipe(context.Background(), req)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("幂等破坏: first=%+v second=%+v", first, second)
	}
}

// TestSaveRecipeSameKeyDifferentPayloadConflict 同 key 改参数 → 409
func TestSaveRecipeSameKeyDifferentPayloadConflict(t *testing.T) {
	svc, _ := newTestSvc(t)
	key := uuid.New()
	req := service.SaveRecipeRequest{OperationID: key, CraftOne: true, Draft: draftOf("原丹", minSchema())}
	if _, err := svc.SaveRecipe(context.Background(), req); err != nil {
		t.Fatalf("first: %v", err)
	}

	req.Draft.Name = "改了名字"
	_, err := svc.SaveRecipe(context.Background(), req)
	if err == nil {
		t.Fatal("同 key 改参应 409")
	}
	if err.GetCode() != "pill.operation_payload_mismatch" {
		t.Fatalf("code=%s, want pill.operation_payload_mismatch", err.GetCode())
	}
}

// ---------- CraftOne：按版本炼制 / 归档拒绝 ----------

// TestCraftOneByRevision 用 SaveRecipe 返回的版本炼制一枚
func TestCraftOneByRevision(t *testing.T) {
	svc, db := newTestSvc(t)
	saveKey := uuid.New()
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: saveKey, Draft: draftOf("炼制源丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}

	craftKey := uuid.New()
	res, err := svc.CraftOne(context.Background(), service.CraftPillRequest{
		OperationID: craftKey, RevisionID: *saved.RevisionID,
	})
	if err != nil {
		t.Fatalf("CraftOne: %v", err)
	}
	if len(res.ItemIDs) != 1 || res.RecipeID == nil {
		t.Fatalf("CraftOne 结果异常: %+v", res)
	}
	if res.RecipeID.String() != saved.RecipeID.String() {
		t.Fatalf("炼制产物属于其它丹方: %v vs %v", res.RecipeID, saved.RecipeID)
	}
	// 重试同 key 幂等
	again, err := svc.CraftOne(context.Background(), service.CraftPillRequest{
		OperationID: craftKey, RevisionID: *saved.RevisionID,
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !reflect.DeepEqual(res, again) {
		t.Fatalf("重试结果不一致: %+v vs %+v", res, again)
	}
	var items int64
	db.Model(&model.PillItem{}).Count(&items)
	if items != 1 {
		t.Fatalf("pill_items=%d, want 1（重试不得新增）", items)
	}
}

// TestCraftOneArchivedRecipeRejected 归档丹方禁止新炼制
func TestCraftOneArchivedRecipeRejected(t *testing.T) {
	svc, _ := newTestSvc(t)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), Draft: draftOf("归档丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveRecipe(context.Background(), service.ArchiveRecipeRequest{
		OperationID: uuid.New(), RecipeID: *saved.RecipeID,
	}); err != nil {
		t.Fatalf("ArchiveRecipe: %v", err)
	}

	_, err = svc.CraftOne(context.Background(), service.CraftPillRequest{
		OperationID: uuid.New(), RevisionID: *saved.RevisionID,
	})
	if err == nil {
		t.Fatal("归档后炼制应被拒绝")
	}
	if err.GetCode() != "recipe.archived" {
		t.Fatalf("code=%s, want recipe.archived", err.GetCode())
	}
}

// ---------- 版本编辑：v2 产生 / v1 不变 / 竞争 409 ----------

// TestUpdateRecipeCreatesV2KeepsV1 编辑产生 v2；v1 内容不变；旧实例仍引用 v1
func TestUpdateRecipeCreatesV2KeepsV1(t *testing.T) {
	svc, db := newTestSvc(t)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("演进丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	itemUUID := saved.ItemIDs[0]

	newSchema := minSchema()
	newSchema["expression_dna"] = map[string]any{"sentence_length": "long", "tone": "沉稳"}
	res, err := svc.UpdateRecipe(context.Background(), service.UpdateRecipeRequest{
		OperationID: uuid.New(), RecipeID: *saved.RecipeID,
		ExpectedRevisionID: *saved.RevisionID,
		Draft:              draftOf("演进丹v2", newSchema),
	})
	if err != nil {
		t.Fatalf("UpdateRecipe: %v", err)
	}
	if res.RevisionID == nil || res.RevisionID.String() == saved.RevisionID.String() {
		t.Fatalf("应产生新版本: %+v vs %+v", res.RevisionID, saved.RevisionID)
	}

	// v1 内容不变
	var v1 model.PillRecipeRevision
	if err := db.Where("pill_recipe_revision_id = ?", saved.RevisionID).First(&v1).Error; err != nil {
		t.Fatal(err)
	}
	if v1.Name != "演进丹" {
		t.Fatalf("v1 被原地修改: %q", v1.Name)
	}
	if v1.SkillSchema["future_extension"] == nil {
		t.Fatalf("v1 schema 被破坏: %+v", v1.SkillSchema)
	}
	// v2 是新内容
	var v2 model.PillRecipeRevision
	if err := db.Where("pill_recipe_revision_id = ?", res.RevisionID).First(&v2).Error; err != nil {
		t.Fatal(err)
	}
	if v2.Name != "演进丹v2" || v2.Revision != 2 {
		t.Fatalf("v2 异常: name=%q revision=%d", v2.Name, v2.Revision)
	}
	// 丹方当前版本指向 v2
	var recipe model.PillRecipe
	if err := db.Where("pill_recipe_id = ?", saved.RecipeID).First(&recipe).Error; err != nil {
		t.Fatal(err)
	}
	if recipe.CurrentRevisionID == nil || *recipe.CurrentRevisionID != v2.PillRecipeRevisionID {
		t.Fatalf("current_revision 未指向 v2: %v", recipe.CurrentRevisionID)
	}
	// 旧实例仍引用 v1
	var item model.PillItem
	if err := db.Where("pill_item_id = ?", itemUUID.String()).First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.RecipeRevisionID != v1.PillRecipeRevisionID {
		t.Fatalf("旧实例被迁移到新版本: item.rev=%s v1.id=%s", item.RecipeRevisionID, v1.PillRecipeRevisionID)
	}
}

// TestUpdateRecipeRevisionConflict expected_revision_id 不匹配 → 409
func TestUpdateRecipeRevisionConflict(t *testing.T) {
	svc, _ := newTestSvc(t)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), Draft: draftOf("竞争丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.UpdateRecipe(context.Background(), service.UpdateRecipeRequest{
		OperationID: uuid.New(), RecipeID: *saved.RecipeID,
		ExpectedRevisionID: uuid.New(), // 编造一个不存在的版本
		Draft:              draftOf("竞争丹v2", minSchema()),
	})
	if err == nil {
		t.Fatal("expected_revision_id 不匹配应 409")
	}
	if err.GetCode() != "recipe.revision_conflict" {
		t.Fatalf("code=%s, want recipe.revision_conflict", err.GetCode())
	}
}

// ---------- 归档 / 弃置终态 ----------

// TestArchiveRecipeKeepsOldItemsReadable 归档后可读旧实例，不可炼制
func TestArchiveRecipeKeepsOldItemsReadable(t *testing.T) {
	svc, _ := newTestSvc(t)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("归档留用丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	itemID := saved.ItemIDs[0]
	if err := svc.ArchiveRecipe(context.Background(), service.ArchiveRecipeRequest{
		OperationID: uuid.New(), RecipeID: *saved.RecipeID,
	}); err != nil {
		t.Fatal(err)
	}

	// 旧实例仍可读（含版本内容）
	detail, err := svc.GetItem(context.Background(), itemID)
	if err != nil {
		t.Fatalf("归档后旧实例不可读: %v", err)
	}
	if detail.Item.State != model.PillAvailable || detail.Revision.Revision != 1 {
		t.Fatalf("旧实例异常: %+v", detail.Item)
	}
	// 再归档一次：幂等成功
	if err := svc.ArchiveRecipe(context.Background(), service.ArchiveRecipeRequest{
		OperationID: uuid.New(), RecipeID: *saved.RecipeID,
	}); err != nil {
		t.Fatalf("重复归档应幂等成功: %v", err)
	}
}

// TestDiscardItemTerminalState 弃置终态：discarded 不可再弃置；实例仍可读（保留去向）
func TestDiscardItemTerminalState(t *testing.T) {
	svc, db := newTestSvc(t)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("弃置丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	itemID := saved.ItemIDs[0]

	if err := svc.DiscardItem(context.Background(), service.DiscardItemRequest{
		OperationID: uuid.New(), ItemID: itemID,
	}); err != nil {
		t.Fatalf("DiscardItem: %v", err)
	}
	var item model.PillItem
	if err := db.Where("pill_item_id = ?", itemID.String()).First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.State != model.PillDiscarded {
		t.Fatalf("state=%s, want discarded", item.State)
	}
	if item.ConsumedAt != nil {
		t.Fatalf("弃置不应写 consumed_at: %v", item.ConsumedAt)
	}

	// 再次弃置 → 409 pill.not_available
	err = svc.DiscardItem(context.Background(), service.DiscardItemRequest{
		OperationID: uuid.New(), ItemID: itemID,
	})
	if err == nil {
		t.Fatal("二次弃置应 409")
	}
	if err.GetCode() != "pill.not_available" {
		t.Fatalf("code=%s, want pill.not_available", err.GetCode())
	}
	// 实例仍可读（去向展示）
	if _, err := svc.GetItem(context.Background(), itemID); err != nil {
		t.Fatalf("弃置后实例不可读: %v", err)
	}
}

// ---------- 查询：分页与聚合数量 ----------

// TestListRecipesAvailableCounts 可用数量按 state='available' GROUP BY recipe_id 聚合
func TestListRecipesAvailableCounts(t *testing.T) {
	svc, _ := newTestSvc(t)
	// A：炼制 2 枚，弃置 1 → 可用 1；B：炼制 0 → 无可用
	a, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("多产丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CraftOne(context.Background(), service.CraftPillRequest{
		OperationID: uuid.New(), RevisionID: *a.RevisionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DiscardItem(context.Background(), service.DiscardItemRequest{
		OperationID: uuid.New(), ItemID: second.ItemIDs[0],
	}); err != nil {
		t.Fatal(err)
	}
	b, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), Draft: draftOf("空库存丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}

	total, recipes, counts, err := svc.ListRecipes(context.Background(), 1, 10, "", false)
	if err != nil {
		t.Fatalf("ListRecipes: %v", err)
	}
	if total != 2 || len(recipes) != 2 {
		t.Fatalf("total=%d len=%d, want 2/2", total, len(recipes))
	}
	got := map[string]int64{}
	for _, r := range recipes {
		got[r.PillRecipe.PillRecipeID] = counts[r.PillRecipe.PillRecipeID]
	}
	if got[a.RecipeID.String()] != 1 {
		t.Fatalf("多产丹可用数=%d, want 1", got[a.RecipeID.String()])
	}
	if v, ok := got[b.RecipeID.String()]; ok && v != 0 {
		t.Fatalf("空库存丹不应有可用数: %d", v)
	}
}

// TestListItemsPaging 库存分页 + recipe_id 过滤
func TestListItemsPaging(t *testing.T) {
	svc, _ := newTestSvc(t)
	a, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("分页丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.CraftOne(context.Background(), service.CraftPillRequest{
			OperationID: uuid.New(), RevisionID: *a.RevisionID,
		}); err != nil {
			t.Fatal(err)
		}
	}

	total, items, err := svc.ListItems(context.Background(), 1, 2, nil)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if total != 3 || len(items) != 2 {
		t.Fatalf("page1: total=%d len=%d, want 3/2", total, len(items))
	}
	_, page2, err := svc.ListItems(context.Background(), 2, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 len=%d, want 1", len(page2))
	}
}

// ---------- GetOperation：断线恢复 ----------

// TestGetOperationReturnsCommittedResult 已提交操作可经 GetOperation 恢复
func TestGetOperationReturnsCommittedResult(t *testing.T) {
	svc, _ := newTestSvc(t)
	key := uuid.New()
	req := service.SaveRecipeRequest{OperationID: key, CraftOne: true, Draft: draftOf("恢复丹", minSchema())}
	want, err := svc.SaveRecipe(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.GetOperation(context.Background(), key)
	if err != nil {
		t.Fatalf("GetOperation: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("恢复结果不一致: want=%+v got=%+v", want, got)
	}
}

// TestGetOperationNotFound 未知操作 → 404
func TestGetOperationNotFound(t *testing.T) {
	svc, _ := newTestSvc(t)
	_, err := svc.GetOperation(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("未知操作应 404")
	}
	if !err.IsType(errors.ErrorTypeRecordNotFound) {
		t.Fatalf("应返回 RecordNotFound: %v", err)
	}
}

// ---------- schema 校验 ----------

// TestSaveRecipeInvalidSchemaRejected 非法 schema → 400 recipe.invalid_schema
func TestSaveRecipeInvalidSchemaRejected(t *testing.T) {
	svc, _ := newTestSvc(t)
	cases := []struct {
		name   string
		schema model.JSONMap
	}{
		{"空 schema", model.JSONMap{}},
		{"缺 expression_dna", model.JSONMap{"mental_models": []any{}}},
		{"expression_dna 非对象", model.JSONMap{"expression_dna": "mixed"}},
		{"mental_models 超长", model.JSONMap{"expression_dna": map[string]any{}, "mental_models": make([]any, 21)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
				OperationID: uuid.New(), Draft: draftOf("坏丹", c.schema),
			})
			if err == nil {
				t.Fatal("非法 schema 应被拒绝")
			}
			if err.GetCode() != "recipe.invalid_schema" {
				t.Fatalf("code=%s, want recipe.invalid_schema", err.GetCode())
			}
		})
	}
}
