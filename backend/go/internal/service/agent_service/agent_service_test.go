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
	if err := db.AutoMigrate(
		&model.DaoAgent{}, &model.ElixirPill{}, &model.AgentPill{}, &model.LanguagePattern{},
		&model.ChatSession{}, &model.SessionMember{}, &model.LLMProvider{}, &model.LLMModel{},
	); err != nil {
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
	_, err := svc.ReplacePillComposition(context.Background(), agent.UUID, []service.PillCompositionItem{
		{PillUUID: pills[0].UUID, Weight: 1},
		{PillUUID: pills[0].UUID, Weight: 2},
	})
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
	_, err := svc.ReplacePillComposition(context.Background(), agent.UUID, []service.PillCompositionItem{
		{PillUUID: pills[0].UUID, Weight: 1},
		{PillUUID: uuid.New(), Weight: 1}, // 不存在
	})
	assertErrType(t, err, errors.ErrorTypeRecordNotFound, "金丹不存在")
}

func TestReplacePillComposition_WeightOutOfRange(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, pills := seedAgentAndPills(t, db, 1)
	for _, w := range []float64{0, -1, 10.5, 11} {
		_, err := svc.ReplacePillComposition(context.Background(), agent.UUID, []service.PillCompositionItem{
			{PillUUID: pills[0].UUID, Weight: w},
		})
		assertErrType(t, err, errors.ErrorTypeInvalidRequest, fmt.Sprintf("权重 %v 越界", w))
	}
}

func TestReplacePillComposition_AgentNotFound(t *testing.T) {
	svc, _ := setupServiceTestDB(t)
	_, err := svc.ReplacePillComposition(context.Background(), uuid.New(), nil)
	assertErrType(t, err, errors.ErrorTypeRecordNotFound, "道人不存在的 UUID")
}

// seedEnabledModel 造一个已启用供应商下的已启用模型
func seedEnabledModel(t *testing.T, db *gorm.DB, modelName string) {
	t.Helper()
	prov := &model.LLMProvider{Name: uuid.NewString(), DisplayName: "P", BaseURL: "http://x", IsEnabled: true}
	if err := db.Create(prov).Error; err != nil {
		t.Fatalf("建供应商失败: %v", err)
	}
	mdl := &model.LLMModel{ProviderID: prov.ID, Name: modelName, DisplayName: modelName, IsEnabled: true}
	if err := db.Create(mdl).Error; err != nil {
		t.Fatalf("建模型失败: %v", err)
	}
}

// seedSingleSession 造一个直挂道人的单聊会话
func seedSingleSession(t *testing.T, db *gorm.DB, agentID uint) {
	t.Helper()
	sess := &model.ChatSession{UUID: uuid.New(), Type: model.SessionTypeSingle, AgentID: &agentID}
	if err := db.Create(sess).Error; err != nil {
		t.Fatalf("建单聊会话失败: %v", err)
	}
}

// seedGroupMembership 造一个群聊会话并把道人拉进成员表
func seedGroupMembership(t *testing.T, db *gorm.DB, agentID uint) {
	t.Helper()
	sess := &model.ChatSession{UUID: uuid.New(), Type: model.SessionTypeGroup}
	if err := db.Create(sess).Error; err != nil {
		t.Fatalf("建群聊会话失败: %v", err)
	}
	if err := db.Create(&model.SessionMember{SessionID: sess.ID, AgentID: agentID, SortOrder: 0}).Error; err != nil {
		t.Fatalf("拉群成员失败: %v", err)
	}
}

func TestDeleteAgentWithSingleChatHistoryReturnsConflict(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, _ := seedAgentAndPills(t, db, 0)
	seedSingleSession(t, db, agent.ID)

	err := svc.DeleteAgent(context.Background(), agent.UUID)
	if err == nil {
		t.Fatal("有单聊历史时删除应返回冲突错误,实际为 nil")
	}
	assertErrType(t, err, errors.ErrorTypeConflict, "有历史删除")
	if err.GetCode() != "service.agent.delete_has_history" {
		t.Fatalf("错误码 = %s, 期望 service.agent.delete_has_history", err.GetCode())
	}
	// 携带 session_count 供前端提示
	ed, ok := err.(errors.ErrorWithData)
	if !ok {
		t.Fatalf("冲突错误应携带 session_count 数据,实际无 data")
	}
	data, _ := ed.GetData().(map[string]any)
	if data["session_count"] != int64(1) {
		t.Fatalf("session_count = %v(%T), 期望 1", ed.GetData(), ed.GetData())
	}
	// 道人未被删除
	var count int64
	db.Model(&model.DaoAgent{}).Where("id = ?", agent.ID).Count(&count)
	if count != 1 {
		t.Fatal("有历史时道人不应被删除")
	}
}

