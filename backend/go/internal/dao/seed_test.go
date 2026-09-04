package dao

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSeedBuiltinPillsIsIdempotentAndPreservesExistingData(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.ElixirPill{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if err := SeedBuiltinPills(db); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if err := db.Model(&model.ElixirPill{}).
		Where("name = ?", "文言文金丹").
		Update("description", "用户保留的说明").Error; err != nil {
		t.Fatalf("customize builtin: %v", err)
	}
	if err := SeedBuiltinPills(db); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	var count int64
	if err := db.Model(&model.ElixirPill{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 5 {
		t.Fatalf("pill count = %d, want 5", count)
	}

	var pill model.ElixirPill
	if err := db.Where("name = ?", "文言文金丹").First(&pill).Error; err != nil {
		t.Fatalf("load customized pill: %v", err)
	}
	if pill.Description != "用户保留的说明" {
		t.Fatalf("description was overwritten: %q", pill.Description)
	}
}

// TestSeedBuiltinRecipesAndGrantFreshInstall 全新安装：
// 5 个内置丹方 + 每丹方赠送 1 枚可用金丹（available）；重复执行不重复产出（重启不自动补货）
func TestSeedBuiltinRecipesAndGrantFreshInstall(t *testing.T) {
	db := openInventoryTestDB(t, inventoryTestModels()...)
	if err := SeedBuiltinRecipes(db); err != nil {
		t.Fatalf("seed recipes: %v", err)
	}
	if err := GrantStarterPills(db); err != nil {
		t.Fatalf("grant: %v", err)
	}

	assertTableCount(t, db, "pill_recipes", 5)
	assertTableCount(t, db, "pill_recipe_revisions", 5)
	assertTableCount(t, db, "pill_items", 5)
	var available int64
	if err := db.Table("pill_items").Where("state = ?", "available").Count(&available).Error; err != nil {
		t.Fatal(err)
	}
	if available != 5 {
		t.Fatalf("available=%d, want 5", available)
	}
	var granted int64
	if err := db.Model(&model.PillStarterGrant{}).Count(&granted).Error; err != nil {
		t.Fatal(err)
	}
	if granted != 5 {
		t.Fatalf("starter grants=%d, want 5", granted)
	}
	// 每丹方恰好 1 枚赠送实例
	var withItem int64
	if err := db.Model(&model.PillStarterGrant{}).Where("item_id IS NOT NULL").Count(&withItem).Error; err != nil {
		t.Fatal(err)
	}
	if withItem != 5 {
		t.Fatalf("带实例的赠送记录=%d, want 5", withItem)
	}

	// 第二次执行：不新增任何数据（防重启自动补货）
	if err := SeedBuiltinRecipes(db); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if err := GrantStarterPills(db); err != nil {
		t.Fatalf("second grant: %v", err)
	}
	for _, table := range []string{"pill_recipes", "pill_recipe_revisions", "pill_items", "pill_starter_grants"} {
		assertTableCount(t, db, table, 5)
	}
}

// TestSeedGrantStarterPills 一次性赠送（迁移子系统已移除，赠送只认内置丹方）：
// 全新库首次赠送：每内置丹方恰好 1 枚 available 实例 + 1 条赠送记录；
// 重复启动：重复 GrantStarterPills（再跑一次种子也无妨）不新增 item、不新增 grant；
// 赠送操作 PayloadHash = hex(sha256("starter_grant|" + recipeUUID))
func TestSeedGrantStarterPills(t *testing.T) {
	db := openInventoryTestDB(t, inventoryTestModels()...)
	if err := SeedBuiltinRecipes(db); err != nil {
		t.Fatalf("seed recipes: %v", err)
	}
	if err := GrantStarterPills(db); err != nil {
		t.Fatalf("first grant: %v", err)
	}

	var recipes []model.PillRecipe
	if err := db.Where("is_builtin = ?", true).Find(&recipes).Error; err != nil {
		t.Fatal(err)
	}
	if len(recipes) != 5 {
		t.Fatalf("内置丹方数=%d, want 5", len(recipes))
	}
	for _, r := range recipes {
		recipeUID := r.UUID.String()
		// 每内置丹方恰好 1 条赠送记录
		var grants int64
		if err := db.Model(&model.PillStarterGrant{}).Where("recipe_id = ?", recipeUID).Count(&grants).Error; err != nil {
			t.Fatal(err)
		}
		if grants != 1 {
			t.Fatalf("recipe %s: 赠送记录=%d, want 1", recipeUID, grants)
		}
		// 每内置丹方恰好 1 枚 available 实例（赠送绑定丹方当前版本）
		if r.CurrentRevisionID == nil {
			t.Fatalf("recipe %s: 内置丹方缺少当前版本", recipeUID)
		}
		var items int64
		if err := db.Table("pill_items").
			Where("recipe_revision_id = ? AND state = ?", *r.CurrentRevisionID, model.PillAvailable).
			Count(&items).Error; err != nil {
			t.Fatal(err)
		}
		if items != 1 {
			t.Fatalf("recipe %s: available 赠送实例=%d, want 1", recipeUID, items)
		}
		// 赠送操作 PayloadHash = hex(sha256("starter_grant|" + recipeUUID))
		sum := sha256.Sum256([]byte("starter_grant|" + recipeUID))
		var op model.PillOperation
		if err := db.Where("kind = ? AND payload_hash = ?", "starter_grant", hex.EncodeToString(sum[:])).First(&op).Error; err != nil {
			t.Fatalf("recipe %s: starter_grant 操作未按预期哈希写入: %v", recipeUID, err)
		}
	}

	// 重复启动：不新增 item、不新增 grant（重启不自动补货）
	if err := SeedBuiltinRecipes(db); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if err := GrantStarterPills(db); err != nil {
		t.Fatalf("second grant: %v", err)
	}
	for _, table := range []string{"pill_recipes", "pill_recipe_revisions", "pill_items", "pill_starter_grants", "pill_operations"} {
		assertTableCount(t, db, table, 5)
	}
}

// openInventoryTestDB 统一赠送测试夹具：临时文件 SQLite + 外键 + 单连接
// 按测试所需显式传入模型列表，防止测试偷偷使用用户数据库
func openInventoryTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "inventory.db")), &gorm.Config{})
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
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	return db
}

// inventoryTestModels 金丹库存子系统模型全集（迁移子系统已移除，不含旧迁移表）
func inventoryTestModels() []any {
	return []any{
		&model.PillRecipe{}, &model.PillRecipeRevision{}, &model.PillItem{},
		&model.AgentPillEffect{}, &model.PillOperation{}, &model.FusionPreview{},
		&model.PillStarterGrant{},
	}
}

// assertTableCount 表行数断言
func assertTableCount(t *testing.T, db *gorm.DB, table string, want int64) {
	t.Helper()
	var got int64
	if err := db.Table(table).Count(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s: got %d want %d", table, got, want)
	}
}
