// Package dao 数据库迁移相关
// 提供数据初始化、索引创建、约束设置等额外的数据库迁移操作
package dao

import (
	"log"

	"github.com/alchemy-furnace/server/model"
	"gorm.io/gorm"
)

// RunMigrations 执行完整的迁移流程，包括自动建表、索引创建、初始数据
func RunMigrations(db *gorm.DB) error {
	log.Println("[炼丹炉] 执行数据库迁移...")

	// 1. 自动迁移表结构
	if err := AutoMigrate(db); err != nil {
		return err
	}

	// 2. 创建索引（GORM AutoMigrate 会自动创建索引，但自定义索引可以在这里添加）
	if err := createIndexes(db); err != nil {
		return err
	}

	// 3. 插入初始数据（如默认道人等）
	if err := seedInitialData(db); err != nil {
		return err
	}

	log.Println("[炼丹炉] 数据库迁移全部完成")
	return nil
}

// createIndexes 创建额外的数据库索引，优化查询性能
func createIndexes(db *gorm.DB) error {
	log.Println("[炼丹炉] 创建数据库索引...")

	// 使用原生 SQL 创建索引（如果 GORM tag 中的索引不够）
	indexes := []string{
		// dao_agents 表索引
		`CREATE INDEX IF NOT EXISTS idx_agents_status ON dao_agents(status)`,

		// agent_pills 表索引（联合唯一索引由 GORM 创建）
		`CREATE INDEX IF NOT EXISTS idx_agent_pills_pill_id ON agent_pills(pill_id)`,

		// language_patterns 表索引
		`CREATE INDEX IF NOT EXISTS idx_language_patterns_agent_id ON language_patterns(agent_id)`,

		// elixir_pills 表索引
		`CREATE INDEX IF NOT EXISTS idx_elixir_pills_is_builtin ON elixir_pills(is_builtin)`,

		// chat_sessions 表索引
		`CREATE INDEX IF NOT EXISTS idx_sessions_agent_id ON chat_sessions(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON chat_sessions(updated_at)`,

		// chat_messages 表索引
		`CREATE INDEX IF NOT EXISTS idx_messages_session_id ON chat_messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_created_at ON chat_messages(created_at)`,
	}

	for _, idx := range indexes {
		if err := db.Exec(idx).Error; err != nil {
			log.Printf("[炼丹炉] 创建索引警告: %v", err)
			// 索引创建失败不阻塞启动，可能只是已存在
		}
	}

	return nil
}

// seedInitialData 插入初始数据，如系统默认道人
func seedInitialData(db *gorm.DB) error {
	log.Println("[炼丹炉] 检查并插入初始数据...")

	// 检查是否已有道人数据
	var count int64
	if err := db.Model(&model.DaoAgent{}).Count(&count).Error; err != nil {
		return err
	}

	// 如果没有道人，创建一个默认的太上老君道人
	if count == 0 {
		defaultAgent := model.DaoAgent{
			Name:        "太上老君",
			Avatar:      "",
			Personality: "你是太上老君，道教三清之一，擅长炼丹和解答修行疑问。你说话带有古风，常用道教典故，性格沉稳睿智。",
			ModelName:   "gpt-4o",
			Status:      "active",
		}
		if err := db.Create(&defaultAgent).Error; err != nil {
			return err
		}
		log.Println("[炼丹炉] 已创建默认道人：太上老君")
	}

	return nil
}

// DropAllTables 删除所有表（危险操作，仅用于开发测试环境）
func DropAllTables(db *gorm.DB) error {
	log.Println("[炼丹炉] 警告：正在删除所有表...")
	tables := []string{
		"chat_messages",
		"chat_sessions",
		"language_patterns",
		"agent_pills",
		"dao_agents",
		"elixir_recipes",
		"elixir_pills",
	}
	for _, table := range tables {
		if err := db.Migrator().DropTable(table); err != nil {
			log.Printf("[炼丹炉] 删除表 %s 失败: %v", table, err)
		}
	}
	log.Println("[炼丹炉] 所有表已删除")
	return nil
}

// ResetDatabase 重置数据库（删除所有表并重新迁移，危险操作）
func ResetDatabase(db *gorm.DB) error {
	if err := DropAllTables(db); err != nil {
		return err
	}
	return RunMigrations(db)
}