func TestDeleteAgentWithGroupChatHistoryReturnsConflict(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, _ := seedAgentAndPills(t, db, 0)
	seedGroupMembership(t, db, agent.ID)

	err := svc.DeleteAgent(context.Background(), agent.UUID)
	assertErrType(t, err, errors.ErrorTypeConflict, "有群聊历史删除")
	if err.GetCode() != "service.agent.delete_has_history" {
		t.Fatalf("错误码 = %s, 期望 service.agent.delete_has_history", err.GetCode())
	}
	var count int64
	db.Model(&model.DaoAgent{}).Where("id = ?", agent.ID).Count(&count)
	if count != 1 {
		t.Fatal("有群聊历史时道人不应被删除")
	}
}

func TestDeleteAgentWithoutHistoryHardDeletes(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, _ := seedAgentAndPills(t, db, 0)
	if err := svc.DeleteAgent(context.Background(), agent.UUID); err != nil {
		t.Fatalf("无历史删除报错: %v", err)
	}
	var count int64
	db.Model(&model.DaoAgent{}).Where("id = ?", agent.ID).Count(&count)
	if count != 0 {
		t.Fatal("无历史道人应被硬删除")
	}
}

func TestUpdateAgentRejectsInvalidStatus(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, _ := seedAgentAndPills(t, db, 0)
	bogus := "bogus"
	_, err := svc.UpdateAgent(context.Background(), agent.UUID, nil, nil, nil, nil, &bogus, nil)
	assertErrType(t, err, errors.ErrorTypeInvalidRequest, "非法 status")
}

func TestUpdateAgentActivatingWithUnavailableFinalModelFails(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	// inactive 道人,其模型在库中不存在(不可用);直接落库绕过创建校验
	agent := &model.DaoAgent{UUID: uuid.New(), Name: "睡道人", ModelName: "ghost-model", Status: "inactive"}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("建道人失败: %v", err)
	}
	active := "active"
	// 不传 model_name,仅激活;最终模型仍是不可用的 ghost-model → 拒绝
	_, err := svc.UpdateAgent(context.Background(), agent.UUID, nil, nil, nil, nil, &active, nil)
	assertErrType(t, err, errors.ErrorTypeInvalidRequest, "激活但模型不可用")
	var reload model.DaoAgent
	db.First(&reload, agent.ID)
	if reload.Status != "inactive" {
		t.Fatal("校验失败时状态不应被改为 active")
	}
}

func TestUpdateAgentActiveAgentWithUnavailableExistingModelFails(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	// active 道人,其现存模型已不可用(后被禁用/删除);只改名也应被拒
	agent := &model.DaoAgent{UUID: uuid.New(), Name: "旧道人", ModelName: "ghost-model", Status: "active"}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("建道人失败: %v", err)
	}
	name := "改名"
	_, err := svc.UpdateAgent(context.Background(), agent.UUID, &name, nil, nil, nil, nil, nil)
	assertErrType(t, err, errors.ErrorTypeInvalidRequest, "active 道人模型不可用(未改 model_name)")
}

func TestUpdateAgentActivatingWithAvailableModelSucceeds(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	seedEnabledModel(t, db, "real-model")
	agent := &model.DaoAgent{UUID: uuid.New(), Name: "睡道人", ModelName: "real-model", Status: "inactive"}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("建道人失败: %v", err)
	}
	active := "active"
	updated, err := svc.UpdateAgent(context.Background(), agent.UUID, nil, nil, nil, nil, &active, nil)
	if err != nil {
		t.Fatalf("激活可用模型报错: %v", err)
	}
	if updated.Status != "active" {
		t.Fatalf("状态 = %s, 期望 active", updated.Status)
	}
}

// assertErrType 断言错误类型(穿透 Relation 链);err 为 nil 时明确失败而非 panic
func assertErrType(t *testing.T, err errors.Error, want errors.ErrorType, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: 期望返回错误,实际为 nil", what)
	}
	if !errors.IsType(err, want) {
		t.Fatalf("%s: 错误类型不匹配 (code=%s, msg=%s), 期望类型 %v", what, err.GetCode(), err.Error(), want)
	}
}
