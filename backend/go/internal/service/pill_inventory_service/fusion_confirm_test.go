// 任务 4 测试：融合预览的原子确认（ConfirmFusion）
// 覆盖 plan 契约：原子产出（新丹方 v1 + 一枚新金丹 + 全部材料 consumed_by_fusion）、
// 部分失败整体回滚（A 仍 available、无产物）、同 key 幂等重试、不同 key 二次确认 409、
// 过期 410、已提交结果在预览过期后仍返回、双连接 consume/fusion 并发恰一个成功。
// 预览本身由 fusion_service 生成，本测试用 seedFusionPreview 直插预览行（模拟持久化结果）。
package pill_inventory_service

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// confirmFixedNow 与 newTestSvc 的固定时钟保持一致（2026-08-31 12:00 UTC）
var confirmFixedNow = time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

// seedFusionAgent 造一个道人（服用材料制造"预览外被消耗"场景）
func seedFusionAgent(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	agent := &model.DaoAgent{Name: "融合道人", ModelName: "gpt-4o", Status: "active"}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("建道人失败: %v", err)
	}
	return agent.UUID
}

// seedTwoItemsSameRecipe 同一丹方炼出两枚可用实例（同一版本两枚可融合，产品规则 8）
func seedTwoItemsSameRecipe(t *testing.T, svc *Inventory) (uuid.UUID, uuid.UUID) {
	t.Helper()
	first, err := svc.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true, Draft: draftOf("融合原料丹", minSchema()),
	})
	if err != nil {
		t.Fatalf("炼第一枚失败: %v", err)
	}
	second, err := svc.CraftOne(context.Background(), service.CraftPillRequest{
		OperationID: uuid.New(), RevisionID: *first.RevisionID,
	})
	if err != nil {
		t.Fatalf("炼第二枚失败: %v", err)
	}
	return first.ItemIDs[0], second.ItemIDs[0]
}

// seedFusionPreview 直插一条未确认预览（模拟 fusion_service 持久化的模型结果）
func seedFusionPreview(t *testing.T, db *gorm.DB, svc *Inventory, itemUUIDs ...uuid.UUID) uuid.UUID {
	t.Helper()
	ids := make(model.JSONList, len(itemUUIDs))
	for i, u := range itemUUIDs {
		ids[i] = u.String()
	}
	p := &model.FusionPreview{
		InputItemsJSON:   ids,
		InputHash:        FusionInputHash(itemUUIDs), // 与真实预览流程（fusion_service）同一哈希算法
		OutputJSON:       model.JSONMap{"name": "融合新丹", "description": "d", "skill_schema": minSchema(), "degraded": false},
		OperatorSnapshot: model.JSONMap{"id": "dialectic", "name": "对立调和"},
		CreatedAt:        svc.now(),
		ExpiresAt:        svc.now().Add(15 * time.Minute),
	}
	if err := db.Create(p).Error; err != nil {
		t.Fatalf("建预览失败: %v", err)
	}
	return p.UUID
}

// confirmFusion 快捷调用：固定操作键
func confirmFusion(t *testing.T, svc *Inventory, previewID uuid.UUID, name string) (*service.PillOperationResult, errors.Error) {
	t.Helper()
	return svc.ConfirmFusion(context.Background(), service.ConfirmFusionRequest{
		OperationID: uuid.New(), PreviewID: previewID, Name: name, Description: "",
	})
}

// loadPreview 按 UUID 读预览行（断言绑定/lineage 用）
func loadPreview(t *testing.T, db *gorm.DB, uid uuid.UUID) *model.FusionPreview {
	t.Helper()
	var p model.FusionPreview
	if err := db.Where("uuid = ?", uid.String()).First(&p).Error; err != nil {
		t.Fatalf("查预览失败: %v", err)
	}
	return &p
}

