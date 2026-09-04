// 任务 7 测试：全链路生命周期 + 故障矩阵 + 旧行为回归
// 覆盖：单条生命周期（炼丹→服用→再炼→融合，库存/能力去向全程断言）、
// 重开 SQLite 重跑种子/赠送库存不复活、三处写失败点 trigger 回滚
// （效果插入在 consume_test.go TestConsumeTriggerRejection，此处补产物插入与
// operation 结果写入）、双连接并发矩阵（两个 consume 抢一枚、同 preview 不同 key、
// 同 key 不同 payload；consume/fusion 抢一枚在 fusion_confirm_test.go 已覆盖）。
package pill_inventory_service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---------- 辅助 ----------

// countRows 表行数（回滚断言：失败后任何步骤不得残留）
func countRows(t *testing.T, db *gorm.DB, m any) int64 {
	t.Helper()
	var n int64
	if err := db.Model(m).Count(&n).Error; err != nil {
		t.Fatalf("统计行数失败: %v", err)
	}
	return n
}

// countItemsByState 按状态统计实例
func countItemsByState(t *testing.T, db *gorm.DB, state model.PillItemState) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.PillItem{}).Where("state = ?", state).Count(&n).Error; err != nil {
		t.Fatalf("统计 state=%s 失败: %v", state, err)
	}
	return n
}

// countAvailableForRevision 某丹方版本的可用实例数（A 丹方库存 = 其版本下的 available 数）
func countAvailableForRevision(t *testing.T, db *gorm.DB, revUUID uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.PillItem{}).
		Joins("JOIN pill_recipe_revisions r ON r.uuid = pill_items.recipe_revision_id").
		Where("r.uuid = ? AND pill_items.state = ?", revUUID.String(), model.PillAvailable).
		Count(&n).Error; err != nil {
		t.Fatalf("统计版本库存失败: %v", err)
	}
	return n
}

// loadItem 按实例 UUID 读行
func loadItem(t *testing.T, db *gorm.DB, uid uuid.UUID) *model.PillItem {
	t.Helper()
	var it model.PillItem
	if err := db.Where("uuid = ?", uid.String()).First(&it).Error; err != nil {
		t.Fatalf("查实例 %s 失败: %v", uid, err)
	}
	return &it
}

// ---------- 1) 单条生命周期 ----------

// TestLifecycleConsumeThenCraftThenFusion 单条生命周期：SaveRecipe A（含 1 枚实例）→
// 服用 → A 库存 0、能力 1 → CraftOne 再得 C → 建 B（含 1 枚）→ 预览/确认融合 C+B →
// 断言 B/C 全部 consumed_by_fusion、产物 1 枚、道人能力不变（融合不触碰能力）
func TestLifecycleConsumeThenCraftThenFusion(t *testing.T) {
	svc, db := newTestSvc(t)
	agent := newTestAgent(t, db)
	ctx := context.Background()

	// 1) 丹方 A + 1 枚实例
	aRes, err := svc.SaveRecipe(ctx, service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("甲丹方", minSchema()),
	})
	if err != nil {
		t.Fatalf("SaveRecipe A: %v", err)
	}
	aItem := aRes.ItemIDs[0]
	if got := countAvailableForRevision(t, db, *aRes.RevisionID); got != 1 {
		t.Fatalf("A 初始库存=%d, want 1", got)
	}

	// 2) 服用 A 实例：A 库存 0、能力 1
	consumeRes, err := svc.Consume(ctx, service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: agent.UUID, ItemID: aItem, Weight: 2, SortOrder: 1,
	})
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got := countAvailableForRevision(t, db, *aRes.RevisionID); got != 0 {
		t.Fatalf("服用后 A 库存=%d, want 0", got)
	}
	if got := countRows(t, db, &model.AgentPillEffect{}); got != 1 {
		t.Fatalf("服用后能力=%d, want 1", got)
	}
	if loadItem(t, db, aItem).State != model.PillConsumedByAgent {
		t.Fatal("服用实例应进入 consumed_by_agent 终态")
	}
	effectID := *consumeRes.EffectID

	// 3) 按 A 版本再炼一枚 C（能力独立于库存，可再按丹方炼制）
	cRes, err := svc.CraftOne(ctx, service.CraftPillRequest{OperationID: uuid.New(), RevisionID: *aRes.RevisionID})
	if err != nil {
		t.Fatalf("CraftOne C: %v", err)
	}
	cItem := cRes.ItemIDs[0]
	if got := countAvailableForRevision(t, db, *aRes.RevisionID); got != 1 {
		t.Fatalf("再炼后 A 库存=%d, want 1", got)
	}

	// 4) 丹方 B + 1 枚实例
	bRes, err := svc.SaveRecipe(ctx, service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("乙丹方", minSchema()),
	})
	if err != nil {
		t.Fatalf("SaveRecipe B: %v", err)
	}
	bItem := bRes.ItemIDs[0]

	// 5) 预览 [C, B] 并确认融合
	pID := seedFusionPreview(t, db, svc, cItem, bItem)
	confRes, err := svc.ConfirmFusion(ctx, service.ConfirmFusionRequest{
		OperationID: uuid.New(), PreviewID: pID, Name: "丙丹方", Description: "",
	})
	if err != nil {
		t.Fatalf("ConfirmFusion: %v", err)
	}

	// 断言：B/C 全部 consumed_by_fusion、产物 1 枚 available、能力仍 1 且快照不变
	if got := loadItem(t, db, cItem).State; got != model.PillConsumedByFusion {
		t.Fatalf("C 状态=%s, want consumed_by_fusion", got)
	}
	if got := loadItem(t, db, bItem).State; got != model.PillConsumedByFusion {
		t.Fatalf("B 状态=%s, want consumed_by_fusion", got)
	}
	if got := countItemsByState(t, db, model.PillConsumedByFusion); got != 2 {
		t.Fatalf("consumed_by_fusion=%d, want 2", got)
	}
	if got := countItemsByState(t, db, model.PillAvailable); got != 1 {
		t.Fatalf("可用库存=%d, want 1（仅融合产物）", got)
	}
	if len(confRes.ItemIDs) != 1 {
		t.Fatalf("融合产物=%d, want 1", len(confRes.ItemIDs))
	}
	var effects []model.AgentPillEffect
	if err := db.Find(&effects).Error; err != nil {
		t.Fatal(err)
	}
	if len(effects) != 1 || effects[0].UUID != effectID {
		t.Fatalf("融合后能力=%d 且 UUID=%v, want 1 且 %v（融合不触碰能力）", len(effects), effects[0].UUID, effectID)
	}
	if loadPreview(t, db, pID).ConfirmedOperationID == nil {
		t.Fatal("预览应绑定成功操作")
	}
}

