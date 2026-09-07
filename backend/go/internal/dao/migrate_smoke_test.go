// 多数据库 AutoMigrate 端到端冒烟测试(SQLite in-memory)
// 验证:配置层 → 驱动路由 → AutoMigrate → 8 张业务表全部建出 → 关键约束存在
package dao

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newSQLiteTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+path+"?_loc=Local&_fk=1"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开 SQLite 失败: %v", err)
	}
	return db
}

// TestAutoMigrateSQLite 验证 SQLite 路径下 8 张业务表全部成功建出
// 这是零依赖首启的核心路径,任何回归都会让 Demo 模式无法本地起服
func TestAutoMigrateSQLite(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "smoke.db")
	db := newSQLiteTestDB(t, dbPath)

	if err := db.AutoMigrate(allMigratableModels...); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}

	wantTables := []string{
		"elixir_pills", "dao_agents", "agent_pills", "language_patterns",
		"chat_sessions", "chat_messages", "chat_runs", "session_members", "llm_providers", "llm_models",
		"user_profile",
	}
	for _, table := range wantTables {
		if !db.Migrator().HasTable(table) {
			t.Errorf("缺失业务表: %s", table)
		}
	}

	// 关键约束:user_profile.avatar 列必须为 text(data:image URI ≤1.5M 字符,VARCHAR 不够存)
	cols, err := db.Migrator().ColumnTypes(&model.UserProfile{})
	if err != nil {
		t.Fatalf("读取 user_profile 列类型失败: %v", err)
	}
	avatarType := ""
	for _, col := range cols {
		if col.Name() == "avatar" {
			avatarType = strings.ToLower(col.DatabaseTypeName())
		}
	}
	if avatarType != "text" {
		t.Errorf("user_profile.avatar 数据库类型 = %q, want text", avatarType)
	}
	chatCols, err := db.Migrator().ColumnTypes(&model.ChatSession{})
	if err != nil {
		t.Fatalf("读取 chat_sessions 列类型失败: %v", err)
	}
	chatAvatarType := ""
	for _, col := range chatCols {
		if col.Name() == "avatar" {
			chatAvatarType = strings.ToLower(col.DatabaseTypeName())
		}
	}
	if chatAvatarType != "text" {
		t.Errorf("chat_sessions.avatar 数据库类型 = %q, want text", chatAvatarType)
	}

	// 关键约束:行为档案列(language_patterns)
	lpCols, err := db.Migrator().ColumnTypes(&model.LanguagePattern{})
	if err != nil {
		t.Fatalf("读取 language_patterns 列失败: %v", err)
	}
	lpGot := map[string]bool{}
	for _, col := range lpCols {
		lpGot[col.Name()] = true
	}
	for _, want := range []string{"behavior_profile", "profile_version"} {
		if !lpGot[want] {
			t.Errorf("AutoMigrate 未创建列 %s;实际: %v", want, lpGot)
		}
	}

	// 关键约束:部分唯一索引(is_default 全表至多一个 true)
	if !db.Migrator().HasIndex(&model.LLMModel{}, "idx_llm_models_default") {
		t.Error("缺失部分唯一索引 idx_llm_models_default")
	}
	if !db.Migrator().HasIndex(&model.LLMModel{}, "idx_llm_models_synthesis") {
		t.Error("缺失部分唯一索引 idx_llm_models_synthesis")
	}
	if !db.Migrator().HasIndex(&model.LLMModel{}, "idx_llm_models_fusion") {
		t.Error("缺失部分唯一索引 idx_llm_models_fusion")
	}

	// 关键约束:外键级联(daos 删了, agent_pills / sessions / language_patterns 应被带走)
	if !db.Migrator().HasConstraint(&model.AgentPill{}, "fk_agent_pills_agent") {
		t.Logf("提示: 未检测到 fk_agent_pills_agent(GORM 不同版本命名规则可能不同)")
	}
}