// TestConfirmFusionAtomicallyProducesOutput 原子确认：A/B 可用时产生新丹方 v1 与一枚新实例，
// A/B 全部转为 consumed_by_fusion；预览绑定本次操作并写入服务器 lineage
func TestConfirmFusionAtomicallyProducesOutput(t *testing.T) {
	svc, db := newTestSvc(t)
	aID, bID := seedTwoItemsSameRecipe(t, svc)
	pID := seedFusionPreview(t, db, svc, aID, bID)

	res, err := confirmFusion(t, svc, pID, "融合新丹")
	if err != nil {
		t.Fatalf("ConfirmFusion 报错: %v", err)
	}
	// plan 断言：产物齐全 + 消耗材料等于输入集合
	if res.RecipeID == nil || res.RevisionID == nil || len(res.ItemIDs) != 1 {
		t.Fatalf("融合产物不完整: %+v", res)
	}
	got := append([]uuid.UUID(nil), res.ConsumedItemIDs...)
	slices.SortFunc(got, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	want := []uuid.UUID{aID, bID}
	slices.SortFunc(want, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("consumed=%v want=%v", got, want)
	}

	// A/B 均为 consumed_by_fusion
	for _, uid := range []uuid.UUID{aID, bID} {
		var item model.PillItem
		if err := db.Where("uuid = ?", uid.String()).First(&item).Error; err != nil {
			t.Fatalf("查材料失败: %v", err)
		}
		if item.State != model.PillConsumedByFusion {
			t.Fatalf("材料 %s 状态 = %s, 期望 consumed_by_fusion", uid.String(), item.State)
		}
	}

	// 新丹方 v1 + 产物 available 且引用新版本
	var recipe model.PillRecipe
	if err := db.Where("uuid = ?", res.RecipeID.String()).First(&recipe).Error; err != nil {
		t.Fatalf("查新丹方失败: %v", err)
	}
	var rev model.PillRecipeRevision
	if err := db.Where("uuid = ?", res.RevisionID.String()).First(&rev).Error; err != nil {
		t.Fatalf("查新版本失败: %v", err)
	}
	if rev.RecipeID != recipe.UUID.String() || rev.Revision != 1 || rev.Name != "融合新丹" {
		t.Fatalf("新版本字段异常: %+v (recipe=%d)", rev, recipe.ID)
	}
	var out model.PillItem
	if err := db.Where("uuid = ?", res.ItemIDs[0].String()).First(&out).Error; err != nil {
		t.Fatalf("查产物失败: %v", err)
	}
	if out.State != model.PillAvailable || out.RecipeRevisionID != rev.UUID.String() {
		t.Fatalf("产物状态/版本异常: state=%s rev=%s", out.State, out.RecipeRevisionID)
	}

	// 预览绑定本次操作 + lineage 写入（父实例/版本/名称 + 操作 UUID + 操作者）
	preview := loadPreview(t, db, pID)
	if preview.ConfirmedOperationID == nil {
		t.Fatal("确认后预览未绑定操作")
	}
	lineage, ok := preview.OutputJSON["lineage"].(map[string]any)
	if !ok {
		t.Fatalf("预览输出缺少服务器 lineage: %+v", preview.OutputJSON)
	}
	if lineage["operation_id"] != res.OperationID.String() {
		t.Fatalf("lineage.operation_id = %v, 期望 %s", lineage["operation_id"], res.OperationID.String())
	}
	if len(lineage["parent_items"].([]any)) != 2 || len(lineage["parent_names"].([]any)) != 2 {
		t.Fatalf("lineage 父材料信息不完整: %+v", lineage)
	}
	if _, ok := lineage["operator"].(map[string]any); !ok {
		t.Fatalf("lineage 缺少操作者快照: %+v", lineage)
	}
}

// TestConfirmFusionFailsWhenMaterialConsumedElsewhere 预览外先服用 B：
// 确认必须失败（409 pill.not_available），A 仍 available、不创建任何产物、预览未绑定
func TestConfirmFusionFailsWhenMaterialConsumedElsewhere(t *testing.T) {
	svc, db := newTestSvc(t)
	aID, bID := seedTwoItemsSameRecipe(t, svc)
	agentID := seedFusionAgent(t, db)
	pID := seedFusionPreview(t, db, svc, aID, bID)

	// 预览外服用 B（模拟另一操作消耗了材料）
	if _, err := svc.Consume(context.Background(), service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: agentID, ItemID: bID, Weight: 1, SortOrder: 1,
	}); err != nil {
		t.Fatalf("预置服用 B 失败: %v", err)
	}

	_, err := confirmFusion(t, svc, pID, "融合新丹")
	if err == nil {
		t.Fatal("材料被消耗后确认应失败, 实际 nil")
	}
	if !errors.IsType(err, errors.ErrorTypeConflict) || err.GetCode() != "pill.not_available" {
		t.Fatalf("错误 = %s (code=%s), 期望 409 pill.not_available", err.Error(), err.GetCode())
	}

	// A 仍 available
	var a model.PillItem
	db.Where("uuid = ?", aID.String()).First(&a)
	if a.State != model.PillAvailable {
		t.Fatalf("失败确认后 A 状态 = %s, 期望 available", a.State)
	}
	// 无新丹方（原有 1 个原料丹方）无产物（原有 2 枚）
	var recipeCount int64
	db.Model(&model.PillRecipe{}).Count(&recipeCount)
	if recipeCount != 1 {
		t.Fatalf("失败确认后丹方数 = %d, 期望 1", recipeCount)
	}
	var itemCount int64
	db.Model(&model.PillItem{}).Count(&itemCount)
	if itemCount != 2 {
		t.Fatalf("失败确认后实例数 = %d, 期望 2", itemCount)
	}
	// 预览未绑定
	if loadPreview(t, db, pID).ConfirmedOperationID != nil {
		t.Fatal("失败确认后预览不应绑定操作")
	}
}

