// 修订轮 schema 契约:业务实体主键统一为 <EntityID> text(uuid.UUID.String()),
// 无内部自增 id、无独立 uuid 列;全部跨表关系为 text 列引用父实体业务主键
// (specs/011 修订轮裁决:本质上是每个实体的唯一标识都是 uuid.UUID.String())。
package dao

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// businessEntities 011 spec §2.1:被 API 寻址/被引用/需稳定身份的业务实体表,
// 以及各自统一的业务主键列名(GORM NamingStrategy snake_case)。
var businessEntities = []struct {
	name  string
	pk    string
	model any
}{
	{"elixir_pills", "elixir_pill_id", &model.ElixirPill{}},
	{"dao_agents", "dao_agent_id", &model.DaoAgent{}},
	{"chat_sessions", "chat_session_id", &model.ChatSession{}},
	{"chat_messages", "chat_message_id", &model.ChatMessage{}},
	{"chat_runs", "chat_run_id", &model.ChatRun{}},
	{"llm_providers", "llm_provider_id", &model.LLMProvider{}},
	{"llm_models", "llm_model_id", &model.LLMModel{}},
	{"agent_memories", "agent_memory_id", &model.AgentMemory{}},
	{"pill_recipes", "pill_recipe_id", &model.PillRecipe{}},
	{"pill_recipe_revisions", "pill_recipe_revision_id", &model.PillRecipeRevision{}},
	{"pill_items", "pill_item_id", &model.PillItem{}},
	{"agent_pill_effects", "agent_pill_effect_id", &model.AgentPillEffect{}},
	{"pill_operations", "pill_operation_id", &model.PillOperation{}},
	{"fusion_previews", "fusion_preview_id", &model.FusionPreview{}},
}

// TestBusinessEntitiesUseUUIDTextPrimaryKey 每个业务实体表必须以 <entity>_id
// text 为唯一主键列;内部自增 id 与独立 uuid 列必须已删除。
func TestBusinessEntitiesUseUUIDTextPrimaryKey(t *testing.T) {
	db := newSQLiteTestDB(t, filepath.Join(t.TempDir(), "uuid-schema.db"))
	require.NoError(t, db.AutoMigrate(allMigratableModels...))

	for _, ent := range businessEntities {
		t.Run(ent.name, func(t *testing.T) {
			require.True(t, db.Migrator().HasColumn(ent.model, ent.pk), "%s 缺业务主键列 %s", ent.name, ent.pk)
			require.False(t, db.Migrator().HasColumn(ent.model, "id"), "%s 不应再有内部自增主键列 id", ent.name)
			require.False(t, db.Migrator().HasColumn(ent.model, "uuid"), "%s 不应再有独立 uuid 列", ent.name)
		})
	}
}

// TestForeignKeysReferenceBusinessKey 关系行为契约(FK 强制开启的独立库):
//  1. agent_pills.agent_id/pill_id 必须能写入父实体业务主键(uuid 文本);
//  2. 不存在的 uuid 文本作为关系值必须被 FK 拒绝(关系值必须真实指向父实体);
//  3. 删除父实体时,以业务主键关联的 agent_pills 行必须级联删除。
//
// 共享的 newSQLiteTestDB 不带 FK 强制(_fk=1 不被 glebarez/modernc 识别,
// 实测 PRAGMA foreign_keys=0),故此处用 _pragma=foreign_keys(1) 自开库。
func TestForeignKeysReferenceBusinessKey(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:"+filepath.Join(t.TempDir(), "uuid-fk.db")+"?_pragma=foreign_keys(1)"),
		&gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(allMigratableModels...))

	agent := &model.DaoAgent{Name: "契约道人"}
	require.NoError(t, db.Create(agent).Error)
	require.NotEmpty(t, agent.DaoAgentID, "BeforeCreate 应生成 uuid 文本业务主键")

	pill := &model.ElixirPill{Name: "契约金丹", SkillSchema: model.JSONMap{"identity_card": "x"}}
	require.NoError(t, db.Create(pill).Error)
	require.NotEmpty(t, pill.ElixirPillID, "BeforeCreate 应生成 uuid 文本业务主键")

	// 1) 以父实体业务主键写关系列:必须成功
	require.NoError(t, db.Exec(
		`INSERT INTO agent_pills (agent_id, pill_id) VALUES (?, ?)`,
		agent.DaoAgentID, pill.ElixirPillID,
	).Error, "agent_pills 必须接受父实体业务主键作为关系值")

	// 2) 以不存在的 uuid 文本写关系列:必须被 FK 拒绝
	err = db.Exec(
		`INSERT INTO agent_pills (agent_id, pill_id) VALUES (?, ?)`,
		uuid.New().String(), uuid.New().String(),
	).Error
	require.Error(t, err, "不存在的 uuid 文本不得作为 agent_pills 关系值")
	require.Contains(t, err.Error(), "FOREIGN KEY", "拒绝原因应为外键约束")

	// 3) 删除父实体(按业务主键定位):关联行级联删除
	require.NoError(t, db.Where("dao_agent_id = ?", agent.DaoAgentID).Delete(&model.DaoAgent{}).Error)
	var remain int64
	require.NoError(t, db.Table("agent_pills").Where("agent_id = ?", agent.DaoAgentID).Count(&remain).Error)
	require.Zero(t, remain, "删除父实体后以业务主键关联的服用记录应级联删除")
}

