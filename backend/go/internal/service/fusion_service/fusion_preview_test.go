// 任务 4 测试：融合预览（PreviewFusion）——两阶段的第一阶段
// 覆盖 plan 契约：至少两枚且去重校验、同版本两枚可预览、模型失败不改变库存、
// 材料不可用 409、输出不合法不给可确认预览、持久化 FusionPreview
// （输入列表/排序集合哈希/输出/操作者/15 分钟有效期）、模型调用在事务外。
package fusion_service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/pill_inventory_service"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// fusionTestNow 固定时钟：与库存测试同基准（2026-08-31 12:00 UTC）
var fusionTestNow = time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

// fakeFusionClient 实现 synthesis.FusionClient；记录调用入参，可注入错误/自定义响应
type fakeFusionClient struct {
	called    bool
	req       []synthesis.PillInput
	excludeOp string
	resp      *synthesis.FuseResponse
	err       error
}

func (f *fakeFusionClient) Fuse(ctx context.Context, pills []synthesis.PillInput, excludeOperatorID string, creds *credential.ModelCredentials) (*synthesis.FuseResponse, error) {
	f.called = true
	f.req = pills
	f.excludeOp = excludeOperatorID
	if f.err != nil {
		return nil, f.err
	}
	if f.resp != nil {
		return f.resp, nil
	}
	return &synthesis.FuseResponse{
		Name:        "麻辣禅师",
		Description: "辣",
		SkillSchema: model.JSONMap{"expression_dna": map[string]any{"sentence_length": "mixed"}},
		Operator:    synthesis.FuseOperator{ID: "dialectic", Name: "对立调和"},
	}, nil
}

