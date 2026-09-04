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
	"github.com/alchemy-furnace/server/internal/service/pill_inventory_service"
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
		&model.AgentPillEffect{}, &model.PillItem{},
		&model.PillRecipe{}, &model.PillRecipeRevision{}, &model.PillOperation{},
		&model.FusionPreview{}, &model.PillStarterGrant{},
		&model.ChatSession{}, &model.SessionMember{}, &model.LLMProvider{}, &model.LLMModel{},
	); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	dao.DB = db
	t.Cleanup(func() { dao.DB = nil })
	return New(dao.NewAgentDao(), dao.NewModelDao(), pill_inventory_service.New(db, time.Now)), db
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

// ---------- 任务 3 Step C: 服务层不得保留绕过库存的写路径 ----------

// craftTestItem 经库存服务炼一枚可用金丹实例并返回实例 UUID
func craftTestItem(t *testing.T, db *gorm.DB, name string) uuid.UUID {
	t.Helper()
	inv := pill_inventory_service.New(db, time.Now)
	res, err := inv.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(),
		CraftOne:    true,
		Draft: service.RecipeDraft{
			Name: name, Description: "测试",
			SkillSchema: model.JSONMap{"expression_dna": map[string]any{"sentence_length": "mixed"}},
		},
	})
	if err != nil {
		t.Fatalf("炼制实例失败: %v", err)
	}
	if len(res.ItemIDs) != 1 {
		t.Fatalf("期望 1 枚实例, 实际 %d", len(res.ItemIDs))
	}
	return res.ItemIDs[0]
}

// TestReplacePillComposition_RemovedFromService 完整服丹编排已在服务层下线:
// 任意输入返回 410 pill.legacy_api_removed,且不产生任何绑定写入(防绕过库存)
func TestReplacePillComposition_RemovedFromService(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, pills := seedAgentAndPills(t, db, 2)
	detail, err := svc.ReplacePillComposition(context.Background(), agent.UUID, []service.PillCompositionItem{
		{PillUUID: pills[0].UUID, Weight: 2.5},
	})
	if err == nil {
		t.Fatal("ReplacePillComposition 应返回 410 gone, 实际 nil")
	}
	assertErrType(t, err, errors.ErrorTypeGone, "完整编排入口")
	if err.GetCode() != "pill.legacy_api_removed" {
		t.Fatalf("错误码 = %s, 期望 pill.legacy_api_removed", err.GetCode())
	}
	if detail != nil {
		t.Fatal("410 不应返回道人详情")
	}
	var count int64
	db.Model(&model.AgentPill{}).Count(&count)
	if count != 0 {
		t.Fatalf("410 不应产生任何绑定, 实际 %d 条", count)
	}
}

// TestReplacePillComposition_InvalidInputAlsoGone 非法输入/空数组一律 410,不再做旧校验
// (清空编排不是移除能力,改走 UnbindPill)
func TestReplacePillComposition_InvalidInputAlsoGone(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, _ := seedAgentAndPills(t, db, 0)
	for _, items := range [][]service.PillCompositionItem{
		{{PillUUID: uuid.New(), Weight: 1}},
		{{PillUUID: uuid.New(), Weight: 0}},
		{{PillUUID: uuid.New(), Weight: 1}, {PillUUID: uuid.New(), Weight: 1}},
		nil,
	} {
		_, err := svc.ReplacePillComposition(context.Background(), agent.UUID, items)
		assertErrType(t, err, errors.ErrorTypeGone, "非法输入也应 410")
	}
}

// ---------- 任务 3 Step C: 服用/移除/调权必须经过库存 ----------

