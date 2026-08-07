// Package dao 负责「炼丹炉」的数据库访问层
// 使用 GORM 作为 ORM 框架，连接 PostgreSQL 数据库
// 提供数据库连接初始化、自动迁移、事务支持等功能
package dao

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/pkg/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 全局数据库连接实例，各 service 层通过此实例访问数据库
var DB *gorm.DB

// InitDatabase 初始化数据库连接
// 根据配置连接 PostgreSQL，设置连接池参数，并执行自动迁移
func InitDatabase(cfg *config.DatabaseConfig) error {
	dsn := cfg.DSN()
	log.Printf("[炼丹炉] 正在连接数据库: host=%s port=%d dbname=%s", cfg.Host, cfg.Port, cfg.DBName)

	// 配置 GORM 日志级别
	logLevel := logger.Info
	if config.Get().Server.Mode == "release" {
		logLevel = logger.Error
	}

	// 打开数据库连接
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.New(
			log.New(log.Writer(), "[GORM] ", log.LstdFlags),
			logger.Config{
				SlowThreshold:             200 * time.Millisecond, // 慢查询阈值
				LogLevel:                  logLevel,
				IgnoreRecordNotFoundError: true, // 忽略记录不存在的错误
				Colorful:                  true,
			},
		),
	})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	// 获取底层的 *sql.DB 实例，配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取底层数据库连接失败: %w", err)
	}

	// 连接池配置
	sqlDB.SetMaxOpenConns(100)                 // 最大打开连接数
	sqlDB.SetMaxIdleConns(10)                  // 最大空闲连接数
	sqlDB.SetConnMaxLifetime(30 * time.Minute) // 连接最大生命周期
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // 空闲连接最大存活时间

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("数据库连接测试失败: %w", err)
	}

	DB = db
	log.Println("[炼丹炉] 数据库连接成功，仙炉已就位")

	// 执行自动迁移，创建表结构
	if err := AutoMigrate(db); err != nil {
		return fmt.Errorf("自动迁移失败: %w", err)
	}

	// 迁移完成后写入内置示例金丹种子数据（幂等：同名金丹已存在则跳过）
	if err := SeedBuiltinPills(db); err != nil {
		return fmt.Errorf("写入内置金丹种子数据失败: %w", err)
	}

	return nil
}

// AutoMigrate 自动迁移数据库表结构
// 根据 GORM 模型创建或更新数据库表，确保表结构与代码模型一致
func AutoMigrate(db *gorm.DB) error {
	log.Println("[炼丹炉] 开始自动迁移数据库表结构...")

	models := []interface{}{
		&model.ElixirPill{},      // 金丹表
		&model.DaoAgent{},        // 道人表
		&model.AgentPill{},       // 服用记录表
		&model.LanguagePattern{}, // 语言模式缓存表
		&model.ChatSession{},     // 会话表
		&model.ChatMessage{},     // 消息表
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("迁移表 %+v 失败: %w", m, err)
		}
	}

	// 删除已废弃的丹方表（RAG 遗留）
	if db.Migrator().HasTable("elixir_recipes") {
		if err := db.Migrator().DropTable("elixir_recipes"); err != nil {
			return fmt.Errorf("删除废弃表 elixir_recipes 失败: %w", err)
		}
		log.Println("[炼丹炉] 已删除废弃表: elixir_recipes")
	}

	// 移除金丹表的 RAG 遗留列（status / vector_count）
	if db.Migrator().HasColumn(&model.ElixirPill{}, "status") {
		if err := db.Migrator().DropColumn(&model.ElixirPill{}, "status"); err != nil {
			return fmt.Errorf("删除废弃列 elixir_pills.status 失败: %w", err)
		}
	}
	if db.Migrator().HasColumn(&model.ElixirPill{}, "vector_count") {
		if err := db.Migrator().DropColumn(&model.ElixirPill{}, "vector_count"); err != nil {
			return fmt.Errorf("删除废弃列 elixir_pills.vector_count 失败: %w", err)
		}
	}

	log.Println("[炼丹炉] 数据库表迁移完成，共 6 张表：elixir_pills, dao_agents, agent_pills, language_patterns, chat_sessions, chat_messages")
	return nil
}

// GetDB 获取数据库实例，如果未初始化则 panic
func GetDB() *gorm.DB {
	if DB == nil {
		log.Fatal("[炼丹炉] 致命错误: 数据库未初始化，请先调用 InitDatabase")
	}
	return DB
}

// Transaction 在事务中执行函数，自动处理提交和回滚
// fn 中如果返回错误，事务会自动回滚；否则提交
func Transaction(fn func(tx *gorm.DB) error) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
}

// CloseDatabase 关闭数据库连接，在程序退出时调用
func CloseDatabase() error {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	log.Println("[炼丹炉] 关闭数据库连接")
	return sqlDB.Close()
}