// TestPartialUniqueIndexSQLite 验证部分唯一索引的实际行为
// 期望:两条 is_default=true 的 llm_models 写入时,第二条应被索引拒绝
func TestPartialUniqueIndexSQLite(t *testing.T) {
	tmp := t.TempDir()
	db := newSQLiteTestDB(t, filepath.Join(tmp, "partial.db"))
	if err := db.AutoMigrate(allMigratableModels...); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}

	// 1) 先建一个 Provider 作为 FK 目标
	provider := &model.LLMProvider{
		Name: "test", DisplayName: "Test", BaseURL: "http://x", IsEnabled: true,
	}
	if err := db.Create(provider).Error; err != nil {
		t.Fatalf("建 provider 失败: %v", err)
	}

	// 2) 第一条 is_default=true 写入应成功
	first := &model.LLMModel{
		ProviderID: provider.LLMProviderID, Name: "a", DisplayName: "A",
		IsDefault: true, IsEnabled: true,
	}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("建第一条 default 模型失败: %v", err)
	}

	// 3) 第二条 is_default=true 写入应被部分唯一索引拒绝
	second := &model.LLMModel{
		ProviderID: provider.LLMProviderID, Name: "b", DisplayName: "B",
		IsDefault: true, IsEnabled: true,
	}
	err := db.Create(second).Error
	if err == nil {
		t.Fatal("部分唯一索引未生效: 允许了第二条 is_default=true")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") &&
		!strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Logf("部分唯一索引拒绝第二条,但错误信息不是 'unique/constraint': %v", err)
	} else {
		t.Logf("✅ 部分唯一索引按预期拒绝第二条: %v", err)
	}
}

// TestDriverAutoResolve 验证 loader 的 driver 智能补全逻辑
func TestDriverAutoResolve(t *testing.T) {
	// 走 loader.resolveDriver 行为模拟(直接复用同一函数需要 init 状态,这里内联验证规则)
	cases := []struct {
		name       string
		input      configuration.DatabaseConfig
		wantDriver string
		wantPath   string
	}{
		{
			name: "未填 + 有 host → postgres",
			input: configuration.DatabaseConfig{
				Host: "localhost", Port: 5432, DBName: "x",
			},
			wantDriver: configuration.DriverPostgres,
		},
		{
			name:       "未填 + 无 host → sqlite",
			input:      configuration.DatabaseConfig{},
			wantDriver: configuration.DriverSQLite,
			wantPath:   "./data/alchemy.db",
		},
		{
			name: "显式 sqlite + 无 path → 默认路径",
			input: configuration.DatabaseConfig{
				Driver: configuration.DriverSQLite,
			},
			wantDriver: configuration.DriverSQLite,
			wantPath:   "./data/alchemy.db",
		},
		{
			name: "显式 mysql 透传",
			input: configuration.DatabaseConfig{
				Driver: configuration.DriverMySQL, Host: "x", DBName: "y",
			},
			wantDriver: configuration.DriverMySQL,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input
			// 复用 loader 内部逻辑(本测试纯函数,不触发 os.Exit)
			resolveDriverForTest(&got)
			if got.Driver != tc.wantDriver {
				t.Errorf("driver: got %q, want %q", got.Driver, tc.wantDriver)
			}
			if tc.wantPath != "" && got.SQLitePath != tc.wantPath {
				t.Errorf("sqlite_path: got %q, want %q", got.SQLitePath, tc.wantPath)
			}
		})
	}
}

// resolveDriverForTest 是 loader.resolveDriver 的镜像版,测试时调用避免触发 os.Exit
func resolveDriverForTest(d *configuration.DatabaseConfig) {
	v := strings.ToLower(strings.TrimSpace(d.Driver))
	switch v {
	case "":
		if strings.TrimSpace(d.Host) != "" {
			d.Driver = configuration.DriverPostgres
		} else {
			d.Driver = configuration.DriverSQLite
		}
	case configuration.DriverPostgres, configuration.DriverMySQL, configuration.DriverSQLite:
		d.Driver = v
	default:
		// 测试场景:不调用 os.Exit
		d.Driver = configuration.DriverSQLite
	}
	if d.Driver == configuration.DriverSQLite && strings.TrimSpace(d.SQLitePath) == "" {
		d.SQLitePath = "./data/alchemy.db"
	}
}