// ---------- 2) 重开 SQLite 库存不复活 ----------

// TestReopenSQLiteInventoryPersists 关闭并重开 SQLite，重跑种子与赠送：
// 种子查重不重写、启动赠送凭 PillStarterGrant 持久化标记不再补货（赠送幂等）。
// 这是防止「重启自动复活」的必测项：已消耗实例保持终态，可用库存数量不变。
func TestReopenSQLiteInventoryPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")
	fixed := func() time.Time { return confirmFixedNow }

	// 第一程：完整启动链（内置丹方种子 → 一次性赠送）
	db1 := openInventoryDBAt(t, path)
	svc1 := New(db1, fixed)
	if err := dao.SeedBuiltinRecipes(db1); err != nil {
		t.Fatalf("内置种子: %v", err)
	}
	if err := dao.GrantStarterPills(db1); err != nil {
		t.Fatalf("启动赠送: %v", err)
	}
	// 服用一枚内置赠送金丹
	agent := newTestAgent(t, db1)
	var first model.PillItem
	if err := db1.Where("state = ?", model.PillAvailable).First(&first).Error; err != nil {
		t.Fatalf("找赠送金丹: %v", err)
	}
	if _, err := svc1.Consume(context.Background(), service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: agent.UUID, ItemID: first.UUID, Weight: 1, SortOrder: 1,
	}); err != nil {
		t.Fatalf("服用: %v", err)
	}
	availBefore := countItemsByState(t, db1, model.PillAvailable)
	consumedBefore := countItemsByState(t, db1, model.PillConsumedByAgent)
	opsBefore := countRows(t, db1, &model.PillOperation{})
	if availBefore == 0 || consumedBefore != 1 {
		t.Fatalf("首程基线异常: available=%d consumed=%d", availBefore, consumedBefore)
	}

	// 关闭连接
	raw, err := db1.DB()
	if err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	// 第二程：重开 + 重跑同一链（种子查重不重写、赠送凭 PillStarterGrant 标记不补货）
	db2 := openInventoryDBAt(t, path)
	if err := dao.SeedBuiltinRecipes(db2); err != nil {
		t.Fatalf("二次种子: %v", err)
	}
	if err := dao.GrantStarterPills(db2); err != nil {
		t.Fatalf("二次赠送: %v", err)
	}

	if got := countItemsByState(t, db2, model.PillAvailable); got != availBefore {
		t.Fatalf("重开后可用库存=%d, want %d（重启不得自动补货）", got, availBefore)
	}
	if got := countItemsByState(t, db2, model.PillConsumedByAgent); got != consumedBefore {
		t.Fatalf("重开后已服用=%d, want %d（终态保留）", got, consumedBefore)
	}
	if got := countRows(t, db2, &model.PillOperation{}); got != opsBefore {
		t.Fatalf("重开后操作行=%d, want %d（不新增补货操作）", got, opsBefore)
	}
	if got := countRows(t, db2, &model.AgentPillEffect{}); got != 1 {
		t.Fatalf("重开后能力=%d, want 1", got)
	}
}

