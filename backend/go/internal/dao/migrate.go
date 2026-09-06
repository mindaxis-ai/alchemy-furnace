// GORM AutoMigrate 驱动: 多数据库统一 schema 同步
// 替代历史 golang-migrate SQL 方案: 由 model/*.go 的 GORM 标签作为唯一事实来源
//   - 幂等: 重复执行只补齐新增列/索引,不会破坏已有数据
//   - 驱动无关: 同一组模型在 postgres / mysql / sqlite 上生成等价表结构
//   - 部分唯一索引(is_default / is_synthesis / is_fusion):
//     模型 tag 上声明 where 子句,PG/SQLite 自动生成 partial index;
//     MySQL 8.0.13+ 同步支持,更早版本降级为普通 unique 索引并由 service 层兜底
package dao

import (
	"fmt"
	"os"
	"strings"

	"github.com/alchemy-furnace/server/model"
	"gorm.io/gorm"
)

// allMigratableModels 全部需要 AutoMigrate 的模型(顺序无关:GORM 解析外键延迟建表)
// 新增模型时在此追加
var allMigratableModels = []any{
	&model.ElixirPill{},
	&model.DaoAgent{},
	&model.AgentPill{},
	&model.LanguagePattern{},
	&model.ChatSession{},
	&model.ChatMessage{},
	&model.ChatRun{},
	&model.SessionMember{},
	&model.LLMProvider{},
	&model.LLMModel{},
	&model.UserProfile{},
	&model.AgentMemory{},
	// 金丹消耗品重构（2026-08-31）：丹方/库存/能力快照/幂等操作/迁移状态
	&model.PillRecipe{},
	&model.PillRecipeRevision{},
	&model.PillItem{},
	&model.AgentPillEffect{},
	&model.PillOperation{},
	&model.FusionPreview{},
	&model.PillStarterGrant{},
}

// nullableAlterations 新老 schema 漂移:GORM AutoMigrate 不会把已存在的 NOT NULL 列改为可空
// 这里显式 ALTER;运行前会查 information_schema 跳过已是可空的列(幂等)
var nullableAlterations = []struct {
	Table  string
	Column string
}{
	// 群聊场景:会话和消息的 AgentID 由单聊时的必填改为可空(指针化)
	{"chat_sessions", "agent_id"},
	{"chat_messages", "agent_id"},
}

// columnTypeAlterations 新老 schema 漂移:列类型变更(VARCHAR → TEXT 等)
// GORM AutoMigrate 在「表已存在」时对列类型加宽各驱动行为不一,且启动路径
// 只在桌面/自部署启动时跑一次(幂等);这里对 PostgreSQL 显式 ALTER,
// 运行前查 information_schema 已是目标类型则跳过(幂等);SQLite/MySQL 由 AutoMigrate 负责
var columnTypeAlterations = []struct {
	Table   string
	Column  string
	NewType string
}{
	// 头像契约:允许 data:image 数据 URI(≤1.5M 字符),VARCHAR(255) 不够存
	{"dao_agents", "avatar", "text"},
	// 用户头像契约:与道人一致,允许 data:image 数据 URI
	{"user_profile", "avatar", "text"},
}

// legacyFKProbes 旧(011 之前)schema 探测点:代表性跨表关系列。
// 011 前关系列是整数自增 ID,011 起一律为 uuid 文本;AutoMigrate 无法把已有整数
// 关系列安全转换为 uuid(存量数据不可映射),必须拒绝静默升级并引导显式 reset。
var legacyFKProbes = []struct {
	Model  any    // 探测表对应的模型(用于 HasTable/ColumnTypes)
	Column string // 代表性关系列名
}{
	{&model.AgentPill{}, "agent_id"},
	{&model.ChatMessage{}, "session_id"},
	{&model.LLMModel{}, "provider_id"},
}

// legacyUUIDColumnProbes 修订轮之前(011 双标识)schema 探测点:已废弃的 uuid 列。
// 修订轮把实体主键统一为 <EntityID> text 单主键并删除 UUID 列;存量库若带着旧 uuid 列
// 被 AutoMigrate 升级,新主键列对旧行是空串,行身份全毁,必须拒绝静默升级并引导 reset。
var legacyUUIDColumnProbes = []struct {
	Model  any    // 探测表对应的模型(用于 HasTable/HasColumn)
	Column string // 已废弃的 uuid 列名
	Table  string // 表名(用于错误信息,显式写死避免反射出 *model.Xxx)
}{
	{&model.DaoAgent{}, "uuid", "dao_agents"},
	{&model.ElixirPill{}, "uuid", "elixir_pills"},
}

