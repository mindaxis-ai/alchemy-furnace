package memory_service

import (
	"context"
	"testing"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const testAgentUID = "0197a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"

func newTestService(t *testing.T) (*MemoryService, *dao.MemoryDao) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentMemory{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dao.DB = db
	t.Cleanup(func() { dao.DB = nil })
	d := dao.NewMemoryDao()
	return NewMemoryService(d, nil, nil), d
}

func seed(t *testing.T, d *dao.MemoryDao, agentID string, m *model.AgentMemory) {
	t.Helper()
	m.AgentID = agentID
	if m.AgentMemoryID == "" {
		m.AgentMemoryID = uuid.New().String()
	}
	if err := d.SaveMemory(context.Background(), m); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestCreateMemoryValidation(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	// 非法 kind
	if _, err := svc.CreateMemory(ctx, testAgentUID, service.MemoryInput{Kind: "bogus", Content: "x"}); err == nil {
		t.Fatal("非法 kind 应拒绝")
	}
	// 内容超 500 字
	long := string(make([]rune, 501))
	if _, err := svc.CreateMemory(ctx, testAgentUID, service.MemoryInput{Kind: "user_fact", Content: long}); err == nil {
		t.Fatal("超长内容应拒绝")
	}
	// 空内容
	if _, err := svc.CreateMemory(ctx, testAgentUID, service.MemoryInput{Kind: "user_fact", Content: "  "}); err == nil {
		t.Fatal("空内容应拒绝")
	}
	// importance 越界
	i := 9
	if _, err := svc.CreateMemory(ctx, testAgentUID, service.MemoryInput{Kind: "user_fact", Content: "ok", Importance: &i}); err == nil {
		t.Fatal("importance>5 应拒绝")
	}
	// 合法创建
	m, err := svc.CreateMemory(ctx, testAgentUID, service.MemoryInput{Kind: "user_fact", Content: "用户喜欢围棋", Keywords: []string{"围棋"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.Status != "active" || m.ContentHash == "" {
		t.Fatalf("新记忆应 active 且带哈希: %+v", m)
	}
}

func TestCreateMemorySameHashMergesImportance(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	m1, _ := svc.CreateMemory(ctx, testAgentUID, service.MemoryInput{Kind: "user_fact", Content: "用户喜欢围棋", Importance: intPtr(2), Confidence: floatPtr(0.6)})
	m2, err := svc.CreateMemory(ctx, testAgentUID, service.MemoryInput{Kind: "user_fact", Content: "用户喜欢围棋", Importance: intPtr(4), Confidence: floatPtr(0.9)})
	if err != nil {
		t.Fatalf("同哈希再创建: %v", err)
	}
	if m2.AgentMemoryID != m1.AgentMemoryID {
		t.Fatalf("同哈希应合并到同一记录: id1=%s id2=%s", m1.AgentMemoryID, m2.AgentMemoryID)
	}
	if m2.Importance != 4 || m2.Confidence != 0.9 {
		t.Fatalf("合并后 importance/confidence 应取新值: %+v", m2)
	}
	list, _ := svc.ListMemories(ctx, testAgentUID, "", true)
	if len(list) != 1 {
		t.Fatalf("应只有 1 条记录: %d", len(list))
	}
}

func TestCreateMemoryConflictSupersedesOld(t *testing.T) {
	svc, d := newTestService(t)
	ctx := context.Background()
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "user_preference", Content: "用户喜欢安静交流,不喜欢太热闹", ContentHash: "old", Importance: 3})
	// 同 kind、内容 bigram ≥0.85(仅末尾多一个"了")→ 冲突:旧 active 置替为新 superseded
	_, err := svc.CreateMemory(ctx, testAgentUID, service.MemoryInput{Kind: "user_preference", Content: "用户喜欢安静交流,不喜欢太热闹了"})
	if err != nil {
		t.Fatalf("create conflicting: %v", err)
	}
	all, _ := svc.ListMemories(ctx, testAgentUID, "", false)
	activeCount, supersededCount := 0, 0
	for _, m := range all {
		if m.Status == "active" {
			activeCount++
		}
		if m.Status == "superseded" {
			supersededCount++
		}
	}
	if activeCount != 1 || supersededCount != 1 {
		t.Fatalf("冲突应 1 active + 1 superseded: %+v", all)
	}
}

func TestCreateMemoryPinnedNeverSuperseded(t *testing.T) {
	svc, d := newTestService(t)
	ctx := context.Background()
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "user_preference", Content: "用户喜欢安静交流,不喜欢太热闹", ContentHash: "old", Pinned: true, Importance: 3})
	// 同内容冲突 + 旧记忆 pinned → 旧记忆保留 active,新记忆照常创建,不产生 superseded
	_, err := svc.CreateMemory(ctx, testAgentUID, service.MemoryInput{Kind: "user_preference", Content: "用户喜欢安静交流,不喜欢太热闹了"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	all, _ := svc.ListMemories(ctx, testAgentUID, "", false)
	supersededCount, activePinned := 0, 0
	for _, m := range all {
		if m.Status == "superseded" {
			supersededCount++
		}
		if m.Status == "active" && m.Pinned {
			activePinned++
		}
	}
	if len(all) != 2 || supersededCount != 0 || activePinned != 1 {
		t.Fatalf("pinned 应保留 active 且不被置替: %+v", all)
	}
}

func TestRetrieveRankingAndCaps(t *testing.T) {
	svc, d := newTestService(t)
	ctx := context.Background()
	// 7 条:1 pinned + 6 普通(importance 各异,含 1 条关键词命中)
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "user_fact", Content: "用户喜欢围棋,每周复盘", ContentHash: "k", Importance: 1, Pinned: true})
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "episode", Content: "上周用户提到工作压力大", ContentHash: "e1", Importance: 5})
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "episode", Content: "上个月一起喝了茶", ContentHash: "e2", Importance: 4})
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "user_preference", Content: "用户喜欢喝茶", ContentHash: "e3", Importance: 3})
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "user_preference", Content: "用户喜欢爬山", ContentHash: "e4", Importance: 3})
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "relationship", Content: "用户与老君交好", ContentHash: "e5", Importance: 2})
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "open_loop", Content: "答应帮用户查棋谱", ContentHash: "e6", Importance: 1})

	snips, err := svc.Retrieve(ctx, testAgentUID, "聊聊围棋的布局")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(snips) > 6 {
		t.Fatalf("≤6 条上限: %d", len(snips))
	}
	// pinned 必在首位
	if len(snips) == 0 || snips[0].Content != "用户喜欢围棋,每周复盘" {
		t.Fatalf("pinned 应排首位: %+v", snips)
	}
	total := 0
	for _, s := range snips {
		total += len([]rune(s.Content))
	}
	if total > 1200 {
		t.Fatalf("总字符超 1200: %d", total)
	}
}

