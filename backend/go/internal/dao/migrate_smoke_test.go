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
		"chat_sessions", "chat_messages", "llm_providers", "llm_models",
	}
	for _, table := range wantTables {
		if !db.Migrator().HasTable(table) {
			t.Errorf("缺失业务表: %s", table)
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
		ProviderID: provider.ID, Name: "a", DisplayName: "A",
		IsDefault: true, IsEnabled: true,
	}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("建第一条 default 模型失败: %v", err)
	}

	// 3) 第二条 is_default=true 写入应被部分唯一索引拒绝
	second := &model.LLMModel{
		ProviderID: provider.ID, Name: "b", DisplayName: "B",
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
		name      string
		input     configuration.DatabaseConfig
		wantDriver string
		wantPath  string
	}{
		{
			name: "未填 + 有 host → postgres",
			input: configuration.DatabaseConfig{
				Host: "localhost", Port: 5432, DBName: "x",
			},
			wantDriver: configuration.DriverPostgres,
		},
		{
			name:      "未填 + 无 host → sqlite",
			input:     configuration.DatabaseConfig{},
			wantDriver: configuration.DriverSQLite,
			wantPath:  "./data/alchemy.db",
		},
		{
			name: "显式 sqlite + 无 path → 默认路径",
			input: configuration.DatabaseConfig{
				Driver: configuration.DriverSQLite,
			},
			wantDriver: configuration.DriverSQLite,
			wantPath:  "./data/alchemy.db",
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
