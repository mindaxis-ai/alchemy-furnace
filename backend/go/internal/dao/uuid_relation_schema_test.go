// 011 重构 schema 契约:业务实体同时具有自增内部主键 id 与唯一 uuid 业务键;
// 全部跨表关系必须引用父实体 uuid,禁止以内部 id 作为关系值(specs/011 §2/§3)。
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

// businessEntities 011 spec §2.1:被 API 寻址/被引用/需稳定身份的业务实体表
var businessEntities = []struct {
	name  string
	model any
}{
	{"elixir_pills", &model.ElixirPill{}},
	{"dao_agents", &model.DaoAgent{}},
	{"chat_sessions", &model.ChatSession{}},
	{"chat_messages", &model.ChatMessage{}},
	{"chat_runs", &model.ChatRun{}},
	{"llm_providers", &model.LLMProvider{}},
	{"llm_models", &model.LLMModel{}},
	{"agent_memories", &model.AgentMemory{}},
	{"pill_recipes", &model.PillRecipe{}},
	{"pill_recipe_revisions", &model.PillRecipeRevision{}},
	{"pill_items", &model.PillItem{}},
	{"agent_pill_effects", &model.AgentPillEffect{}},
	{"pill_operations", &model.PillOperation{}},
	{"fusion_previews", &model.FusionPreview{}},
}

// TestBusinessEntitiesKeepInternalIDAndUniqueUUID 每个业务实体表必须同时具备
// id 自增主键列与 uuid 唯一业务键列(索引名 idx_<table>_uuid)。
func TestBusinessEntitiesKeepInternalIDAndUniqueUUID(t *testing.T) {
	db := newSQLiteTestDB(t, filepath.Join(t.TempDir(), "uuid-schema.db"))
	require.NoError(t, db.AutoMigrate(allMigratableModels...))

	for _, ent := range businessEntities {
		t.Run(ent.name, func(t *testing.T) {
			require.True(t, db.Migrator().HasColumn(ent.model, "id"), "%s 缺内部主键列 id", ent.name)
			require.True(t, db.Migrator().HasColumn(ent.model, "uuid"), "%s 缺业务键列 uuid", ent.name)
			require.True(t, db.Migrator().HasIndex(ent.model, "idx_"+ent.name+"_uuid"), "%s 缺 uuid 唯一索引", ent.name)
		})
	}
}

// TestUUIDForeignKeysReferenceBusinessUUID 关系行为契约(FK 强制开启的独立库):
//  1. agent_pills.agent_id/pill_id 必须能写入父实体 uuid(数据库接受 uuid 值并满足 FK);
//  2. 用父实体的内部自增 id 作为关系值必须被 FK 拒绝(内部 id 永不得成为关系值);
//  3. 删除父实体时,以 uuid 关联的 agent_pills 行必须级联删除。
//
// 共享的 newSQLiteTestDB 不带 FK 强制(_fk=1 不被 glebarez/modernc 识别,
// 实测 PRAGMA foreign_keys=0),故此处用 _pragma=foreign_keys(1) 自开库。
func TestUUIDForeignKeysReferenceBusinessUUID(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:"+filepath.Join(t.TempDir(), "uuid-fk.db")+"?_pragma=foreign_keys(1)"),
		&gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(allMigratableModels...))

	agent := &model.DaoAgent{Name: "契约道人"}
	require.NoError(t, db.Create(agent).Error)
	require.NotEqual(t, uuid.Nil, agent.UUID)
	require.NotZero(t, agent.ID)

	pill := &model.ElixirPill{Name: "契约金丹", SkillSchema: model.JSONMap{"identity_card": "x"}}
	require.NoError(t, db.Create(pill).Error)
	require.NotEqual(t, uuid.Nil, pill.UUID)
	require.NotZero(t, pill.ID)

	// 1) 以父实体 uuid 写关系列:必须成功
	require.NoError(t, db.Exec(
		`INSERT INTO agent_pills (agent_id, pill_id) VALUES (?, ?)`,
		agent.UUID.String(), pill.UUID.String(),
	).Error, "agent_pills 必须接受父实体 uuid 作为关系值")

	// 2) 以父实体内部自增 id 写关系列:必须被 FK 拒绝
	err = db.Exec(
		`INSERT INTO agent_pills (agent_id, pill_id) VALUES (?, ?)`,
		agent.ID, pill.ID,
	).Error
	require.Error(t, err, "内部自增 id 不得作为 agent_pills 关系值")
	require.Contains(t, err.Error(), "FOREIGN KEY", "拒绝原因应为外键约束")

	// 3) 删除父实体(按 uuid 定位):uuid 关联行级联删除
	require.NoError(t, db.Where("uuid = ?", agent.UUID).Delete(&model.DaoAgent{}).Error)
	var remain int64
	require.NoError(t, db.Table("agent_pills").Where("agent_id = ?", agent.UUID.String()).Count(&remain).Error)
	require.Zero(t, remain, "删除父实体后 uuid 关联的服用记录应级联删除")
}

// TestRelationColumnsUseUUIDType 关系列数据库类型必须为文本/uuid 语义(SQLite 下
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
	}
	for _, rc := range rels {
		t.Run(rc.column, func(t *testing.T) {
			colType := strings.ToLower(columnDatabaseType(t, db, rc.model, rc.column))
			require.NotContains(t, colType, "int", "%s.%s 是整数列 %q,关系列必须是 UUID", tableNameOf(t, db, rc.model), rc.column, colType)
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