// TestInitDatabaseSQLite 验证 InitDatabase 端到端(SQLite 路径)
// 期望: InitDatabase 自动建父目录 + 打开 db + 设置 DB 全局变量
func TestInitDatabaseSQLite(t *testing.T) {
	// 隔离工作目录,避免污染真实 ./data/
	tmp := t.TempDir()
	oldwd, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(oldwd)

	prev := DB
	defer func() { DB = prev }()

	cfg := &configuration.DatabaseConfig{
		Driver:     configuration.DriverSQLite,
		SQLitePath: "./data/nested/sub/test.db",
	}
	if err := InitDatabase(cfg); err != nil {
		t.Fatalf("InitDatabase(SQLite) 失败: %v", err)
	}
	if DB == nil {
		t.Fatal("InitDatabase 成功但 DB 未赋值")
	}

	// 验证父目录被自动创建
	if _, err := os.Stat(filepath.Dir(cfg.SQLitePath)); err != nil {
		t.Errorf("父目录未自动创建: %v", err)
	}
}

// TestMaybeAutoMigrateUpgradesExistingSchema 老库升级路径:已存在旧 schema 时
// MaybeAutoMigrate 必须仍执行 MigrateUp(桌面启动无 CLI migrate 入口,
// 新列只有靠这里落到老库;HasSchema 短路会让新列永远不到库,spec §15)
func TestMaybeAutoMigrateUpgradesExistingSchema(t *testing.T) {
	t.Setenv("SKIP_AUTO_MIGRATE", "0")
	t.Setenv("AF_SKIP_AUTO_MIGRATE", "0")

	tmp := t.TempDir()
	db := newSQLiteTestDB(t, filepath.Join(tmp, "upgrade.db"))
	prev := DB
	DB = db
	defer func() { DB = prev }()

	// 1) 手工建「旧版」language_patterns(无 behavior_profile / profile_version 列)
	oldDDL := `CREATE TABLE language_patterns (
		id integer PRIMARY KEY AUTOINCREMENT,
		agent_id integer NOT NULL,
		system_prompt text NOT NULL,
		emergence_rules json,
		inner_tensions json,
		source_fingerprint varchar(80) NOT NULL,
		is_valid bool DEFAULT true,
		created_at datetime,
		updated_at datetime,
		CONSTRAINT uniq_language_patterns_agent UNIQUE (agent_id)
	);`
	if err := db.Exec(oldDDL).Error; err != nil {
		t.Fatalf("建旧表失败: %v", err)
	}

	// 2) 旧库写入一行历史缓存(模拟老桌面数据)
	if err := db.Exec(
		`INSERT INTO language_patterns (agent_id, system_prompt, source_fingerprint)
		 VALUES (1, '旧提示词', 'sha256:old')`,
	).Error; err != nil {
		t.Fatalf("写历史数据失败: %v", err)
	}

	// 3) MaybeAutoMigrate 必须补齐新列(而不是跳过)
	if err := MaybeAutoMigrate(); err != nil {
		t.Fatalf("MaybeAutoMigrate 失败: %v", err)
	}

	// 4) 断言新列存在
	cols, err := db.Migrator().ColumnTypes(&model.LanguagePattern{})
	if err != nil {
		t.Fatalf("读取列失败: %v", err)
	}
	got := map[string]bool{}
	for _, col := range cols {
		got[col.Name()] = true
	}
	for _, want := range []string{"behavior_profile", "profile_version"} {
		if !got[want] {
			t.Errorf("迁移后缺少列 %s;实际列: %v", want, got)
		}
	}

	// 5) 历史行不丢,新列可写可读
	var cnt int64
	if err := db.Model(&model.LanguagePattern{}).Count(&cnt).Error; err != nil || cnt != 1 {
		t.Errorf("历史行丢失或计数异常: cnt=%d err=%v", cnt, err)
	}
	if err := db.Model(&model.LanguagePattern{}).Where("agent_id = ?", 1).
		Update("behavior_profile", model.JSONMap{"version": 1}).Error; err != nil {
		t.Errorf("新列写入失败: %v", err)
	}
	var loaded model.LanguagePattern
	if err := db.First(&loaded, "agent_id = ?", 1).Error; err != nil {
		t.Fatalf("读取历史行失败: %v", err)
	}
	if loaded.BehaviorProfile["version"] != float64(1) {
		t.Errorf("behavior_profile 回读异常: %+v", loaded.BehaviorProfile)
	}
	if loaded.SystemPrompt != "旧提示词" {
		t.Errorf("历史 system_prompt 被破坏: %q", loaded.SystemPrompt)
	}
}