// detectLegacyIntegerFK 检查探测点关系列的数据库类型;任一为整数类型 → 返回旧 schema 错误。
// 另检查废弃 uuid 列是否存在(011 双标识中间态库) → 同样拒绝。
// 全新库(表不存在)或已是 text(新 schema) → nil。只读探测,不做任何写入。
func detectLegacyIntegerFK(db *gorm.DB) error {
	for _, probe := range legacyFKProbes {
		if !db.Migrator().HasTable(probe.Model) {
			continue // 全新库:无表即无旧 schema
		}
		cols, err := db.Migrator().ColumnTypes(probe.Model)
		if err != nil {
			return fmt.Errorf("读取 %s.%s 列类型失败: %w", db.NamingStrategy.TableName(fmt.Sprintf("%T", probe.Model)), probe.Column, err)
		}
		for _, col := range cols {
			if col.Name() != probe.Column {
				continue
			}
			dbType := strings.ToLower(strings.TrimSpace(col.DatabaseTypeName()))
			if strings.Contains(dbType, "int") || strings.Contains(dbType, "serial") {
				return fmt.Errorf(
					"检测到旧版数据库 schema(关系列为整数外键,如 %s),无法自动升级为 UUID 业务键 schema;"+
						"请运行 `migrate reset` 重建数据库(全部业务数据将被清空并重新种子),或删除数据目录后重新初始化",
					probe.Column)
			}
		}
	}
	// 011 双标识中间态库:废弃 uuid 列仍存在 → 拒绝(主键统一后旧行无法安全映射)
	for _, probe := range legacyUUIDColumnProbes {
		if !db.Migrator().HasTable(probe.Model) {
			continue
		}
		if db.Migrator().HasColumn(probe.Model, probe.Column) {
			return fmt.Errorf(
				"检测到修订前数据库 schema(%s 仍带 uuid 独立列),实体主键已统一为业务主键文本,无法自动升级;"+
					"请运行 `migrate reset` 重建数据库(全部业务数据将被清空并重新种子),或删除数据目录后重新初始化",
				probe.Table)
		}
	}
	return nil
}

// MigrateUp 同步全部业务表到当前模型定义(幂等,跨驱动)
// 历史 raw-SQL 迁移文件已不再依赖;若是从旧部署首次切换,可重复运行直至无差异
func MigrateUp() error {
	if DB == nil {
		return fmt.Errorf("数据库未初始化")
	}
	// 旧 schema 守门:整数外键的老库拒绝静默升级(数据不可映射),先于任何写入
	if err := detectLegacyIntegerFK(DB); err != nil {
		return err
	}
	if err := DB.AutoMigrate(allMigratableModels...); err != nil {
		return fmt.Errorf("AutoMigrate 失败: %w", err)
	}
	// 手动 ALTER 列约束:GORM 无法回溯调整已建列的可空性
	for _, alt := range nullableAlterations {
		if err := alterColumnToNullable(DB, alt.Table, alt.Column); err != nil {
			return fmt.Errorf("ALTER %s.%s 失败: %w", alt.Table, alt.Column, err)
		}
	}
	// 手动 ALTER 列类型:老库 VARCHAR → TEXT 等加宽(幂等)
	for _, alt := range columnTypeAlterations {
		if err := alterColumnType(DB, alt.Table, alt.Column, alt.NewType); err != nil {
			return fmt.Errorf("ALTER %s.%s 失败: %w", alt.Table, alt.Column, err)
		}
	}
	return nil
}

