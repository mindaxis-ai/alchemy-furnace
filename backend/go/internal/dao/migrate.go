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
	&model.LLMProvider{},
	&model.LLMModel{},
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
	return nil
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

// MaybeAutoMigrate 启动期调用: SKIP_AUTO_MIGRATE=1 关闭,否则在空库上跑 AutoMigrate
// 配合 HasSchema 实现「零配置首次启动」体验
//   - 空库: 自动建表(SQLite/新环境零门槛)
//   - 已有 schema: 跳过(避免每次启动刷一堆 AutoMigrate 日志)
//   - 显式关闭: 生产受控环境(运维走自己的迁移流水线)
func MaybeAutoMigrate() error {
	if DB == nil {
		return fmt.Errorf("数据库未初始化")
	}
	if isAutoMigrateDisabled() {
		return nil
	}
	has, err := HasSchema()
	if err != nil {
		return err
	}
	if has {
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