// TestBindPillConsumesInventoryItem 服用走库存: 实例 available→consumed_by_agent,
// 生成能力快照(身份=实例 UUID,权重/顺序=请求值), EffectsRevision 递增, 缓存失效
func TestBindPillConsumesInventoryItem(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, _ := seedAgentAndPills(t, db, 0)
	itemID := craftTestItem(t, db, "服丹测试")
	// 预置有效缓存,服用后应失效
	if err := db.Create(&model.LanguagePattern{AgentID: agent.UUID.String(), SystemPrompt: "c", SourceFingerprint: "sha256:x", IsValid: true}).Error; err != nil {
		t.Fatalf("建缓存失败: %v", err)
	}

	if err := svc.BindPill(context.Background(), agent.UUID, itemID, 2.5, 3); err != nil {
		t.Fatalf("BindPill 报错: %v", err)
	}

	// 实例已消耗,去向可读
	var item model.PillItem
	if err := db.Where("uuid = ?", itemID).First(&item).Error; err != nil {
		t.Fatalf("查实例失败: %v", err)
	}
	if item.State != model.PillConsumedByAgent {
		t.Fatalf("实例状态 = %s, 期望 consumed_by_agent", item.State)
	}

	// 能力快照生成(身份=实例 UUID)
	var ef model.AgentPillEffect
	if err := db.Preload("Item").Where("agent_id = ?", agent.UUID.String()).First(&ef).Error; err != nil {
		t.Fatalf("查能力快照失败: %v", err)
	}
	if ef.Item.UUID != itemID {
		t.Fatalf("能力快照关联实例 = %s, 期望 %s", ef.Item.UUID, itemID)
	}
	if ef.Weight != 2.5 || ef.SortOrder != 3 {
		t.Fatalf("权重/顺序 = %v/%v, 期望 2.5/3", ef.Weight, ef.SortOrder)
	}

	// EffectsRevision 递增 + 缓存失效
	var reload model.DaoAgent
	db.First(&reload, agent.ID)
	if reload.EffectsRevision != 1 {
		t.Fatalf("effects_revision = %d, 期望 1", reload.EffectsRevision)
	}
	var pattern model.LanguagePattern
	db.Where("agent_id = ?", agent.UUID.String()).First(&pattern)
	if pattern.IsValid {
		t.Fatal("服用后语言模式缓存未被失效")
	}
}

// TestBindPillDuplicateItemRejected 同一实例二次服用(不同幂等键)返回 409,
// 不产生第二条能力;未知道人/未知实例返回 404
func TestBindPillDuplicateItemRejected(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, _ := seedAgentAndPills(t, db, 0)
	itemID := craftTestItem(t, db, "重复服丹")
	if err := svc.BindPill(context.Background(), agent.UUID, itemID, 1, 1); err != nil {
		t.Fatalf("首次服用报错: %v", err)
	}
	err := svc.BindPill(context.Background(), agent.UUID, itemID, 1, 2)
	assertErrType(t, err, errors.ErrorTypeConflict, "重复服用同实例")
	if err.GetCode() != "pill.not_available" {
		t.Fatalf("错误码 = %s, 期望 pill.not_available", err.GetCode())
	}
	var count int64
	db.Model(&model.AgentPillEffect{}).Count(&count)
	if count != 1 {
		t.Fatalf("重复服用后能力数 = %d, 期望 1", count)
	}
}

// TestBindPillUnknownTargets 未知实例/未知道人 → 404(服用事务内含校验)
func TestBindPillUnknownTargets(t *testing.T) {
	svc, _ := setupServiceTestDB(t)
	err := svc.BindPill(context.Background(), uuid.New(), uuid.New(), 1, 1)
	assertErrType(t, err, errors.ErrorTypeRecordNotFound, "未知道人")
}