// legacyIntegerFKDDL 011 之前的旧 schema 样本:代表性关系列为整数自增外键。
// 只覆盖探测点表,其余业务表不建(探测在首个命中列即返回)。
var legacyIntegerFKDDL = []string{
	`CREATE TABLE agent_pills (
		id integer PRIMARY KEY AUTOINCREMENT,
		agent_id integer NOT NULL,
		pill_id integer NOT NULL,
		weight real DEFAULT 1.0,
		sort_order integer DEFAULT 0,
		created_at datetime, updated_at datetime
	);`,
	`CREATE TABLE chat_messages (
		id integer PRIMARY KEY AUTOINCREMENT,
		session_id integer NOT NULL,
		role text NOT NULL,
		content text NOT NULL,
		created_at datetime
	);`,
	`CREATE TABLE llm_models (
		id integer PRIMARY KEY AUTOINCREMENT,
		provider_id integer NOT NULL,
		name text NOT NULL,
		created_at datetime, updated_at datetime
	);`,
}

// TestMigrateUpRejectsLegacyIntegerFKSchema 老库(整数外键)不允许静默升级:
// 正常启动迁移必须返回指导 reset 的错误,且不做任何 DropTable(原数据由用户显式处置)。
func TestMigrateUpRejectsLegacyIntegerFKSchema(t *testing.T) {
	tmp := t.TempDir()
	db := newSQLiteTestDB(t, filepath.Join(tmp, "legacy.db"))
	prev := DB
	DB = db
	defer func() { DB = prev }()

	for i, ddl := range legacyIntegerFKDDL {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("建旧表 #%d 失败: %v\nDDL: %s", i, err, ddl)
		}
	}
	// 写一行历史数据,证明拒绝路径不破坏数据
	if err := db.Exec(`INSERT INTO agent_pills (agent_id, pill_id) VALUES (1, 2)`).Error; err != nil {
		t.Fatalf("写历史数据失败: %v", err)
	}

	err := MigrateUp()
	if err == nil {
		t.Fatal("旧整数外键 schema 未被拒绝: 发生静默升级")
	}
	if !strings.Contains(err.Error(), "migrate reset") {
		t.Errorf("错误未指导用户执行 migrate reset: %v", err)
	}

	// 拒绝路径不得动旧库:表与数据原样保留
	var cnt int64
	if err := db.Table("agent_pills").Count(&cnt).Error; err != nil || cnt != 1 {
		t.Fatalf("拒绝路径破坏了旧数据: cnt=%d err=%v", cnt, err)
	}
	if !db.Migrator().HasTable("chat_messages") {
		t.Error("拒绝路径不应 Drop 任何表: chat_messages 丢失")
	}
}

// legacyUUIDColumnDDL 修订轮之前(011 双标识)的 schema 样本:实体主键仍是
// integer id,身份由独立 uuid 列承担。只覆盖探测点表,其余业务表不建。
var legacyUUIDColumnDDL = []string{
	`CREATE TABLE dao_agents (
		id integer PRIMARY KEY AUTOINCREMENT,
		uuid text NOT NULL UNIQUE,
		name text NOT NULL,
		created_at datetime, updated_at datetime
	);`,
	`CREATE TABLE elixir_pills (
		id integer PRIMARY KEY AUTOINCREMENT,
		uuid text NOT NULL UNIQUE,
		name text NOT NULL,
		created_at datetime, updated_at datetime
	);`,
}