// ---------- 3) 失败点 trigger 回滚 ----------
// 效果插入回滚已由 consume_test.go TestConsumeTriggerRejection 覆盖；
// 此处补产物插入（融合第二失败点）与 operation 结果写入（第三失败点）。

// TestConfirmFusionTriggerRejectionOnItemInsert 产物 INSERT 被 trigger 拒绝：
// 材料 CAS、新丹方/版本、预览绑定、operation 结果全部随事务回滚；
// 移除 trigger 后同 key 重试成功一次（断线重试语义）
func TestConfirmFusionTriggerRejectionOnItemInsert(t *testing.T) {
	svc, db := newTestSvc(t)
	ctx := context.Background()
	aID, bID := seedTwoItemsSameRecipe(t, svc)
	pID := seedFusionPreview(t, db, svc, aID, bID)
	recipesBefore := countRows(t, db, &model.PillRecipe{})
	opsBefore := countRows(t, db, &model.PillOperation{})

	if err := db.Exec(`CREATE TRIGGER reject_item_insert BEFORE INSERT ON pill_items
		BEGIN SELECT RAISE(ABORT, 'no item'); END`).Error; err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	if _, err := svc.ConfirmFusion(ctx, service.ConfirmFusionRequest{
		OperationID: key, PreviewID: pID, Name: "产物插入失败", Description: "",
	}); err == nil {
		t.Fatal("产物插入被拒时 ConfirmFusion 应失败")
	}

	// 整体回滚：材料不部分消耗、无新丹方、预览未绑定、无操作行
	if got := loadItem(t, db, aID).State; got != model.PillAvailable {
		t.Fatalf("回滚后 A=%s, want available（材料不部分消耗）", got)
	}
	if got := loadItem(t, db, bID).State; got != model.PillAvailable {
		t.Fatalf("回滚后 B=%s, want available", got)
	}
	if got := countRows(t, db, &model.PillRecipe{}); got != recipesBefore {
		t.Fatalf("回滚后丹方=%d, want %d（新丹方不得残留）", got, recipesBefore)
	}
	if loadPreview(t, db, pID).ConfirmedOperationID != nil {
		t.Fatal("回滚后预览不得绑定")
	}
	if got := countRows(t, db, &model.PillOperation{}); got != opsBefore {
		t.Fatalf("回滚后操作行=%d, want %d（确认事务零残留；不含 setup 行）", got, opsBefore)
	}

	// 移除 trigger，同 key 重试成功一次
	if err := db.Exec("DROP TRIGGER reject_item_insert").Error; err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmFusion(ctx, service.ConfirmFusionRequest{
		OperationID: key, PreviewID: pID, Name: "产物插入失败", Description: "",
	})
	if err != nil {
		t.Fatalf("移除 trigger 后同 key 重试应成功: %v", err)
	}
	if len(res.ItemIDs) != 1 {
		t.Fatalf("重试产物=%d, want 1", len(res.ItemIDs))
	}
	if got := countItemsByState(t, db, model.PillConsumedByFusion); got != 2 {
		t.Fatalf("重试后 consumed_by_fusion=%d, want 2", got)
	}
}

