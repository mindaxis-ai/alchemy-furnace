package dao

import (
	"testing"

	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSeedBuiltinPillsIsIdempotentAndPreservesExistingData(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.ElixirPill{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if err := SeedBuiltinPills(db); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if err := db.Model(&model.ElixirPill{}).
		Where("name = ?", "文言文金丹").
		Update("description", "用户保留的说明").Error; err != nil {
		t.Fatalf("customize builtin: %v", err)
	}
	if err := SeedBuiltinPills(db); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	var count int64
	if err := db.Model(&model.ElixirPill{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 5 {
		t.Fatalf("pill count = %d, want 5", count)
	}

	var pill model.ElixirPill
	if err := db.Where("name = ?", "文言文金丹").First(&pill).Error; err != nil {
		t.Fatalf("load customized pill: %v", err)
	}
	if pill.Description != "用户保留的说明" {
		t.Fatalf("description was overwritten: %q", pill.Description)
	}
}