// openFusionTestDB 服务层测试夹具：临时文件 SQLite + 外键 + 单连接
func openFusionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fusion.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = raw.Close() })
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.PillRecipe{}, &model.PillRecipeRevision{}, &model.PillItem{},
		&model.AgentPillEffect{}, &model.PillOperation{}, &model.FusionPreview{},
		&model.PillStarterGrant{},
		&model.DaoAgent{}, &model.LanguagePattern{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

// newFusionSvc 返回服务（固定时钟 + mock 客户端）+ db（造数据/断言用）
func newFusionSvc(t *testing.T) (*Fusion, *gorm.DB, *fakeFusionClient) {
	t.Helper()
	db := openFusionTestDB(t)
	client := &fakeFusionClient{}
	return NewWithClock(db, client, nil, func() time.Time { return fusionTestNow }), db, client
}

// seedFusionItems 同一丹方炼出 count 枚可用实例，返回实例 UUID 列表
func seedFusionItems(t *testing.T, db *gorm.DB, count int) []uuid.UUID {
	t.Helper()
	inv := pill_inventory_service.New(db, func() time.Time { return fusionTestNow })
	first, err := inv.SaveRecipe(context.Background(), service.SaveRecipeRequest{
		OperationID: uuid.New(), CraftOne: true,
		Draft: service.RecipeDraft{Name: "预览原料丹", SkillSchema: model.JSONMap{
			"expression_dna": map[string]any{"sentence_length": "mixed"},
		}},
	})
	if err != nil {
		t.Fatalf("炼第一枚失败: %v", err)
	}
	ids := []uuid.UUID{first.ItemIDs[0]}
	for i := 1; i < count; i++ {
		res, err := inv.CraftOne(context.Background(), service.CraftPillRequest{
			OperationID: uuid.New(), RevisionID: *first.RevisionID,
		})
		if err != nil {
			t.Fatalf("炼第 %d 枚失败: %v", i+1, err)
		}
		ids = append(ids, res.ItemIDs[0])
	}
	return ids
}

// TestPreviewFusionPersistsPreviewAndReturns 两枚同版本实例可预览（产品规则 8）：
// 返回预览 ID + 15 分钟有效期 + 模型结果；DB 行保存输入列表/集合哈希/输出/操作者快照；
// 模型收到的 PillInput 身份 = 实例 UUID、内容 = 版本快照
func TestPreviewFusionPersistsPreviewAndReturns(t *testing.T) {
	svc, db, client := newFusionSvc(t)
	ids := seedFusionItems(t, db, 2)

	res, err := svc.PreviewFusion(context.Background(), service.PreviewFusionRequest{ItemIDs: ids})
	if err != nil {
		t.Fatalf("PreviewFusion 报错: %v", err)
	}
	if res.PreviewID == "" {
		t.Fatal("返回缺少预览 ID")
	}
	if !res.ExpiresAt.Equal(fusionTestNow.Add(15 * time.Minute)) {
		t.Fatalf("expires_at = %v, 期望 +15 分钟", res.ExpiresAt)
	}
	if res.Name != "麻辣禅师" || res.Operator.ID != "dialectic" {
		t.Fatalf("响应字段异常: %+v", res)
	}

	// 模型输入：实例 UUID 身份 + 版本名称/schema
	if !client.called || len(client.req) != 2 {
		t.Fatalf("模型调用参数异常: %+v", client.req)
	}
	for i, in := range client.req {
		if in.ID != ids[i].String() || in.Name != "预览原料丹" {
			t.Fatalf("PillInput[%d] = %+v, 期望 id=%s", i, in, ids[i].String())
		}
	}

	// DB 行：输入列表保持请求顺序 + 排序集合哈希 + 输出 + 操作者 + 有效期
	var preview model.FusionPreview
	if err := db.Where("fusion_preview_id = ?", res.PreviewID).First(&preview).Error; err != nil {
		t.Fatalf("查预览失败: %v", err)
	}
	if len(preview.InputItemsJSON) != 2 || preview.InputItemsJSON[0] != ids[0].String() {
		t.Fatalf("InputItemsJSON = %v, 期望 [%s %s]", preview.InputItemsJSON, ids[0], ids[1])
	}
	if len(preview.InputHash) != 64 {
		t.Fatalf("InputHash 长度 = %d, 期望 64（sha256）", len(preview.InputHash))
	}
	schema, ok := preview.OutputJSON["skill_schema"].(map[string]any)
	if !ok {
		t.Fatalf("OutputJSON 缺少 skill_schema: %+v", preview.OutputJSON)
	}
	if _, ok := schema["expression_dna"]; !ok {
		t.Fatalf("输出 schema 内容不完整: %+v", schema)
	}
	op, ok := preview.OperatorSnapshot["id"].(string)
	if !ok || op != "dialectic" {
		t.Fatalf("OperatorSnapshot 异常: %+v", preview.OperatorSnapshot)
	}
	if !preview.ExpiresAt.Equal(fusionTestNow.Add(15 * time.Minute)) {
		t.Fatalf("预览 expires_at = %v", preview.ExpiresAt)
	}
}

// TestPreviewFusionRejectsTooFewAndDuplicates 少于两枚 / 重复材料 ID 拒绝
func TestPreviewFusionRejectsTooFewAndDuplicates(t *testing.T) {
	svc, db, _ := newFusionSvc(t)
	ids := seedFusionItems(t, db, 2)

	_, err := svc.PreviewFusion(context.Background(), service.PreviewFusionRequest{ItemIDs: ids[:1]})
	if err == nil || !errors.IsType(err, errors.ErrorTypeInvalidRequest) {
		t.Fatalf("单枚应 400 InvalidRequest, 实际 %v", err)
	}

	dup := []uuid.UUID{ids[0], ids[0]}
	_, err = svc.PreviewFusion(context.Background(), service.PreviewFusionRequest{ItemIDs: dup})
	if err == nil || !errors.IsType(err, errors.ErrorTypeInvalidRequest) {
		t.Fatalf("重复材料应 400 InvalidRequest, 实际 %v", err)
	}
	if err.GetCode() != "service.fusion.duplicate_items" {
		t.Fatalf("错误码 = %s, 期望 service.fusion.duplicate_items", err.GetCode())
	}
}

// TestPreviewFusionModelFailureKeepsInventory 模型调用失败：返回错误、
// 所有材料仍 available、不写任何预览行（LLM 请求不在事务内）
func TestPreviewFusionModelFailureKeepsInventory(t *testing.T) {
	svc, db, client := newFusionSvc(t)
	ids := seedFusionItems(t, db, 2)
	client.err = errors.New(errors.ErrorTypeServerInternalError, "test.model_down", "模型不可用")

	_, err := svc.PreviewFusion(context.Background(), service.PreviewFusionRequest{ItemIDs: ids})
	if err == nil {
		t.Fatal("模型失败应返回错误, 实际 nil")
	}
	for _, uid := range ids {
		var item model.PillItem
		if err := db.Where("pill_item_id = ?", uid.String()).First(&item).Error; err != nil {
			t.Fatalf("查材料失败: %v", err)
		}
		if item.State != model.PillAvailable {
			t.Fatalf("模型失败后材料 %s 状态 = %s, 期望 available", uid.String(), item.State)
		}
	}
	var count int64
	db.Model(&model.FusionPreview{}).Count(&count)
	if count != 0 {
		t.Fatalf("模型失败后预览行数 = %d, 期望 0", count)
	}
}

// TestPreviewFusionUnavailableMaterialRejected 材料已被消耗：预览 409 pill.not_available
func TestPreviewFusionUnavailableMaterialRejected(t *testing.T) {
	svc, db, _ := newFusionSvc(t)
	ids := seedFusionItems(t, db, 2)
	agentUID := uuid.New()
	agent := &model.DaoAgent{DaoAgentID: agentUID.String(), Name: "预览道人", ModelName: "gpt-4o", Status: "active"}
	if err := db.Create(agent).Error; err != nil {
		t.Fatalf("建道人失败: %v", err)
	}
	inv := pill_inventory_service.New(db, func() time.Time { return fusionTestNow })
	if _, err := inv.Consume(context.Background(), service.ConsumePillRequest{
		OperationID: uuid.New(), AgentID: agentUID, ItemID: ids[0], Weight: 1, SortOrder: 1,
	}); err != nil {
		t.Fatalf("预置服用失败: %v", err)
	}

	_, err := svc.PreviewFusion(context.Background(), service.PreviewFusionRequest{ItemIDs: ids})
	if err == nil {
		t.Fatal("含已服用材料应 409, 实际 nil")
	}
	if !errors.IsType(err, errors.ErrorTypeConflict) || err.GetCode() != "pill.not_available" {
		t.Fatalf("错误 = %s (code=%s), 期望 409 pill.not_available", err.Error(), err.GetCode())
	}
}

// TestPreviewFusionInvalidOutputRejected 模型输出不合法：不给可确认预览（§3.3）
func TestPreviewFusionInvalidOutputRejected(t *testing.T) {
	svc, db, client := newFusionSvc(t)
	ids := seedFusionItems(t, db, 2)
	client.resp = &synthesis.FuseResponse{
		Name:        "坏丹",
		SkillSchema: model.JSONMap{"not_a_real_field": "x"},
		Operator:    synthesis.FuseOperator{ID: "o", Name: "O"},
	}

	_, err := svc.PreviewFusion(context.Background(), service.PreviewFusionRequest{ItemIDs: ids})
	if err == nil {
		t.Fatal("不合法输出应拒绝, 实际 nil")
	}
	if !errors.IsType(err, errors.ErrorTypeInvalidRequest) || err.GetCode() != "recipe.invalid_schema" {
		t.Fatalf("错误 = %s (code=%s), 期望 400 recipe.invalid_schema", err.Error(), err.GetCode())
	}
	var count int64
	db.Model(&model.FusionPreview{}).Count(&count)
	if count != 0 {
		t.Fatalf("不合法输出后预览行数 = %d, 期望 0", count)
	}
}

// TestPreviewFusionExcludeOperatorForwarded 排除算子 ID 透传模型 + 存操作者快照
func TestPreviewFusionExcludeOperatorForwarded(t *testing.T) {
	svc, db, client := newFusionSvc(t)
	ids := seedFusionItems(t, db, 2)

	_, err := svc.PreviewFusion(context.Background(), service.PreviewFusionRequest{
		ItemIDs: ids, ExcludeOperatorID: "dialectic",
	})
	if err != nil {
		t.Fatalf("PreviewFusion 报错: %v", err)
	}
	if !strings.Contains(client.excludeOp, "dialectic") {
		t.Fatalf("exclude_operator_id 未透传: %q", client.excludeOp)
	}
	var preview model.FusionPreview
	if err := db.First(&preview).Error; err != nil {
		t.Fatalf("查预览失败: %v", err)
	}
	if preview.OperatorSnapshot["exclude_operator_id"] != "dialectic" {
		t.Fatalf("操作者快照缺少 exclude_operator_id: %+v", preview.OperatorSnapshot)
	}
}