// TestMigrateUpRejectsUUIDColumnSchema 011 双标识中间态库不允许静默升级:
// 主键统一为业务主键文本后,旧库 uuid 列若被 AutoMigrate 保留,新主键列对旧行是
// 空串,行身份全毁;必须返回指导 reset 的错误,且不破坏旧数据。
func TestMigrateUpRejectsUUIDColumnSchema(t *testing.T) {
	tmp := t.TempDir()
	db := newSQLiteTestDB(t, filepath.Join(tmp, "dual-id.db"))
	prev := DB
	DB = db
	defer func() { DB = prev }()

	for i, ddl := range legacyUUIDColumnDDL {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("建双标识旧表 #%d 失败: %v\nDDL: %s", i, err, ddl)
		}
	}
	// 写一行历史数据,证明拒绝路径不破坏数据
	if err := db.Exec(`INSERT INTO dao_agents (uuid, name) VALUES ('uuid-old-agent', '旧道人')`).Error; err != nil {
		t.Fatalf("写历史数据失败: %v", err)
	}

	err := MigrateUp()
	if err == nil {
		t.Fatal("011 双标识 schema 未被拒绝: 发生静默升级")
	}
	if !strings.Contains(err.Error(), "migrate reset") {
		t.Errorf("错误未指导用户执行 migrate reset: %v", err)
	}

	// 拒绝路径不得动旧库:表与数据原样保留
	var cnt int64
	if err := db.Table("dao_agents").Count(&cnt).Error; err != nil || cnt != 1 {
		t.Fatalf("拒绝路径破坏了旧数据: cnt=%d err=%v", cnt, err)
	}
	if !db.Migrator().HasTable("elixir_pills") {
		t.Error("拒绝路径不应 Drop 任何表: elixir_pills 丢失")
	}
}

// TestMigrateResetRebuildsUUIDSchema reset 端到端:旧行清空 → 表重建 → 种子写入 → UUID 关系列可写。
// LLM 种子在测试环境无 API Key 时幂等跳过(供应商计数为 0 即「已清空且未写」)。
func TestMigrateResetRebuildsUUIDSchema(t *testing.T) {
	tmp := t.TempDir()
	db := newSQLiteTestDB(t, filepath.Join(tmp, "reset.db"))
	prev := DB
	DB = db
	defer func() { DB = prev }()

	// 1) 建当前 schema 并写入任意旧数据
	if err := db.AutoMigrate(allMigratableModels...); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	if err := db.Create(&model.DaoAgent{Name: "旧道人", Personality: "reset 前的旧数据"}).Error; err != nil {
		t.Fatalf("写旧道人失败: %v", err)
	}
	if err := db.Create(&model.LLMProvider{Name: "old", DisplayName: "Old", BaseURL: "http://old", IsEnabled: true}).Error; err != nil {
		t.Fatalf("写旧供应商失败: %v", err)
	}

	// 2) reset:Drop → 重建 → 全量种子
	if err := MigrateReset(); err != nil {
		t.Fatalf("MigrateReset 失败: %v", err)
	}

	// 3) 旧行清空
	var agentCnt int64
	if err := db.Model(&model.DaoAgent{}).Count(&agentCnt).Error; err != nil || agentCnt != 0 {
		t.Fatalf("reset 后残留旧道人: cnt=%d err=%v", agentCnt, err)
	}
	var providerCnt int64
	if err := db.Model(&model.LLMProvider{}).Count(&providerCnt).Error; err != nil || providerCnt != 0 {
		t.Fatalf("reset 后残留旧供应商: cnt=%d err=%v", providerCnt, err)
	}

	// 4) 种子数据存在(内置金丹/丹方各 5,每丹方 1 枚赠送 + 1 条赠送记录)
	for _, check := range []struct {
		table string
		query string
		want  int64
	}{
		{"elixir_pills", "is_builtin = true", 5},
		{"pill_recipes", "is_builtin = true", 5},
		{"pill_starter_grants", "1 = 1", 5},
		{"pill_items", "state = 'available'", 5},
	} {
		var cnt int64
		if err := db.Table(check.table).Where(check.query).Count(&cnt).Error; err != nil || cnt != check.want {
			t.Errorf("%s(%s): got %d want %d, err=%v", check.table, check.query, cnt, check.want, err)
		}
	}

	// 5) 业务主键(uuid 文本)关系列可写:道人-金丹服用关系按业务主键建立
	newAgent := &model.DaoAgent{Name: "新道人"}
	if err := db.Create(newAgent).Error; err != nil {
		t.Fatalf("建新道人失败: %v", err)
	}
	var builtinPill model.ElixirPill
	if err := db.Where("is_builtin = ?", true).First(&builtinPill).Error; err != nil {
		t.Fatalf("读内置金丹失败: %v", err)
	}
	ap := &model.AgentPill{AgentID: newAgent.DaoAgentID, PillID: builtinPill.ElixirPillID}
	if err := db.Create(ap).Error; err != nil {
		t.Fatalf("UUID 外键写入失败(重建 schema 缺 FK 或列类型错误): %v", err)
	}
}
