// Package dao 数据库访问层: 多数据库连接管理 + GORM AutoMigrate + 种子
// schema 来源为 model/*.go 的 GORM 标签;运行时由 db.AutoMigrate 全自动建表
// 驱动由 configuration.DatabaseConfig.Driver 决定(postgres / mysql / sqlite)
//   - 生产推荐: postgres(支持 JSONB / 部分唯一索引 / UUID 原生类型)
//   - 兼容 MySQL: 需 8.0.13+ 才能利用部分唯一索引;更低版本靠 service 层校验兜底
//   - 零依赖体验: sqlite(单文件 ./data/alchemy.db,Demo / 试用 / 单机首选)
package dao

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/alchemy-furnace/server/internal/configuration"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 全局数据库连接实例
var DB *gorm.DB

// InitDatabase 初始化数据库连接(不执行迁移/种子,二者由调用方显式触发)
// 驱动由 cfg.Driver 决定;SQLite 自动创建父目录
func InitDatabase(cfg *configuration.DatabaseConfig) error {
	log.Printf("[炼丹炉] 正在连接数据库: driver=%s host=%s port=%d dbname=%s",
		cfg.Driver, cfg.Host, cfg.Port, cfg.DBName)

	logLevel := logger.Info
	if configuration.Configuration.Server.Mode == "release" {
		logLevel = logger.Error
	}

	gormCfg := &gorm.Config{
		Logger: logger.New(
			log.New(log.Writer(), "[GORM] ", log.LstdFlags),
			logger.Config{
				SlowThreshold:             200 * time.Millisecond,
				LogLevel:                  logLevel,
				IgnoreRecordNotFoundError: true,
				Colorful:                  true,
			},
		),
	}

	db, err := openByDriver(cfg, gormCfg)
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取底层数据库连接失败: %w", err)
	}
	applyConnPool(sqlDB, cfg.Driver)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("数据库连接测试失败: %w", err)
	}

	DB = db
	log.Printf("[炼丹炉] 数据库连接成功,仙炉已就位 (driver=%s)", cfg.Driver)
	return nil
}

// openByDriver 按 cfg.Driver 选 GORM 驱动并 Open 连接
func openByDriver(cfg *configuration.DatabaseConfig, gormCfg *gorm.Config) (*gorm.DB, error) {
	switch cfg.Driver {
	case configuration.DriverSQLite:
		// SQLite 单文件: 父目录若不存在则自动创建,确保首次启动零配置
		if dir := filepath.Dir(cfg.SQLitePath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("创建 SQLite 父目录失败 (%s): %w", dir, err)
			}
		}
		return gorm.Open(sqlite.Open(cfg.DSN()), gormCfg)
	case configuration.DriverPostgres:
		return gorm.Open(postgres.Open(cfg.DSN()), gormCfg)
	case configuration.DriverMySQL:
		return gorm.Open(mysql.Open(cfg.DSN()), gormCfg)
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %q", cfg.Driver)
	}
}

// applyConnPool 按驱动差异化设置连接池
//   - SQLite: 单写锁,连接数强制 1,否则并发写会触发 "database is locked"
//   - Postgres / MySQL: 走通用池配置
func applyConnPool(sqlDB interface {
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
	SetConnMaxLifetime(time.Duration)
	SetConnMaxIdleTime(time.Duration)
}, driver string) {
	if driver == configuration.DriverSQLite {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(0) // SQLite 单连接长持有即可
		sqlDB.SetConnMaxIdleTime(0)
		return
	}
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)
}

// GetDB 获取数据库实例,未初始化则 panic
func GetDB() *gorm.DB {
	if DB == nil {
		log.Fatal("[炼丹炉] 致命错误: 数据库未初始化,请先调用 InitDatabase")
	}
	return DB
}

// Transaction 在事务中执行函数,fn 返回错误则回滚
func Transaction(fn func(tx *gorm.DB) error) error {
	return DB.Transaction(fn)
}

// CloseDatabase 关闭数据库连接
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

// IsNoRows 兼容包: gorm.ErrRecordNotFound 重新导出,避免 service 层直接 import gorm
func IsNoRows(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
