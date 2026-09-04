// 丹方/库存迁移测试：旧 ElixirPill 定义 → PillRecipe/PillRecipeRevision/PillItem/AgentPillEffect
// 迁移必须：先一致性备份 → 预检（缺列/孤儿绑定/重复绑定）→ 事务回填 → 数量断言 → 完成标记
// 失败不得静默丢弃记录或标记成功；旧表保留用于受控回滚
package dao

import (
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// openInventoryTestDB 统一迁移测试夹具：临时文件 SQLite + 外键 + 单连接
// 按各测试所需显式传入模型列表，防止测试偷偷使用用户数据库
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

// legacyInventoryModels 迁移输入用的旧模型
func legacyInventoryModels() []any {
	return []any{&model.ElixirPill{}, &model.DaoAgent{}, &model.AgentPill{}}
}

// TestMigratePillInventoryPreservesConsumption 核心断言：3 个旧定义（0/1/2 绑定）
// 迁移后必须得到 3 丹方 + 3 版本 + 4 实例（1 可用 + 3 已服用）+ 3 能力
func TestMigratePillInventoryPreservesConsumption(t *testing.T) {
	db := openInventoryTestDB(t, append(legacyInventoryModels(), pillInventoryModels()...)...)

	agents := []model.DaoAgent{
		{Name: "道人甲"}, {Name: "道人乙"},
	}
	if err := db.Create(&agents).Error; err != nil {
		t.Fatal(err)
	}
	pills := []model.ElixirPill{
		{
			Name: "无绑定丹", Description: "无绑定描述",
			SkillSchema: model.JSONMap{
				"identity_card":  "A",
				"future_unknown": map[string]any{"keep": true, "nested": []any{1, "x"}},
			},
			Tags: model.JSONList{"甲"}, Author: "测试", Version: "1.0.0",
		},
		{Name: "单绑丹", Description: "单绑描述", SkillSchema: model.JSONMap{"identity_card": "B"}, Tags: model.JSONList{"乙"}, Author: "测试", Version: "1.2.0"},
		{Name: "双绑丹", Description: "双绑描述", SkillSchema: model.JSONMap{"identity_card": "C"}, Tags: model.JSONList{"丙"}, Author: "测试", Version: "2.0.0"},
	}
	if err := db.Create(&pills).Error; err != nil {
		t.Fatal(err)
	}
	// 单绑丹 → 道人甲；双绑丹 → 道人甲 + 道人乙（两个道人绑定同一金丹）
	binds := []model.AgentPill{
		{AgentID: agents[0].UUID.String(), PillID: pills[1].UUID.String(), Weight: 0.8, SortOrder: 0},
		{AgentID: agents[0].UUID.String(), PillID: pills[2].UUID.String(), Weight: 1.5, SortOrder: 1},
		{AgentID: agents[1].UUID.String(), PillID: pills[2].UUID.String(), Weight: 0.5, SortOrder: 2},
	}
	if err := db.Create(&binds).Error; err != nil {
		t.Fatal(err)
	}

	if err := MigratePillInventory(db); err != nil {
		t.Fatalf("MigratePillInventory: %v", err)
	}

	for table, want := range map[string]int64{
		"pill_recipes": 3, "pill_recipe_revisions": 3,
		"pill_items": 4, "agent_pill_effects": 3,
	} {
		var got int64
		if err := db.Table(table).Count(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s: got %d want %d", table, got, want)
		}
	}
	var available int64
	if err := db.Table("pill_items").Where("state = ?", "available").Count(&available).Error; err != nil {
		t.Fatal(err)
	}
	if available != 1 {
		t.Fatalf("available=%d, want 1", available)
	}

	// 未绑定丹：1 枚可用实例，来源为迁移操作，origin_index=0
	var item model.PillItem
	if err := db.Where("recipe_revision_id IN (SELECT id FROM pill_recipe_revisions WHERE name = ?)", "无绑定丹").First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.State != model.PillAvailable {
		t.Fatalf("无绑定丹实例 state=%q, want available", item.State)
	}
	if item.OriginOperationID == 0 || item.OriginIndex != 0 {
		t.Fatalf("无绑定丹实例来源异常: op=%d index=%d", item.OriginOperationID, item.OriginIndex)
	}

	// 已绑定：实例为 consumed_by_agent，消耗时间/来源保留；能力快照保留权重、顺序、名称与完整内容
	var eff model.AgentPillEffect
	if err := db.Where("agent_id = ?", agents[0].UUID.String()).Order("sort_order").First(&eff).Error; err != nil {
		t.Fatal(err)
	}
	if eff.NameSnapshot != "单绑丹" || eff.Weight != 0.8 || eff.SortOrder != 0 {
		t.Fatalf("能力快照异常: name=%q weight=%v order=%d", eff.NameSnapshot, eff.Weight, eff.SortOrder)
	}
	if !reflect.DeepEqual(eff.SchemaSnapshot, pills[1].SkillSchema) {
		t.Fatalf("能力内容被改写: got=%+v want=%+v", eff.SchemaSnapshot, pills[1].SkillSchema)
	}
	var consumed []model.PillItem
	if err := db.Where("recipe_revision_id = (SELECT id FROM pill_recipe_revisions WHERE name = ?)", "双绑丹").
		Order("origin_index").Find(&consumed).Error; err != nil {
		t.Fatal(err)
	}
	if len(consumed) != 2 {
		t.Fatalf("双绑丹实例数=%d, want 2", len(consumed))
	}
	for i, it := range consumed {
		if it.State != model.PillConsumedByAgent || it.OriginIndex != i {
			t.Fatalf("双绑丹实例[%d] state=%q origin_index=%d", i, it.State, it.OriginIndex)
		}
		if it.ConsumedAt == nil {
			t.Fatalf("双绑丹实例[%d] 缺少消耗时间", i)
		}
	}
	var cEffs []model.AgentPillEffect
	if err := db.Where("recipe_revision_id = (SELECT id FROM pill_recipe_revisions WHERE name = ?)", "双绑丹").
		Order("sort_order").Find(&cEffs).Error; err != nil {
		t.Fatal(err)
	}
	if len(cEffs) != 2 || cEffs[0].AgentID != agents[0].UUID.String() || cEffs[1].AgentID != agents[1].UUID.String() {
		t.Fatalf("双绑丹能力异常: %+v", cEffs)
	}

	// LegacyMap：3 条 pill 映射 + 3 条 bind 映射
	var pillMaps, bindMaps int64
	if err := db.Model(&model.PillLegacyMap{}).Where("legacy_kind = ?", "pill").Count(&pillMaps).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.PillLegacyMap{}).Where("legacy_kind = ?", "bind").Count(&bindMaps).Error; err != nil {
		t.Fatal(err)
	}
	if pillMaps != 3 || bindMaps != 3 {
		t.Fatalf("legacy map: pill=%d bind=%d, want 3/3", pillMaps, bindMaps)
	}

	// 迁移报告：计数齐备、含备份路径、不记录 schema 全文
	var st model.PillMigrationState
	if err := db.Where("key = ?", PillInventoryMigrationKey).First(&st).Error; err != nil {
		t.Fatalf("缺少迁移完成标记: %v", err)
	}
	rep := st.ReportJSON
	for _, field := range []string{"legacy_pills", "legacy_binds", "recipes", "available_items", "history_items", "effects", "backup_path"} {
		if _, ok := rep[field]; !ok {
			t.Errorf("迁移报告缺少字段 %s: %+v", field, rep)
		}
	}
	if raw, err := json.Marshal(st.ReportJSON); err != nil {
		t.Fatalf("报告不可序列化: %v", err)
	} else if strings.Contains(string(raw), "identity_card") {
		t.Error("迁移报告不得包含 schema 全文")
	}
}