// TestUnbindPillRemovesEffectOnly 移除能力: 软删(removed_at 保留历史),EffectsRevision 递增,
// 缓存失效;原实例保持 consumed_by_agent 不返还库存;二次移除 404
func TestUnbindPillRemovesEffectOnly(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, _ := seedAgentAndPills(t, db, 0)
	itemID := craftTestItem(t, db, "移除测试")
	if err := svc.BindPill(context.Background(), agent.UUID, itemID, 1, 1); err != nil {
		t.Fatalf("预置服用失败: %v", err)
	}
	// 预置有效缓存,移除后应失效
	if err := db.Create(&model.LanguagePattern{AgentID: agent.UUID.String(), SystemPrompt: "c", SourceFingerprint: "sha256:x", IsValid: true}).Error; err != nil {
		t.Fatalf("建缓存失败: %v", err)
	}

	if err := svc.UnbindPill(context.Background(), agent.UUID, itemID); err != nil {
		t.Fatalf("UnbindPill 报错: %v", err)
	}

	var ef model.AgentPillEffect
	if err := db.Where("agent_id = ?", agent.UUID.String()).First(&ef).Error; err != nil {
		t.Fatalf("查能力失败: %v", err)
	}
	if ef.RemovedAt == nil {
		t.Fatal("移除后 removed_at 应为非空(软删保留历史)")
	}
	// 实例不返还
	var item model.PillItem
	db.Where("uuid = ?", itemID).First(&item)
	if item.State != model.PillConsumedByAgent {
		t.Fatalf("移除能力后实例状态 = %s, 期望保持 consumed_by_agent", item.State)
	}
	// 版本递增(服用 1 + 移除 1)
	var reload model.DaoAgent
	db.First(&reload, agent.ID)
	if reload.EffectsRevision != 2 {
		t.Fatalf("effects_revision = %d, 期望 2", reload.EffectsRevision)
	}
	var pattern model.LanguagePattern
	db.Where("agent_id = ?", agent.UUID.String()).First(&pattern)
	if pattern.IsValid {
		t.Fatal("移除能力后语言模式缓存未被失效")
	}
	// 再次移除 → 404(无活跃能力)
	err := svc.UnbindPill(context.Background(), agent.UUID, itemID)
	assertErrType(t, err, errors.ErrorTypeRecordNotFound, "二次移除")
}

// TestUpdateAgentPillUpdatesEffect 调整权重/顺序走能力表(实例 UUID 标识),
// 递增 EffectsRevision + 失效缓存;未吸收实例 404
func TestUpdateAgentPillUpdatesEffect(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, _ := seedAgentAndPills(t, db, 0)
	itemID := craftTestItem(t, db, "调权测试")
	if err := svc.BindPill(context.Background(), agent.UUID, itemID, 1, 1); err != nil {
		t.Fatalf("预置服用失败: %v", err)
	}
	if err := db.Create(&model.LanguagePattern{AgentID: agent.UUID.String(), SystemPrompt: "c", SourceFingerprint: "sha256:x", IsValid: true}).Error; err != nil {
		t.Fatalf("建缓存失败: %v", err)
	}

	w, s := 2.0, 4
	if err := svc.UpdateAgentPill(context.Background(), agent.UUID, itemID, &w, &s); err != nil {
		t.Fatalf("UpdateAgentPill 报错: %v", err)
	}
	var ef model.AgentPillEffect
	if err := db.Where("agent_id = ?", agent.UUID.String()).First(&ef).Error; err != nil {
		t.Fatalf("查能力失败: %v", err)
	}
	if ef.Weight != 2 || ef.SortOrder != 4 {
		t.Fatalf("权重/顺序 = %v/%v, 期望 2/4", ef.Weight, ef.SortOrder)
	}
	var reload model.DaoAgent
	db.First(&reload, agent.ID)
	if reload.EffectsRevision != 2 {
		t.Fatalf("effects_revision = %d, 期望 2", reload.EffectsRevision)
	}
	var pattern model.LanguagePattern
	db.Where("agent_id = ?", agent.UUID.String()).First(&pattern)
	if pattern.IsValid {
		t.Fatal("调整权重后语言模式缓存未被失效")
	}
	// 未吸收实例 → 404
	err := svc.UpdateAgentPill(context.Background(), agent.UUID, uuid.New(), &w, &s)
	assertErrType(t, err, errors.ErrorTypeRecordNotFound, "未知实例调权")
}

// seedEnabledModel 造一个已启用供应商下的已启用模型
func seedEnabledModel(t *testing.T, db *gorm.DB, modelName string) {
	t.Helper()
	prov := &model.LLMProvider{Name: uuid.NewString(), DisplayName: "P", BaseURL: "http://x", IsEnabled: true}
	if err := db.Create(prov).Error; err != nil {
		t.Fatalf("建供应商失败: %v", err)
	}
	mdl := &model.LLMModel{ProviderID: prov.UUID.String(), Name: modelName, DisplayName: modelName, IsEnabled: true}
	if err := db.Create(mdl).Error; err != nil {
		t.Fatalf("建模型失败: %v", err)
	}
}

