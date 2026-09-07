package dao

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TestDeletePillCascadeRemovesAgentPillsByUUIDText 锁定 DeletePill 的服用记录级联按 UUID 文本匹配:
// agent_pills.pill_id 是 UUID 文本列(011),旧实现用内部 uint pill.ID 匹配会删 0 行且被外键 CASCADE
// 掩盖,残留孤儿服用记录(2026-09 Task 6 修复点)。本用例刻意不开外键,让显式删除的结果可独立断言。
func TestDeletePillCascadeRemovesAgentPillsByUUIDText(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pill_delete.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if raw, err := db.DB(); err != nil {
		t.Fatal(err)
	} else {
		raw.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = raw.Close() })
	}
	if err := db.AutoMigrate(&model.DaoAgent{}, &model.ElixirPill{}, &model.AgentPill{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	agent := &model.DaoAgent{DaoAgentID: uuid.New().String(), Name: "级联测试道人"}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	pill := &model.ElixirPill{ElixirPillID: uuid.New().String(), Name: "级联测试金丹", SkillSchema: model.JSONMap{}}
	if err := db.Create(pill).Error; err != nil {
		t.Fatalf("create pill: %v", err)
	}
	other := &model.ElixirPill{ElixirPillID: uuid.New().String(), Name: "无关金丹", SkillSchema: model.JSONMap{}}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("create other pill: %v", err)
	}
	if err := db.Create(&model.AgentPill{AgentID: agent.DaoAgentID, PillID: pill.ElixirPillID}).Error; err != nil {
		t.Fatalf("create agent_pill: %v", err)
	}

	d := &PillDao{}
	if err := d.DeletePill(context.Background(), pill); err != nil {
		t.Fatalf("DeletePill: %v", err)
	}

	var left int64
	if err := db.Model(&model.AgentPill{}).Where("pill_id = ?", pill.ElixirPillID).Count(&left).Error; err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("DeletePill 后 agent_pills 残留 %d 行(级联未按 UUID 文本匹配)", left)
	}
	var total int64
	if err := db.Model(&model.AgentPill{}).Count(&total).Error; err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("agent_pills 总行数 %d, want 0", total)
	}
}
