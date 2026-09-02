// GORM AutoMigrate 驱动: 多数据库统一 schema 同步
// 替代历史 golang-migrate SQL 方案: 由 model/*.go 的 GORM 标签作为唯一事实来源
//   - 幂等: 重复执行只补齐新增列/索引,不会破坏已有数据
//   - 驱动无关: 同一组模型在 postgres / mysql / sqlite 上生成等价表结构
//   - 部分唯一索引(is_default / is_synthesis / is_fusion):
//       模型 tag 上声明 where 子句,PG/SQLite 自动生成 partial index;
//       MySQL 8.0.13+ 同步支持,更早版本降级为普通 unique 索引并由 service 层兜底
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
	&model.PillMigrationState{},
	&model.PillLegacyMap{},
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

// MigrateUp 同步全部业务表到当前模型定义(幂等,跨驱动)
// 历史 raw-SQL 迁移文件已不再依赖;若是从旧部署首次切换,可重复运行直至无差异
func MigrateUp() error {
	if DB == nil {
		return fmt.Errorf("数据库未初始化")
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