// seedSingleSession 造一个直挂道人的单聊会话(关系键=道人UUID文本)
func seedSingleSession(t *testing.T, db *gorm.DB, agentUID string) {
	t.Helper()
	sess := &model.ChatSession{UUID: uuid.New(), Type: model.SessionTypeSingle, AgentID: &agentUID}
	if err := db.Create(sess).Error; err != nil {
		t.Fatalf("建单聊会话失败: %v", err)
	}
}

// seedGroupMembership 造一个群聊会话并把道人拉进成员表(关系键=UUID文本)
func seedGroupMembership(t *testing.T, db *gorm.DB, agentUID string) {
	t.Helper()
	sess := &model.ChatSession{UUID: uuid.New(), Type: model.SessionTypeGroup}
	if err := db.Create(sess).Error; err != nil {
		t.Fatalf("建群聊会话失败: %v", err)
	}
	if err := db.Create(&model.SessionMember{SessionID: sess.UUID.String(), AgentID: agentUID, SortOrder: 0}).Error; err != nil {
		t.Fatalf("拉群成员失败: %v", err)
	}
}

func TestDeleteAgentWithSingleChatHistoryReturnsConflict(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	agent, _ := seedAgentAndPills(t, db, 0)
	seedSingleSession(t, db, agent.UUID.String())

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
	seedGroupMembership(t, db, agent.UUID.String())

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
	_, err := svc.UpdateAgent(context.Background(), agent.UUID, nil, nil, nil, nil, &bogus, nil, nil)
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
	_, err := svc.UpdateAgent(context.Background(), agent.UUID, nil, nil, nil, nil, &active, nil, nil)
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
	_, err := svc.UpdateAgent(context.Background(), agent.UUID, &name, nil, nil, nil, nil, nil, nil)
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
	updated, err := svc.UpdateAgent(context.Background(), agent.UUID, nil, nil, nil, nil, &active, nil, nil)
	if err != nil {
		t.Fatalf("激活可用模型报错: %v", err)
	}
	if updated.Status != "active" {
		t.Fatalf("状态 = %s, 期望 active", updated.Status)
	}
}

func TestUpdateAgentMemoryEnabled(t *testing.T) {
	svc, db := setupServiceTestDB(t)
	seedEnabledModel(t, db, "real-model")
	agent := &model.DaoAgent{UUID: uuid.New(), Name: "记忆道人", ModelName: "real-model", Status: "active"}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("建道人失败: %v", err)
	}

	// 默认启用(true)
	got, err := svc.UpdateAgent(context.Background(), agent.UUID, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("首次更新报错: %v", err)
	}
	if !got.MemoryEnabled {
		t.Fatal("默认 MemoryEnabled 应为 true")
	}

	// 显式关闭 → 落库
	disabled := false
	updated, err := svc.UpdateAgent(context.Background(), agent.UUID, nil, nil, nil, nil, nil, nil, &disabled)
	if err != nil {
		t.Fatalf("关闭记忆报错: %v", err)
	}
	if updated.MemoryEnabled {
		t.Fatal("关闭后响应 MemoryEnabled 应为 false")
	}
	var reload model.DaoAgent
	db.First(&reload, agent.ID)
	if reload.MemoryEnabled {
		t.Fatal("关闭后落库 MemoryEnabled 应为 false")
	}

	// 显式开启 → 落库
	enabled := true
	updated, err = svc.UpdateAgent(context.Background(), agent.UUID, nil, nil, nil, nil, nil, nil, &enabled)
	if err != nil {
		t.Fatalf("开启记忆报错: %v", err)
	}
	if !updated.MemoryEnabled {
		t.Fatal("开启后响应 MemoryEnabled 应为 true")
	}
	db.First(&reload, agent.ID)
	if !reload.MemoryEnabled {
		t.Fatal("开启后落库 MemoryEnabled 应为 true")
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
