package dao

import (
	"context"
	"testing"

	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// newMemoryTestDB 内存库注入全局 dao.DB(仿 migrate_smoke_test 的 DB 注入模式)
// testAgentUUID 测试用道人业务标识(uuid 文本,与其他域测试同一约定)
var testAgentUUID = uuid.NewString()

func newMemoryTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentMemory{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = db
	t.Cleanup(func() { DB = nil })
}

func TestMemoryDAOCreateAndList(t *testing.T) {
	newMemoryTestDB(t)
	d := NewMemoryDao()
	ctx := context.Background()

	m1 := &model.AgentMemory{
		UUID: uuid.New(), AgentID: testAgentUUID, Kind: "user_fact",
		Content: "用户喜欢围棋", ContentHash: "h1", Importance: 4, Confidence: 0.9,
	}
	m2 := &model.AgentMemory{
		UUID: uuid.New(), AgentID: testAgentUUID, Kind: "episode",
		Content: "上个月一起复盘了一盘棋", ContentHash: "h2",
	}
	if err := d.SaveMemory(ctx, m1); err != nil {
		t.Fatalf("save m1: %v", err)
	}
	if err := d.SaveMemory(ctx, m2); err != nil {
		t.Fatalf("save m2: %v", err)
	}
	all, err := d.ListMemories(ctx, testAgentUUID, "", true)
	if err != nil || len(all) != 2 {
		t.Fatalf("list all: n=%d err=%v", len(all), err)
	}
	fact, err := d.ListMemories(ctx, testAgentUUID, "user_fact", true)
	if err != nil || len(fact) != 1 || fact[0].Kind != "user_fact" {
		t.Fatalf("list by kind: n=%d err=%v", len(fact), err)
	}
	// onlyActive=false 包含 superseded
	if err := d.SupersedeMemory(ctx, m1.ID); err != nil {
		t.Fatalf("supersede: %v", err)
	}
	activeOnly, _ := d.ListMemories(ctx, testAgentUUID, "", true)
	if len(activeOnly) != 1 {
		t.Fatalf("active only: n=%d, want 1", len(activeOnly))
	}
	withSuperseded, _ := d.ListMemories(ctx, testAgentUUID, "", false)
	if len(withSuperseded) != 2 {
		t.Fatalf("with superseded: n=%d, want 2", len(withSuperseded))
	}
}

func TestMemoryDAOTakeByUUIDAndHash(t *testing.T) {
	newMemoryTestDB(t)
	d := NewMemoryDao()
	ctx := context.Background()
	m := &model.AgentMemory{
		UUID: uuid.New(), AgentID: testAgentUUID, Kind: "user_fact",
		Content: "用户喜欢围棋", ContentHash: "h1",
	}
	if err := d.SaveMemory(ctx, m); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := d.TakeMemoryByUUID(ctx, m.UUID)
	if err != nil || got.ID != m.ID {
		t.Fatalf("take by uuid: %+v err=%v", got, err)
	}
	hit, err := d.FindActiveByContentHash(ctx, testAgentUUID, "h1")
	if err != nil || hit == nil || hit.ID != m.ID {
		t.Fatalf("find by hash: %+v err=%v", hit, err)
	}
	if _, err := d.TakeMemoryByUUID(ctx, uuid.New()); err == nil {
		t.Fatal("missing uuid 应返回错误")
	}
}

func TestMemoryDAOSupersedeAndTouchAndDelete(t *testing.T) {
	newMemoryTestDB(t)
	d := NewMemoryDao()
	ctx := context.Background()
	m := &model.AgentMemory{UUID: uuid.New(), AgentID: testAgentUUID, Kind: "user_fact", Content: "x", ContentHash: "h"}
	if err := d.SaveMemory(ctx, m); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := d.TouchMemory(ctx, m.ID); err != nil {
		t.Fatalf("touch: %v", err)
	}
	after, _ := d.GetMemory(ctx, m.ID)
	if after.LastAccessedAt == nil {
		t.Fatal("touch 后 LastAccessedAt 应非空")
	}
	if err := d.SupersedeMemory(ctx, m.ID); err != nil {
		t.Fatalf("supersede: %v", err)
	}
	s, _ := d.GetMemory(ctx, m.ID)
	if s.Status != "superseded" {
		t.Fatalf("status=%q, want superseded", s.Status)
	}
	n, err := d.DeleteMemoriesByAgent(ctx, testAgentUUID)
	if err != nil || n != 1 {
		t.Fatalf("delete by agent: n=%d err=%v", n, err)
	}
	rest, _ := d.ListMemories(ctx, testAgentUUID, "", false)
	if len(rest) != 0 {
		t.Fatalf("物理删除后仍残留 %d 条", len(rest))
	}
}