// TestMigratePillInventoryEmptyDatabase 空库（全新安装）：不迁移任何旧数据，
// 完成标记标记为 fresh，后续启动不再重复判定
func TestMigratePillInventoryEmptyDatabase(t *testing.T) {
	db := openInventoryTestDB(t, pillInventoryModels()...)

	if err := MigratePillInventory(db); err != nil {
		t.Fatalf("空库迁移: %v", err)
	}

	var st model.PillMigrationState
	if err := db.Where("key = ?", PillInventoryMigrationKey).First(&st).Error; err != nil {
		t.Fatalf("空库未写完成标记: %v", err)
	}
	if st.ReportJSON["is_fresh_install"] != true {
		t.Fatalf("空库报告应标记 fresh, got %+v", st.ReportJSON)
	}
	for _, table := range []string{"pill_recipes", "pill_items", "agent_pill_effects"} {
		var got int64
		if err := db.Table(table).Count(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != 0 {
			t.Fatalf("空库 %s 不应有数据, got %d", table, got)
		}
	}
}

// TestMigratePillInventoryIdempotent 第二次启动迁移不新增任何数据（防重启复活）
func TestMigratePillInventoryIdempotent(t *testing.T) {
	db := openInventoryTestDB(t, append(legacyInventoryModels(), pillInventoryModels()...)...)
	pill := model.ElixirPill{Name: "幂等丹", Description: "d", SkillSchema: model.JSONMap{"identity_card": "X"}, Version: "1.0.0"}
	if err := db.Create(&pill).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigratePillInventory(db); err != nil {
		t.Fatal(err)
	}

	before := map[string]int64{}
	for _, table := range []string{"pill_recipes", "pill_recipe_revisions", "pill_items", "agent_pill_effects", "pill_legacy_maps", "pill_operations"} {
		var got int64
		if err := db.Table(table).Count(&got).Error; err != nil {
			t.Fatal(err)
		}
		before[table] = got
	}

	// 第二次迁移：完成标记已存在，直接跳过
	if err := MigratePillInventory(db); err != nil {
		t.Fatal(err)
	}
	for table, want := range before {
		var got int64
		if err := db.Table(table).Count(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("第二次迁移 %s: got %d want %d", table, got, want)
		}
	}
}

// TestMigratePillInventoryKeepsLegacyTables 旧表保留用于受控回滚，行数不变
func TestMigratePillInventoryKeepsLegacyTables(t *testing.T) {
	db := openInventoryTestDB(t, append(legacyInventoryModels(), pillInventoryModels()...)...)
	agent := model.DaoAgent{Name: "甲"}
	pill := model.ElixirPill{Name: "旧丹", Description: "d", SkillSchema: model.JSONMap{"identity_card": "X"}}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&pill).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AgentPill{AgentID: agent.UUID.String(), PillID: pill.UUID.String()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigratePillInventory(db); err != nil {
		t.Fatal(err)
	}

	if !db.Migrator().HasTable("elixir_pills") || !db.Migrator().HasTable("agent_pills") {
		t.Fatal("迁移后旧表被删除，无法受控回滚")
	}
	var pills, binds int64
	if err := db.Table("elixir_pills").Count(&pills).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("agent_pills").Count(&binds).Error; err != nil {
		t.Fatal(err)
	}
	if pills != 1 || binds != 1 {
		t.Fatalf("旧表数据被改动: pills=%d binds=%d", pills, binds)
	}
}

// TestMigratePillInventoryRejectsDuplicateBinding 同旧定义与道人重复绑定是异常数据：
// 迁移前报告并阻止切换，不静默丢弃
func TestMigratePillInventoryRejectsDuplicateBinding(t *testing.T) {
	db := openInventoryTestDB(t, append(legacyInventoryModels(), pillInventoryModels()...)...)
	// 历史版本 agent_pills 无 (agent_id, pill_id) 唯一约束：删掉 AutoMigrate 建的表，手工重建
	if err := db.Migrator().DropTable("agent_pills"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE agent_pills (
		id integer PRIMARY KEY AUTOINCREMENT,
		agent_id varchar(36) NOT NULL,
		pill_id varchar(36) NOT NULL,
		weight real DEFAULT 1.0,
		sort_order integer DEFAULT 0,
		created_at datetime
	)`).Error; err != nil {
		t.Fatal(err)
	}
	agent := model.DaoAgent{Name: "重复道人"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	pill := model.ElixirPill{Name: "重复丹", Description: "d", SkillSchema: model.JSONMap{"identity_card": "X"}}
	if err := db.Create(&pill).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO agent_pills (agent_id, pill_id, weight) VALUES (?, ?, 1.0), (?, ?, 2.0)`,
		agent.UUID.String(), pill.UUID.String(), agent.UUID.String(), pill.UUID.String()).Error; err != nil {
		t.Fatal(err)
	}

	err := MigratePillInventory(db)
	if err == nil {
		t.Fatal("重复绑定应被预检拒绝")
	}
	if !strings.Contains(err.Error(), "agent_pills") {
		t.Fatalf("错误应指明异常表: %v", err)
	}
	assertMigrationAborted(t, db)
}

// TestMigratePillInventoryRejectsOrphanBinding 绑定指向不存在的旧金丹：预检报告并阻止
func TestMigratePillInventoryRejectsOrphanBinding(t *testing.T) {
	db := openInventoryTestDB(t, append(legacyInventoryModels(), pillInventoryModels()...)...)
	agent := model.DaoAgent{Name: "甲"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable("agent_pills"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE agent_pills (
		id integer PRIMARY KEY AUTOINCREMENT,
		agent_id varchar(36) NOT NULL,
		pill_id varchar(36) NOT NULL,
		weight real DEFAULT 1.0,
		sort_order integer DEFAULT 0,
		created_at datetime
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO agent_pills (agent_id, pill_id) VALUES (?, ?)`,
		agent.UUID.String(), uuid.NewString()).Error; err != nil {
		t.Fatal(err)
	}

	err := MigratePillInventory(db)
	if err == nil {
		t.Fatal("孤儿绑定应被预检拒绝")
	}
	assertMigrationAborted(t, db)
}

// TestMigratePillInventoryRejectsMissingColumns 旧表缺列（异常 schema）：预检拒绝
func TestMigratePillInventoryRejectsMissingColumns(t *testing.T) {
	db := openInventoryTestDB(t, pillInventoryModels()...)
	if err := db.Exec(`CREATE TABLE elixir_pills (
		id integer PRIMARY KEY AUTOINCREMENT,
		name varchar(100) NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO elixir_pills (name) VALUES ('残缺丹')`).Error; err != nil {
		t.Fatal(err)
	}

	err := MigratePillInventory(db)
	if err == nil {
		t.Fatal("缺列旧表应被预检拒绝")
	}
	if !strings.Contains(err.Error(), "缺少列") || !strings.Contains(err.Error(), "elixir_pills") {
		t.Fatalf("错误应指明缺失列与异常表: %v", err)
	}
	assertMigrationAborted(t, db)
}

// TestMigratePillInventoryBacksUpBeforeUpgrade 升级前必须留一致性备份；
// 备份失败不得继续迁移
func TestMigratePillInventoryBacksUpBeforeUpgrade(t *testing.T) {
	// 夹具只建旧表：验证备份发生在建新表之前（真实启动顺序：备份 → AutoMigrate）
	db := openInventoryTestDB(t, legacyInventoryModels()...)
	pill := model.ElixirPill{Name: "备份丹", Description: "d", SkillSchema: model.JSONMap{"identity_card": "X"}}
	if err := db.Create(&pill).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigratePillInventory(db); err != nil {
		t.Fatal(err)
	}

	var st model.PillMigrationState
	if err := db.Where("key = ?", PillInventoryMigrationKey).First(&st).Error; err != nil {
		t.Fatal(err)
	}
	backupPath, _ := st.ReportJSON["backup_path"].(string)
	if backupPath == "" {
		t.Fatal("迁移报告缺少 backup_path")
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("备份文件不存在 %s: %v", backupPath, err)
	}
	// 备份必须可打开且一致性完好
	backupDB, err := gorm.Open(sqlite.Open(backupPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("备份文件打不开: %v", err)
	}
	var integrity string
	if err := backupDB.Raw("PRAGMA integrity_check").Row().Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("备份 integrity_check = %q, want ok", integrity)
	}
	// 备份是迁移前的旧世界：有旧表无新表
	if !backupDB.Migrator().HasTable("elixir_pills") {
		t.Fatal("备份应包含旧表")
	}
	if backupDB.Migrator().HasTable("pill_recipes") {
		t.Fatal("备份不应包含迁移后的新表")
	}
}

// assertMigrationAborted 迁移被预检拒绝后：无完成标记、新表无任何写入
func assertMigrationAborted(t *testing.T, db *gorm.DB) {
	t.Helper()
	var states int64
	if err := db.Model(&model.PillMigrationState{}).Count(&states).Error; err != nil {
		t.Fatal(err)
	}
	if states != 0 {
		t.Fatalf("预检失败仍写了完成标记: %d 条", states)
	}
	for _, table := range []string{"pill_recipes", "pill_recipe_revisions", "pill_items", "agent_pill_effects"} {
		var got int64
		if err := db.Table(table).Count(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != 0 {
			t.Fatalf("预检失败仍写入了 %s: %d 行", table, got)
		}
	}
}