// TestRelationColumnsUseUUIDType 关系列数据库类型必须为文本语义(SQLite 下
// 存放 uuid 字符串),不得是 integer:覆盖 agent_pills/language_patterns/chat_messages/
// chat_runs/session_members/llm_models/agent_memories 的代表关系列。
func TestRelationColumnsUseUUIDType(t *testing.T) {
	db := newSQLiteTestDB(t, filepath.Join(t.TempDir(), "uuid-cols.db"))
	require.NoError(t, db.AutoMigrate(allMigratableModels...))

	type relCol struct {
		model  any
		column string
	}
	rels := []relCol{
		{&model.AgentPill{}, "agent_id"},
		{&model.AgentPill{}, "pill_id"},
		{&model.LanguagePattern{}, "agent_id"},
		{&model.ChatSession{}, "agent_id"},
		{&model.ChatMessage{}, "session_id"},
		{&model.ChatMessage{}, "agent_id"},
		{&model.ChatRun{}, "session_id"},
		{&model.ChatRun{}, "user_message_id"},
		{&model.SessionMember{}, "session_id"},
		{&model.SessionMember{}, "agent_id"},
		{&model.LLMModel{}, "provider_id"},
		{&model.AgentMemory{}, "agent_id"},
		// 011 库存域关系列（Task 3 翻转范围）
		{&model.PillRecipe{}, "current_revision_id"},
		{&model.PillRecipeRevision{}, "recipe_id"},
		{&model.PillItem{}, "recipe_revision_id"},
		{&model.PillItem{}, "consume_operation_id"},
		{&model.PillItem{}, "origin_operation_id"},
		{&model.AgentPillEffect{}, "item_id"},
		{&model.AgentPillEffect{}, "recipe_revision_id"},
		{&model.FusionPreview{}, "confirmed_operation_id"},
		{&model.PillStarterGrant{}, "recipe_id"},
		{&model.PillStarterGrant{}, "item_id"},
	}
	for _, rc := range rels {
		t.Run(rc.column, func(t *testing.T) {
			colType := strings.ToLower(columnDatabaseType(t, db, rc.model, rc.column))
			require.NotContains(t, colType, "int", "%s.%s 是整数列 %q,关系列必须是 uuid 文本", tableNameOf(t, db, rc.model), rc.column, colType)
		})
	}
}

// columnDatabaseType 读取指定列在 SQLite 中声明的数据库类型(建表时 SQL 类型原样保留)
func columnDatabaseType(t *testing.T, db *gorm.DB, m any, column string) string {
	t.Helper()
	cols, err := db.Migrator().ColumnTypes(m)
	require.NoError(t, err)
	for _, c := range cols {
		if c.Name() == column {
			return c.DatabaseTypeName()
		}
	}
	t.Fatalf("列 %s 不存在", column)
	return ""
}

// tableNameOf 解析 GORM 表名,仅用于失败信息
func tableNameOf(t *testing.T, db *gorm.DB, m any) string {
	t.Helper()
	stmt := &gorm.Statement{DB: db}
	require.NoError(t, stmt.Parse(m))
	return stmt.Table
}
