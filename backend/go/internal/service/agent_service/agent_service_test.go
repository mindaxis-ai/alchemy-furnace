// 完整服丹编排(ReplacePillComposition)服务层测试
// 真实 sqlite 内存库 + 真实 DAO,不引入 mock;覆盖校验顺序与单次原子 DAO 调用
package agent_service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// setupServiceTestDB 建内存库并注入全局 dao.DB,返回真实 service 与 db
func setupServiceTestDB(t *testing.T) (*Agent, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:agentsvc%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("打开 sqlite 内存库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.DaoAgent{}, &model.ElixirPill{}, &model.AgentPill{}, &model.LanguagePattern{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	dao.DB = db
	t.Cleanup(func() { dao.DB = nil })
	return New(dao.NewAgentDao(), dao.NewPillDao(), dao.NewModelDao()), db
}

// seedAgentAndPills 造一个道人与 n 枚金丹
func seedAgentAndPills(t *testing.T, db *gorm.DB, pillCount int) (*model.DaoAgent, []*model.ElixirPill) {
	t.Helper()
	agent := &model.DaoAgent{Name: "编排道人", ModelName: "gpt-4o", Status: "active"}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("创建道人失败: %v", err)
	}
	pills := make([]*model.ElixirPill, 0, pillCount)
	for i := 0; i < pillCount; i++ {
		p := &model.ElixirPill{Name: uuid.NewString(), SkillSchema: model.JSONMap{"identity_card": "x"}}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("创建金丹失败: %v", err)
		}
		pills = append(pills, p)
	}
	return agent, pills
}

func TestReplacePillComposition_SuccessOrdersAndWeights(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, pills := seedAgentAndPills(t, db, 3)
	// 预置有效缓存,成功后应失效
	if err := db.Create(&model.LanguagePattern{AgentID: agent.ID, SystemPrompt: "c", SourceFingerprint: "sha256:x", IsValid: true}).Error; err != nil {
		t.Fatalf("建缓存失败: %v", err)
	}

	// 故意乱序 + 自定义权重
	items := []service.PillCompositionItem{
		{PillUUID: pills[2].UUID, Weight: 2.5},
		{PillUUID: pills[0].UUID, Weight: 1.0},
		{PillUUID: pills[1].UUID, Weight: 0.5},
	}
	detail, err := svc.ReplacePillComposition(context.Background(), agent.UUID, items)
	if err != nil {
		t.Fatalf("ReplacePillComposition 报错: %v", err)
	}
	if len(detail.AgentPills) != 3 {
		t.Fatalf("服用记录数 = %d, 期望 3", len(detail.AgentPills))
	}
	want := []uuid.UUID{pills[2].UUID, pills[0].UUID, pills[1].UUID}
	wantW := []float64{2.5, 1.0, 0.5}
	for i, ap := range detail.AgentPills {
		if ap.Pill.UUID != want[i] {
			t.Fatalf("第 %d 位 pill = %s, 期望 %s(顺序应保持请求顺序)", i, ap.Pill.UUID, want[i])
		}
		if ap.Weight != wantW[i] {
			t.Fatalf("第 %d 位 weight = %v, 期望 %v", i, ap.Weight, wantW[i])
		}
		if ap.SortOrder != i+1 {
			t.Fatalf("第 %d 位 sort_order = %d, 期望 %d", i, ap.SortOrder, i+1)
		}
	}

	// 缓存已失效
	var pattern model.LanguagePattern
	if err := db.Where("agent_id = ?", agent.ID).First(&pattern).Error; err != nil {
		t.Fatalf("读缓存失败: %v", err)
	}
	if pattern.IsValid {
		t.Fatal("语言模式缓存未被失效")
	}
}

func TestReplacePillComposition_EmptyClearsRelations(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, pills := seedAgentAndPills(t, db, 2)
	// 先有绑定
	if _, err := svc.ReplacePillComposition(context.Background(), agent.UUID, []service.PillCompositionItem{
		{PillUUID: pills[0].UUID, Weight: 1},
		{PillUUID: pills[1].UUID, Weight: 1},
	}); err != nil {
		t.Fatalf("预置编排失败: %v", err)
	}
	// 空数组清空
	detail, err := svc.ReplacePillComposition(context.Background(), agent.UUID, nil)
	if err != nil {
		t.Fatalf("空数组 ReplacePillComposition 报错: %v", err)
	}
	if len(detail.AgentPills) != 0 {
		t.Fatalf("清空后服用记录数 = %d, 期望 0", len(detail.AgentPills))
	}
}

func TestReplacePillComposition_DuplicateUUIDRejected(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, pills := seedAgentAndPills(t, db, 2)
	err := mustErr(svc.ReplacePillComposition(context.Background(), agent.UUID, []service.PillCompositionItem{
		{PillUUID: pills[0].UUID, Weight: 1},
		{PillUUID: pills[0].UUID, Weight: 2},
	}))
	assertErrType(t, err, errors.ErrorTypeInvalidRequest, "重复金丹 UUID")
	// 不应产生任何绑定
	var count int64
	if db.Model(&model.AgentPill{}).Where("agent_id = ?", agent.ID).Count(&count); count != 0 {
		t.Fatalf("重复被拒后仍写入 %d 条绑定", count)
	}
}

func TestReplacePillComposition_PillNotFound(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, pills := seedAgentAndPills(t, db, 1)
	err := mustErr(svc.ReplacePillComposition(context.Background(), agent.UUID, []service.PillCompositionItem{
		{PillUUID: pills[0].UUID, Weight: 1},
		{PillUUID: uuid.New(), Weight: 1}, // 不存在
	}))
	assertErrType(t, err, errors.ErrorTypeRecordNotFound, "金丹不存在")
}

func TestReplacePillComposition_WeightOutOfRange(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, pills := seedAgentAndPills(t, db, 1)
	for _, w := range []float64{0, -1, 10.5, 11} {
		err := mustErr(svc.ReplacePillComposition(context.Background(), agent.UUID, []service.PillCompositionItem{
			{PillUUID: pills[0].UUID, Weight: w},
		}))
		assertErrType(t, err, errors.ErrorTypeInvalidRequest, fmt.Sprintf("权重 %v 越界", w))
	}
}

func TestReplacePillComposition_AgentNotFound(t *testing.T) {
	svc, _ := setupServiceTestDB(t)
	err := mustErr(svc.ReplacePillComposition(context.Background(), uuid.New(), nil))
	assertErrType(t, err, errors.ErrorTypeRecordNotFound, "道人不存在的 UUID")
}

// mustErr 从 (detail, err) 取出 err 并断言非空
func mustErr(_ *model.DaoAgent, err errors.Error) errors.Error {
	if err == nil {
		panic("期望返回错误,实际为 nil")
	}
	return err
}

// assertErrType 断言错误类型(穿透 Relation 链)
func assertErrType(t *testing.T, err errors.Error, want errors.ErrorType, what string) {
	t.Helper()
	if !errors.IsType(err, want) {
		t.Fatalf("%s: 错误类型不匹配 (code=%s, msg=%s), 期望类型 %v", what, err.GetCode(), err.Error(), want)
	}
}