// TestConfirmFusionSameKeyIdempotent 同预览同 key 重试返回同一结果，不重复产出
func TestConfirmFusionSameKeyIdempotent(t *testing.T) {
	svc, db := newTestSvc(t)
	aID, bID := seedTwoItemsSameRecipe(t, svc)
	pID := seedFusionPreview(t, db, svc, aID, bID)

	key := uuid.New()
	req := service.ConfirmFusionRequest{OperationID: key, PreviewID: pID, Name: "融合新丹", Description: ""}
	first, err := svc.ConfirmFusion(context.Background(), req)
	if err != nil {
		t.Fatalf("首次确认失败: %v", err)
	}
	second, err := svc.ConfirmFusion(context.Background(), req)
	if err != nil {
		t.Fatalf("同 key 重试失败: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("同 key 重试结果不一致:\nfirst=%+v\nsecond=%+v", first, second)
	}
	var recipeCount int64
	db.Model(&model.PillRecipe{}).Count(&recipeCount)
	if recipeCount != 2 {
		t.Fatalf("同 key 重试后丹方数 = %d, 期望 2（1 原料 + 1 产物）", recipeCount)
	}
}

// TestConfirmFusionDifferentKeyRejected 同预览不同 key 二次确认 409，
// 且消息携带已存在的操作 UUID（不生成第二份产物）
func TestConfirmFusionDifferentKeyRejected(t *testing.T) {
	svc, db := newTestSvc(t)
	aID, bID := seedTwoItemsSameRecipe(t, svc)
	pID := seedFusionPreview(t, db, svc, aID, bID)

	res, err := confirmFusion(t, svc, pID, "融合新丹")
	if err != nil {
		t.Fatalf("首次确认失败: %v", err)
	}
	_, err = confirmFusion(t, svc, pID, "融合新丹")
	if err == nil {
		t.Fatal("同预览不同 key 二次确认应 409, 实际 nil")
	}
	if !errors.IsType(err, errors.ErrorTypeConflict) || err.GetCode() != "fusion.preview_already_confirmed" {
		t.Fatalf("错误 = %s (code=%s), 期望 409 fusion.preview_already_confirmed", err.Error(), err.GetCode())
	}
	if !strings.Contains(err.Error(), res.OperationID.String()) {
		t.Fatalf("409 消息应携带已有操作 UUID, 实际: %s", err.Error())
	}
	var recipeCount int64
	db.Model(&model.PillRecipe{}).Count(&recipeCount)
	if recipeCount != 2 {
		t.Fatalf("二次确认后丹方数 = %d, 期望 2", recipeCount)
	}
}

// TestConfirmFusionExpiredPreviewRejected 过期预览 410 fusion.preview_expired；材料不动
func TestConfirmFusionExpiredPreviewRejected(t *testing.T) {
	svc, db := newTestSvc(t)
	aID, bID := seedTwoItemsSameRecipe(t, svc)
	pID := seedFusionPreview(t, db, svc, aID, bID)
	if err := db.Model(&model.FusionPreview{}).Where("uuid = ?", pID.String()).
		Update("expires_at", svc.now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("改过期失败: %v", err)
	}

	_, err := confirmFusion(t, svc, pID, "融合新丹")
	if err == nil {
		t.Fatal("过期预览确认应失败, 实际 nil")
	}
	if !errors.IsType(err, errors.ErrorTypeGone) || err.GetCode() != "fusion.preview_expired" {
		t.Fatalf("错误 = %s (code=%s), 期望 410 fusion.preview_expired", err.Error(), err.GetCode())
	}
	for _, uid := range []uuid.UUID{aID, bID} {
		var item model.PillItem
		db.Where("uuid = ?", uid.String()).First(&item)
		if item.State != model.PillAvailable {
			t.Fatalf("过期拒绝后材料 %s 状态 = %s, 期望 available", uid.String(), item.State)
		}
	}
}

// TestConfirmFusionCommittedResultSurvivesExpiry 确认成功后才过期：
// 同 key 重试仍返回已提交结果，不报过期（§3.3：先检查已提交幂等结果）
func TestConfirmFusionCommittedResultSurvivesExpiry(t *testing.T) {
	svc, db := newTestSvc(t)
	aID, bID := seedTwoItemsSameRecipe(t, svc)
	pID := seedFusionPreview(t, db, svc, aID, bID)

	key := uuid.New()
	req := service.ConfirmFusionRequest{OperationID: key, PreviewID: pID, Name: "融合新丹", Description: ""}
	first, err := svc.ConfirmFusion(context.Background(), req)
	if err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	// 确认后把预览改为过期
	if err := db.Model(&model.FusionPreview{}).Where("uuid = ?", pID.String()).
		Update("expires_at", svc.now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("改过期失败: %v", err)
	}
	second, err := svc.ConfirmFusion(context.Background(), req)
	if err != nil {
		t.Fatalf("已提交结果应无视预览过期, 实际报错: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("已提交结果不一致:\nfirst=%+v\nsecond=%+v", first, second)
	}
}

// TestConfirmFusionUnknownPreviewNotFound 未知预览 404 fusion.preview_not_found
func TestConfirmFusionUnknownPreviewNotFound(t *testing.T) {
	svc, _ := newTestSvc(t)
	_, err := confirmFusion(t, svc, uuid.New(), "融合新丹")
	if err == nil {
		t.Fatal("未知预览应 404, 实际 nil")
	}
	if !errors.IsType(err, errors.ErrorTypeRecordNotFound) || err.GetCode() != "fusion.preview_not_found" {
		t.Fatalf("错误 = %s (code=%s), 期望 404 fusion.preview_not_found", err.Error(), err.GetCode())
	}
}

// TestConfirmFusionEmptyNameRejected 确认只能改名称/描述；空名称 400
func TestConfirmFusionEmptyNameRejected(t *testing.T) {
	svc, db := newTestSvc(t)
	aID, bID := seedTwoItemsSameRecipe(t, svc)
	pID := seedFusionPreview(t, db, svc, aID, bID)
	_, err := svc.ConfirmFusion(context.Background(), service.ConfirmFusionRequest{
		OperationID: uuid.New(), PreviewID: pID, Name: "  ", Description: "",
	})
	if err == nil {
		t.Fatal("空名称应 400, 实际 nil")
	}
	if !err.IsType(errors.ErrorTypeInvalidRequest) {
		t.Fatalf("错误类型不匹配 (code=%s, msg=%s), 期望 InvalidRequest", err.GetCode(), err.Error())
	}
}

// TestConsumeFusionRaceSingleWinner 两个独立 SQLite 连接并发：
// 连接 1 服用 A 与 连接 2 确认融合(A+B) 抢同一枚材料，恰有一个消费方成功；
// 失败融合不得消耗 B（整体回滚）
func TestConsumeFusionRaceSingleWinner(t *testing.T) {
	path := t.TempDir() + "/race.db"
	db1 := openInventoryDBAt(t, path)
	svc1 := New(db1, func() time.Time { return confirmFixedNow })
	agentID := seedFusionAgent(t, db1)
	aID, bID := seedTwoItemsSameRecipe(t, svc1)
	pID := seedFusionPreview(t, db1, svc1, aID, bID)

	db2 := openInventoryDBAt(t, path)
	svc2 := New(db2, func() time.Time { return confirmFixedNow })

	consumeCh := make(chan errors.Error, 1)
	confirmCh := make(chan errors.Error, 1)
	go func() {
		_, err := svc1.Consume(context.Background(), service.ConsumePillRequest{
			OperationID: uuid.New(), AgentID: agentID, ItemID: aID, Weight: 1, SortOrder: 1,
		})
		consumeCh <- err
	}()
	go func() {
		_, err := svc2.ConfirmFusion(context.Background(), service.ConfirmFusionRequest{
			OperationID: uuid.New(), PreviewID: pID, Name: "竞态融合", Description: "",
		})
		confirmCh <- err
	}()
	consumeErr, confirmErr := <-consumeCh, <-confirmCh

	if (consumeErr == nil) == (confirmErr == nil) {
		t.Fatalf("期望恰一个消费方成功: consume=%v confirm=%v", consumeErr, confirmErr)
	}
	// 失败的融合不得消耗 B（若融合失败，B 必须仍 available）
	if confirmErr != nil {
		var b model.PillItem
		if err := db1.Where("uuid = ?", bID.String()).First(&b).Error; err != nil {
			t.Fatalf("查 B 失败: %v", err)
		}
		if b.State != model.PillAvailable {
			t.Fatalf("失败融合后 B 状态 = %s, 期望 available（不部分消耗）", b.State)
		}
		// 失败融合不得绑定预览
		if loadPreview(t, db1, pID).ConfirmedOperationID != nil {
			t.Fatal("失败融合后预览不应绑定")
		}
	}
}