// TestOperationResultWriteTriggerRollsBack operation 结果写入被 trigger 拒绝
// （成功提交点）：占位、丹方、版本、实例全部回滚；同 key 重试前无任何残留；
// 移除 trigger 后同 key 重试恰好成功一次
func TestOperationResultWriteTriggerRollsBack(t *testing.T) {
	svc, db := newTestSvc(t)
	ctx := context.Background()

	if err := db.Exec(`CREATE TRIGGER reject_op_result BEFORE UPDATE ON pill_operations
		BEGIN SELECT RAISE(ABORT, 'no result'); END`).Error; err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	if _, err := svc.SaveRecipe(ctx, service.SaveRecipeRequest{
		OperationID: key, CraftOne: true, Draft: draftOf("写结果失败", minSchema()),
	}); err == nil {
		t.Fatal("结果写入被拒时 SaveRecipe 应失败")
	}

	// 占位与业务同事务：失败后零残留（同 key 重试不受任何脏占位影响）
	if got := countRows(t, db, &model.PillRecipe{}); got != 0 {
		t.Fatalf("回滚后丹方=%d, want 0", got)
	}
	if got := countRows(t, db, &model.PillRecipeRevision{}); got != 0 {
		t.Fatalf("回滚后版本=%d, want 0", got)
	}
	if got := countRows(t, db, &model.PillItem{}); got != 0 {
		t.Fatalf("回滚后实例=%d, want 0", got)
	}
	if got := countRows(t, db, &model.PillOperation{}); got != 0 {
		t.Fatalf("回滚后操作行=%d, want 0", got)
	}

	// 移除 trigger，同 key 重试成功一次
	if err := db.Exec("DROP TRIGGER reject_op_result").Error; err != nil {
		t.Fatal(err)
	}
	res, err := svc.SaveRecipe(ctx, service.SaveRecipeRequest{
		OperationID: key, CraftOne: true, Draft: draftOf("写结果失败", minSchema()),
	})
	if err != nil {
		t.Fatalf("移除 trigger 后同 key 重试应成功: %v", err)
	}
	if len(res.ItemIDs) != 1 {
		t.Fatalf("重试产物=%d, want 1", len(res.ItemIDs))
	}
	if got := countRows(t, db, &model.PillItem{}); got != 1 {
		t.Fatalf("重试后实例=%d, want 1（只落一次）", got)
	}
	if got := countRows(t, db, &model.PillOperation{}); got != 1 {
		t.Fatalf("重试后操作行=%d, want 1", got)
	}
}

// ---------- 4) 双连接并发矩阵 ----------
// consume/fusion 抢一枚由 fusion_confirm_test.go TestConsumeFusionRaceSingleWinner 覆盖。