// alterColumnToNullable 将指定列改为可空(已是可空则跳过);驱动差异:
//   - PostgreSQL:走 information_schema + ALTER COLUMN DROP NOT NULL
//   - SQLite/MySQL:GORM AutoMigrate 已能直接调整,这里 no-op
func alterColumnToNullable(db *gorm.DB, table, column string) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	var notNull bool
	row := db.Raw(`
		SELECT is_nullable = 'NO'
		FROM information_schema.columns
		WHERE table_schema = CURRENT_SCHEMA()
		  AND table_name = ? AND column_name = ?
	`, table, column).Row()
	if err := row.Scan(&notNull); err != nil {
		return fmt.Errorf("查询可空性失败: %w", err)
	}
	if !notNull {
		return nil
	}
	stmt := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", table, column)
	return db.Exec(stmt).Error
}

// alterColumnType 将指定列改为目标类型(已是目标类型则跳过);驱动差异:
//   - PostgreSQL:走 information_schema + ALTER COLUMN TYPE
//   - SQLite/MySQL:AutoMigrate 已能处理列类型变更,这里 no-op
func alterColumnType(db *gorm.DB, table, column, newType string) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	var dataType string
	row := db.Raw(`
		SELECT data_type
		FROM information_schema.columns
		WHERE table_schema = CURRENT_SCHEMA()
		  AND table_name = ? AND column_name = ?
	`, table, column).Row()
	if err := row.Scan(&dataType); err != nil {
		return fmt.Errorf("查询列类型失败: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(dataType), newType) {
		return nil
	}
	stmt := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s", table, column, newType)
	return db.Exec(stmt).Error
}

// MigrateDown 丢弃全部业务表(等同 drop-all;用于本地 / 演示环境重建)
// SQLite 单文件部署慎用: 直接删 db 文件更快,这里走 GORM Migrator.DropTable 走全流程
func MigrateDown() error {
	if DB == nil {
		return fmt.Errorf("数据库未初始化")
	}
	if err := DB.Migrator().DropTable(allMigratableModels...); err != nil {
		return fmt.Errorf("DropTable 失败: %w", err)
	}
	return nil
}

// MigrateReset 显式重建:MigrateDown → MigrateUp → 全量种子。
// 用于旧(整数外键)schema 升级到 UUID 业务键 schema——存量数据不迁移,库重建后由
// 种子链重置内置内容;Cobra 侧经 `migrate reset`(带确认)调用。任何阶段失败立即返回。
func MigrateReset() error {
	if DB == nil {
		return fmt.Errorf("数据库未初始化")
	}
	if err := MigrateDown(); err != nil {
		return fmt.Errorf("reset 清除旧表失败: %w", err)
	}
	if err := MigrateUp(); err != nil {
		return fmt.Errorf("reset 重建表失败: %w", err)
	}
	if err := SeedAll(GetDB()); err != nil {
		return fmt.Errorf("reset 写入种子失败: %w", err)
	}
	return nil
}

// HasSchema 探测是否已经存在任意业务表,供 serve 启动决定是否需要 AutoMigrate
// (避免每次启动都跑一遍全表 schema diff 的日志噪声)
func HasSchema() (bool, error) {
	if DB == nil {
		return false, fmt.Errorf("数据库未初始化")
	}
	return DB.Migrator().HasTable(&model.ElixirPill{}), nil
}

// MaybeAutoMigrate 启动期调用: SKIP_AUTO_MIGRATE=1 关闭,否则总是执行 MigrateUp(幂等)。
// 变更背景:旧逻辑在 schema 已存在时短路(配合 HasSchema 避免启动日志噪声),但桌面启动
// 没有 CLI migrate 入口,新列(如 behavior_profile)永远不会落到既有库;
// AutoMigrate 幂等且只补齐新增列/索引,代价仅是启动时一次 schema diff,收益是
// 老库自动升级(spec §15)。
func MaybeAutoMigrate() error {
	if DB == nil {
		return fmt.Errorf("数据库未初始化")
	}
	if isAutoMigrateDisabled() {
		return nil
	}
	return MigrateUp()
}

// isAutoMigrateDisabled 检查 SKIP_AUTO_MIGRATE / AF_SKIP_AUTO_MIGRATE
// 接受 1 / true / TRUE(大小写不敏感),其他值视为启用
func isAutoMigrateDisabled() bool {
	for _, name := range []string{"SKIP_AUTO_MIGRATE", "AF_SKIP_AUTO_MIGRATE"} {
		if v, ok := os.LookupEnv(name); ok {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "1", "true", "yes", "on":
				return true
			}
		}
	}
	return false
}