func TestRetrieveEmptyMessageFallsBackToImportance(t *testing.T) {
	svc, d := newTestService(t)
	ctx := context.Background()
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "episode", Content: "低重要度记忆", ContentHash: "l", Importance: 1})
	seed(t, d, testAgentUID, &model.AgentMemory{Kind: "episode", Content: "高重要度记忆", ContentHash: "h", Importance: 5})
	snips, err := svc.Retrieve(ctx, testAgentUID, "")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(snips) != 2 || snips[0].Content != "高重要度记忆" {
		t.Fatalf("空消息应按 importance 降序: %+v", snips)
	}
}

func TestMemoryCRUDAndClear(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	m, _ := svc.CreateMemory(ctx, testAgentUID, service.MemoryInput{Kind: "episode", Content: "一次对话"})
	pinned := true
	updated, err := svc.UpdateMemory(ctx, testAgentUID, uuid.MustParse(m.AgentMemoryID), service.MemoryInput{Content: "一次重要对话", Pinned: &pinned})
	if err != nil || !updated.Pinned || updated.Content != "一次重要对话" {
		t.Fatalf("update: %+v err=%v", updated, err)
	}
	if err := svc.DeleteMemory(ctx, testAgentUID, uuid.MustParse(m.AgentMemoryID)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	left, _ := svc.ListMemories(ctx, testAgentUID, "", false)
	if len(left) != 0 {
		t.Fatalf("删除后应无记录: %d", len(left))
	}
}

func intPtr(i int) *int           { return &i }
func floatPtr(f float64) *float64 { return &f }