// TestTwoConsumeRaceSingleItem 双连接：两个不同 key 的 consume 抢同一枚 → 恰一个成功，
// 能力/终态/EffectsRevision 只落一次
func TestTwoConsumeRaceSingleItem(t *testing.T) {
	path := t.TempDir() + "/two-consume.db"
	db1 := openInventoryDBAt(t, path)
	svc1 := New(db1, func() time.Time { return confirmFixedNow })
	agent := seedFusionAgent(t, db1)
	saved, err := svc1.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("抢丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	itemID := saved.ItemIDs[0]

	db2 := openInventoryDBAt(t, path)
	svc2 := New(db2, func() time.Time { return confirmFixedNow })

	ch1 := make(chan errors.Error, 1)
	ch2 := make(chan errors.Error, 1)
	go func() {
		_, err := svc1.Consume(context.Background(), service.ConsumePillRequest{
			OperationID: uuid.New(), AgentID: agent, ItemID: itemID, Weight: 1, SortOrder: 1,
		})
		ch1 <- err
	}()
	go func() {
		_, err := svc2.Consume(context.Background(), service.ConsumePillRequest{
			OperationID: uuid.New(), AgentID: agent, ItemID: itemID, Weight: 2, SortOrder: 2,
		})
		ch2 <- err
	}()
	err1, err2 := <-ch1, <-ch2
	if (err1 == nil) == (err2 == nil) {
		t.Fatalf("期望恰一个消费方成功: c1=%v c2=%v", err1, err2)
	}

	if got := countRows(t, db1, &model.AgentPillEffect{}); got != 1 {
		t.Fatalf("能力=%d, want 1（只落一次）", got)
	}
	if got := loadItem(t, db1, itemID).State; got != model.PillConsumedByAgent {
		t.Fatalf("实例状态=%s, want consumed_by_agent", got)
	}
	var ag model.DaoAgent
	if err := db1.First(&ag, "uuid = ?", agent.String()).Error; err != nil {
		t.Fatal(err)
	}
	if ag.EffectsRevision != 1 {
		t.Fatalf("effects_revision=%d, want 1", ag.EffectsRevision)
	}
}

// TestSamePreviewTwoConnectionsSingleConfirm 双连接：同 preview 不同 key 并发确认 →
// 恰一个成功；终态一致（材料全部 consumed_by_fusion、产物 1 枚、预览绑定一次）
func TestSamePreviewTwoConnectionsSingleConfirm(t *testing.T) {
	path := t.TempDir() + "/same-preview.db"
	db1 := openInventoryDBAt(t, path)
	svc1 := New(db1, func() time.Time { return confirmFixedNow })
	aID, bID := seedTwoItemsSameRecipe(t, svc1)
	pID := seedFusionPreview(t, db1, svc1, aID, bID)

	db2 := openInventoryDBAt(t, path)
	svc2 := New(db2, func() time.Time { return confirmFixedNow })

	ch1 := make(chan errors.Error, 1)
	ch2 := make(chan errors.Error, 1)
	go func() {
		_, err := svc1.ConfirmFusion(context.Background(), service.ConfirmFusionRequest{
			OperationID: uuid.New(), PreviewID: pID, Name: "并发确认甲", Description: "",
		})
		ch1 <- err
	}()
	go func() {
		_, err := svc2.ConfirmFusion(context.Background(), service.ConfirmFusionRequest{
			OperationID: uuid.New(), PreviewID: pID, Name: "并发确认乙", Description: "",
		})
		ch2 <- err
	}()
	err1, err2 := <-ch1, <-ch2
	if (err1 == nil) == (err2 == nil) {
		t.Fatalf("期望恰一个确认成功: c1=%v c2=%v", err1, err2)
	}

	if got := countItemsByState(t, db1, model.PillConsumedByFusion); got != 2 {
		t.Fatalf("consumed_by_fusion=%d, want 2（材料全部消耗）", got)
	}
	if got := countItemsByState(t, db1, model.PillAvailable); got != 1 {
		t.Fatalf("available=%d, want 1（仅产物；不部分消耗）", got)
	}
	if got := countRows(t, db1, &model.PillRecipe{}); got != 2 {
		t.Fatalf("丹方=%d, want 2（原料丹 + 融合丹，各一次）", got)
	}
	if loadPreview(t, db1, pID).ConfirmedOperationID == nil {
		t.Fatal("预览应绑定成功操作")
	}
}

// TestSameKeyDifferentPayloadTwoConnections 双连接：同一 key 不同 payload 并发 →
// 恰一个成功；失败方 409 负载不一致（pill.operation_payload_mismatch），业务只落一次
func TestSameKeyDifferentPayloadTwoConnections(t *testing.T) {
	path := t.TempDir() + "/same-key.db"
	db1 := openInventoryDBAt(t, path)
	svc1 := New(db1, func() time.Time { return confirmFixedNow })
	agent := seedFusionAgent(t, db1)
	saved, err := svc1.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("同键丹", minSchema()),
	})
	if err != nil {
		t.Fatal(err)
	}
	itemID := saved.ItemIDs[0]

	db2 := openInventoryDBAt(t, path)
	svc2 := New(db2, func() time.Time { return confirmFixedNow })

	key := uuid.New()
	ch1 := make(chan errors.Error, 1)
	ch2 := make(chan errors.Error, 1)
	go func() {
		_, err := svc1.Consume(context.Background(), service.ConsumePillRequest{
			OperationID: key, AgentID: agent, ItemID: itemID, Weight: 1, SortOrder: 1,
		})
		ch1 <- err
	}()
	go func() {
		_, err := svc2.Consume(context.Background(), service.ConsumePillRequest{
			OperationID: key, AgentID: agent, ItemID: itemID, Weight: 2, SortOrder: 2,
		})
		ch2 <- err
	}()
	err1, err2 := <-ch1, <-ch2

	successes := 0
	var loser errors.Error
	for _, e := range []errors.Error{err1, err2} {
		if e == nil {
			successes++
		} else {
			loser = e
		}
	}
	if successes != 1 {
		t.Fatalf("期望恰一个成功: c1=%v c2=%v", err1, err2)
	}
	if loser.GetCode() != "pill.operation_payload_mismatch" {
		t.Fatalf("失败方 code=%s, want pill.operation_payload_mismatch", loser.GetCode())
	}

	if got := countRows(t, db1, &model.AgentPillEffect{}); got != 1 {
		t.Fatalf("能力=%d, want 1（只落一次）", got)
	}
	// 只数 consume 操作：同一 key 只提交一次（setup 的 SaveRecipe 另有一行，不计）
	var opCount int64
	if err := db1.Model(&model.PillOperation{}).Where("kind = ?", "consume").Count(&opCount).Error; err != nil {
		t.Fatal(err)
	}
	if opCount != 1 {
		t.Fatalf("consume 操作行=%d, want 1（同一 key 只提交一次）", opCount)
	}
	if got := loadItem(t, db1, itemID).State; got != model.PillConsumedByAgent {
		t.Fatalf("实例状态=%s, want consumed_by_agent", got)
	}
}
