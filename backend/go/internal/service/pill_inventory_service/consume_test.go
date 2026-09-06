// 任务 3 测试：服用事务（§3.2）
// 覆盖：正常消耗（实例终态+快照+EffectsRevision+缓存失效）、同 key 幂等重试、
// 不同 key 抢同实例 409、失败注入全回滚、同版本活跃能力唯一约束 409、
// 归档丹方已有实例可服、默认权重/顺序
package pill_inventory_service

import (
	"context"
	"reflect"
	"testing"

	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// newTestAgent 建测试道人（fixture 含 dao_agents/language_patterns 表）
func newTestAgent(t *testing.T, db *gorm.DB) *model.DaoAgent {
	t.Helper()
	agent := &model.DaoAgent{Name: "测试道人", Personality: "冷静沉稳，惜字如金"}
	if err := db.Create(agent).Error; err != nil {
		t.Fatal(err)
	}
	return agent
}

// craftOneOf 按版本炼制一枚并返回实例 UUID
func craftOneOf(t *testing.T, svc *Inventory, revID uuid.UUID) uuid.UUID {
	t.Helper()
	res, err := svc.CraftOne(context.Background(), service.CraftPillRequest{
		OperationID: uuid.New(), RevisionID: revID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res.ItemIDs[0]
}

// ---------- 正常消耗 ----------

// TestConsumeHappyPath 服用成功：实例终态、快照深拷贝、EffectsRevision 递增、缓存失效、去向可读
func TestConsumeHappyPath(t *testing.T) {
	svc, db := newTestSvc(t)
	agent := newTestAgent(t, db)
	// 预先存在的有效缓存，服用后必须同事务失效
	if err := db.Create(&model.LanguagePattern{
		AgentID: agent.DaoAgentID, SystemPrompt: "旧提示词", IsValid: true,
		SourceFingerprint: "sha256:old", ProfileVersion: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("服丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	itemID := saved.ItemIDs[0]

	res, err := svc.Consume(context.Background(), service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: uuid.MustParse(agent.DaoAgentID), ItemID: itemID, Weight: 2, SortOrder: 3,
	})
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if res.EffectID == nil {
		t.Fatal("应返回 effect_id")
	}
	if len(res.ConsumedItemIDs) != 1 || res.ConsumedItemIDs[0] != itemID {
		t.Fatalf("consumed_item_ids=%v, want [%v]", res.ConsumedItemIDs, itemID)
	}

	// 实例终态：consumed_by_agent + 时间 + 操作去向
	var item model.PillItem
	if err := db.Where("pill_item_id = ?", itemID.String()).First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.State != model.PillConsumedByAgent {
		t.Fatalf("state=%s, want consumed_by_agent", item.State)
	}
	if item.ConsumedAt == nil || item.ConsumeOperationID == nil {
		t.Fatalf("消耗去向缺失: consumed_at=%v op=%v", item.ConsumedAt, item.ConsumeOperationID)
	}

	// 能力快照：名称/完整 schema（含未知字段深拷贝）/权重/顺序
	var effects []model.AgentPillEffect
	if err := db.Where("agent_id = ?", agent.DaoAgentID).Find(&effects).Error; err != nil {
		t.Fatal(err)
	}
	if len(effects) != 1 {
		t.Fatalf("effects=%d, want 1", len(effects))
	}
	ef := effects[0]
	if ef.AgentPillEffectID != res.EffectID.String() {
		t.Fatalf("effect id 不一致: %v vs %v", ef.AgentPillEffectID, res.EffectID)
	}
	if ef.NameSnapshot != "服丹" {
		t.Fatalf("name_snapshot=%q", ef.NameSnapshot)
	}
	if ef.SchemaSnapshot["future_extension"] == nil {
		t.Fatalf("schema 快照丢失未知字段: %+v", ef.SchemaSnapshot)
	}
	if ef.Weight != 2 || ef.SortOrder != 3 {
		t.Fatalf("weight=%v sort=%d, want 2/3", ef.Weight, ef.SortOrder)
	}
	if ef.RemovedAt != nil {
		t.Fatal("新能力不应有 removed_at")
	}

	// EffectsRevision 递增 + 缓存失效（同事务）
	var ag model.DaoAgent
	if err := db.Where("dao_agent_id = ?", agent.DaoAgentID).First(&ag).Error; err != nil {
		t.Fatal(err)
	}
	if ag.EffectsRevision != 1 {
		t.Fatalf("effects_revision=%d, want 1", ag.EffectsRevision)
	}
	var lp model.LanguagePattern
	if err := db.Where("agent_id = ?", agent.DaoAgentID).First(&lp).Error; err != nil {
		t.Fatal(err)
	}
	if lp.IsValid {
		t.Fatal("服用后语言模式缓存应失效")
	}

	// 去向展示：已消耗实例仍可读（含版本内容）
	if _, err := svc.GetItem(context.Background(), itemID); err != nil {
		t.Fatalf("已消耗实例不可读: %v", err)
	}
}

// TestConsumeSecondKeySameItemFails 同一实例第二次服用（新 key）→ 409，无二次消耗
func TestConsumeSecondKeySameItemFails(t *testing.T) {
	svc, db := newTestSvc(t)
	agent := newTestAgent(t, db)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("服丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	itemID := saved.ItemIDs[0]
	req := service.ConsumePillRequest{OperationID: uuid.New(), AgentID: uuid.MustParse(agent.DaoAgentID), ItemID: itemID, Weight: 1, SortOrder: 1}
	if _, err := svc.Consume(context.Background(), req); err != nil {
		t.Fatalf("first: %v", err)
	}

	req.OperationID = uuid.New()
	_, err = svc.Consume(context.Background(), req)
	if err == nil {
		t.Fatal("同实例二次服用应 409")
	}
	if err.GetCode() != "pill.not_available" {
		t.Fatalf("code=%s, want pill.not_available", err.GetCode())
	}
	var n int64
	db.Model(&model.AgentPillEffect{}).Where("agent_id = ?", agent.DaoAgentID).Count(&n)
	if n != 1 {
		t.Fatalf("effects=%d, want 1", n)
	}
	var ag model.DaoAgent
	db.Where("dao_agent_id = ?", agent.DaoAgentID).First(&ag)
	if ag.EffectsRevision != 1 {
		t.Fatalf("effects_revision=%d, want 1", ag.EffectsRevision)
	}
}

// TestConsumeSameKeyRetry 同 key 重试返回原结果，不重复消耗
func TestConsumeSameKeyRetry(t *testing.T) {
	svc, db := newTestSvc(t)
	agent := newTestAgent(t, db)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("服丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	req := service.ConsumePillRequest{OperationID: key, AgentID: uuid.MustParse(agent.DaoAgentID), ItemID: saved.ItemIDs[0], Weight: 1, SortOrder: 1}
	first, err := svc.Consume(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Consume(context.Background(), req)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("重试结果不一致: %+v vs %+v", first, second)
	}
	var n int64
	db.Model(&model.AgentPillEffect{}).Count(&n)
	if n != 1 {
		t.Fatalf("effects=%d, want 1", n)
	}
}

// ---------- 失败注入：任何步骤失败整体回滚 ----------

// TestConsumeTriggerRejection 能力 INSERT 被 trigger 拒绝 → 库存/操作/EffectsRevision 全部不变；
// 移除 trigger 后同 key 重试可成功一次（断线重试语义）
func TestConsumeTriggerRejection(t *testing.T) {
	svc, db := newTestSvc(t)
	agent := newTestAgent(t, db)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("服丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	itemID := saved.ItemIDs[0]
	key := uuid.New()
	req := service.ConsumePillRequest{OperationID: key, AgentID: uuid.MustParse(agent.DaoAgentID), ItemID: itemID, Weight: 1, SortOrder: 1}

	if err := db.Exec(`
		CREATE TRIGGER reject_effect_insert BEFORE INSERT ON agent_pill_effects
		BEGIN SELECT RAISE(ABORT, 'test: effect insert rejected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Consume(context.Background(), req); err == nil {
		t.Fatal("trigger 拒绝时 Consume 应失败")
	}

	// 全部回滚：实例仍可用、无能力、无操作占位、revision 未动
	var item model.PillItem
	db.Where("pill_item_id = ?", itemID.String()).First(&item)
	if item.State != model.PillAvailable {
		t.Fatalf("失败后实例应回滚为 available, got %s", item.State)
	}
	var n int64
	db.Model(&model.AgentPillEffect{}).Count(&n)
	if n != 0 {
		t.Fatalf("失败后能力数=%d, want 0", n)
	}
	db.Model(&model.PillOperation{}).Where("pill_operation_id = ?", key.String()).Count(&n)
	if n != 0 {
		t.Fatalf("失败后 operation 应回滚（不可见空结果）, got %d", n)
	}
	var ag model.DaoAgent
	db.Where("dao_agent_id = ?", agent.DaoAgentID).First(&ag)
	if ag.EffectsRevision != 0 {
		t.Fatalf("失败后 effects_revision=%d, want 0", ag.EffectsRevision)
	}

	// 移除 trigger，同 key 重试成功一次
	if err := db.Exec("DROP TRIGGER reject_effect_insert").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Consume(context.Background(), req); err != nil {
		t.Fatalf("移除 trigger 后同 key 重试应成功: %v", err)
	}
	db.Model(&model.AgentPillEffect{}).Count(&n)
	if n != 1 {
		t.Fatalf("重试后能力数=%d, want 1", n)
	}
}

// ---------- 同版本活跃能力唯一约束 ----------

// TestConsumeDuplicateActiveEffect 同一道人同版本活跃能力已存在 → 409 pill.effect_already_active；
// 第二枚实例必须保持可用（整体回滚）
func TestConsumeDuplicateActiveEffect(t *testing.T) {
	svc, db := newTestSvc(t)
	agent := newTestAgent(t, db)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), Draft: draftOf("重复丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	itemA := craftOneOf(t, svc, *saved.RevisionID)
	itemB := craftOneOf(t, svc, *saved.RevisionID)

	if _, err := svc.Consume(context.Background(), service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: uuid.MustParse(agent.DaoAgentID), ItemID: itemA, Weight: 1, SortOrder: 1,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Consume(context.Background(), service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: uuid.MustParse(agent.DaoAgentID), ItemID: itemB, Weight: 1, SortOrder: 2,
	})
	if err == nil {
		t.Fatal("同版本活跃能力重复服用应 409")
	}
	if err.GetCode() != "pill.effect_already_active" {
		t.Fatalf("code=%s, want pill.effect_already_active", err.GetCode())
	}
	var item model.PillItem
	db.Where("pill_item_id = ?", itemB.String()).First(&item)
	if item.State != model.PillAvailable {
		t.Fatalf("被拒服用后第二枚应仍可用, got %s", item.State)
	}
	var n int64
	db.Model(&model.AgentPillEffect{}).Where("agent_id = ?", agent.DaoAgentID).Count(&n)
	if n != 1 {
		t.Fatalf("effects=%d, want 1", n)
	}
}

// ---------- 归档丹方不阻止已有实例服用 ----------

// TestConsumeArchivedRecipeItemStillWorks 归档后已有实例仍可服用（§3.2 步骤 1）
func TestConsumeArchivedRecipeItemStillWorks(t *testing.T) {
	svc, db := newTestSvc(t)
	agent := newTestAgent(t, db)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("归档服丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveRecipe(context.Background(), service.ArchiveRecipeRequest{
		OperationID: uuid.New(), RecipeID: *saved.RecipeID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Consume(context.Background(), service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: uuid.MustParse(agent.DaoAgentID), ItemID: saved.ItemIDs[0], Weight: 1, SortOrder: 1,
	}); err != nil {
		t.Fatalf("归档丹方已有实例应可服用: %v", err)
	}
	var item model.PillItem
	db.Where("pill_item_id = ?", saved.ItemIDs[0].String()).First(&item)
	if item.State != model.PillConsumedByAgent {
		t.Fatalf("state=%s, want consumed_by_agent", item.State)
	}
}

// ---------- 基础校验与默认值 ----------

// TestConsumeUnknownTargets 未知道人/未知实例 → 404
func TestConsumeUnknownTargets(t *testing.T) {
	svc, db := newTestSvc(t)
	agent := newTestAgent(t, db)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("服丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 未知道人
	_, err = svc.Consume(context.Background(), service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: uuid.New(), ItemID: saved.ItemIDs[0], Weight: 1, SortOrder: 1,
	})
	if err == nil || err.GetCode() != "agent.not_found" {
		t.Fatalf("未知道人: err=%v code=%s, want 404 agent.not_found", err, err.GetCode())
	}
	// 未知实例
	_, err = svc.Consume(context.Background(), service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: uuid.MustParse(agent.DaoAgentID), ItemID: uuid.New(), Weight: 1, SortOrder: 1,
	})
	if err == nil || err.GetCode() != "pill.not_found" {
		t.Fatalf("未知实例: err=%v code=%s, want 404 pill.not_found", err, err.GetCode())
	}
}

// TestConsumeDefaults weight<=0 回退 1.0；sort_order<=0 取当前最大+1
func TestConsumeDefaults(t *testing.T) {
	svc, db := newTestSvc(t)
	agent := newTestAgent(t, db)
	saved, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("默认丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Consume(context.Background(), service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: uuid.MustParse(agent.DaoAgentID), ItemID: saved.ItemIDs[0], Weight: 0, SortOrder: 0,
	}); err != nil {
		t.Fatal(err)
	}
	var ef model.AgentPillEffect
	db.Where("agent_id = ?", agent.DaoAgentID).First(&ef)
	if ef.Weight != 1.0 {
		t.Fatalf("weight=%v, want 1.0", ef.Weight)
	}
	if ef.SortOrder != 1 {
		t.Fatalf("sort_order=%d, want 1（max 0 + 1）", ef.SortOrder)
	}
}
