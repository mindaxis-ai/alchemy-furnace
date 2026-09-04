// 任务 5 测试：金丹消耗品库存 HTTP 接口（plan checklist 1-2）
// 覆盖：缺 Idempotency-Key=400、冲突=409、过期=410、合法请求响应字段完整、
// 效果归属校验 404、断线恢复 GET /pill-operations、DesktopGuard 拦截时库存不可操作。
// 夹具：真实 sqlite 内存库 + 真实 Inventory + 真实 fusion_service(假引擎) + 真实 agent_service，
// 零 mock（融合引擎为最小假实现，与 fusion_service 单测同模式）。
package pill_inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/service/agent_service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/fusion_service"
	"github.com/alchemy-furnace/server/internal/service/pill_inventory_service"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/middleware"
	"github.com/alchemy-furnace/server/server/http/router"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// fixedNow 固定时钟：与库存/融合服务测试同基准（2026-08-31 12:00 UTC）
var fixedNow = time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

// fakeFusionClient 假合成引擎：固定返回合法融合输出（实现 synthesis.FusionClient）
type fakeFusionClient struct{}

func (fakeFusionClient) Fuse(_ context.Context, _ []synthesis.PillInput, _ string, _ *credential.ModelCredentials) (*synthesis.FuseResponse, error) {
	return &synthesis.FuseResponse{
		Name:        "融合新丹",
		Description: "由两枚金丹融合而来",
		SkillSchema: model.JSONMap{"expression_dna": map[string]any{"sentence_length": "mixed"}},
		Operator:    synthesis.FuseOperator{ID: "dialectic", Name: "对立调和"},
	}, nil
}

// minSchema 最小合法能力 schema（与库存测试共用形状）
func minSchema() model.JSONMap {
	return model.JSONMap{"expression_dna": map[string]any{"sentence_length": "mixed"}}
}

// setupTestDB 初始化 sqlite 内存库并注入全局 dao.DB（全模型迁移）
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:pillinventory%d?mode=memory&cache=shared", time.Now().UnixNano())
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
		&model.DaoAgent{}, &model.LanguagePattern{},
		&model.PillRecipe{}, &model.PillRecipeRevision{}, &model.PillItem{},
		&model.AgentPillEffect{}, &model.PillOperation{}, &model.FusionPreview{},
		&model.PillStarterGrant{},
	); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	dao.DB = db
	t.Cleanup(func() { dao.DB = nil })
	return db
}

// setupRouter 装配真实服务栈（Inventory + fusion_service + agent_service）+ 新 handler，
// 注册任务 5 全部新路由（§2.3）
func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())

	db := dao.GetDB()
	inventory := pill_inventory_service.New(db, func() time.Time { return fixedNow })
	fusion := fusion_service.NewWithClock(db, fakeFusionClient{}, nil, func() time.Time { return fixedNow })
	agent := agent_service.New(dao.NewAgentDao(), dao.NewModelDao(), inventory)
	h := New(inventory, fusion, agent)

	v1 := r.Group("/api/v1")
	// 丹方
	v1.GET("/recipes", router.Wrapper(h.ListRecipes))
	v1.POST("/recipes", router.Wrapper(h.SaveRecipe))
	v1.GET("/recipes/:id", router.Wrapper(h.GetRecipe))
	v1.GET("/recipes/:id/revisions/:revision_id", router.Wrapper(h.GetRecipeRevision))
	v1.POST("/recipes/:id/revisions", router.Wrapper(h.UpdateRecipe))
	v1.POST("/recipes/:id/archive", router.Wrapper(h.ArchiveRecipe))
	v1.POST("/recipes/:id/craft", router.Wrapper(h.CraftPill))
	// 金丹库存
	v1.GET("/pill-items", router.Wrapper(h.ListPillItems))
	v1.GET("/pill-items/:id", router.Wrapper(h.GetPillItem))
	v1.POST("/pill-items/:id/discard", router.Wrapper(h.DiscardItem))
	// 道人服用与能力编排
	v1.POST("/agents/:uuid/consume", router.Wrapper(h.ConsumePill))
	v1.GET("/agents/:uuid/effects", router.Wrapper(h.ListEffects))
	v1.PUT("/agents/:uuid/effects", router.Wrapper(h.UpdateEffects))
	v1.POST("/agents/:uuid/effects/:effect_id/remove", router.Wrapper(h.RemoveEffect))
	// 融合两阶段
	v1.POST("/fusion/previews", router.Wrapper(h.PreviewFusion))
	v1.POST("/fusion/confirm", router.Wrapper(h.ConfirmFusion))
	// 幂等操作查询（断线恢复）
	v1.GET("/pill-operations/:id", router.Wrapper(h.GetOperation))
	return r
}

// seedAgent 造一个道人，返回对外 UUID
func seedAgent(t *testing.T, db *gorm.DB) string {
	t.Helper()
	agent := model.DaoAgent{Name: "测试道人", ModelName: "gpt-4o", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("建道人失败: %v", err)
	}
	return agent.UUID.String()
}

// seedRecipeAndItem 直接造数：丹方 v1 + 一枚可用金丹实例 + 道人，返回 (道人UUID, 实例UUID)
func seedRecipeAndItem(t *testing.T, db *gorm.DB) (string, string) {
	t.Helper()
	agentID := seedAgent(t, db)
	recipe := model.PillRecipe{}
	if err := db.Create(&recipe).Error; err != nil {
		t.Fatalf("建丹方失败: %v", err)
	}
	rev := model.PillRecipeRevision{
		RecipeID:    recipe.UUID.String(),
		Revision:    1,
		Name:        "测试丹方",
		SkillSchema: minSchema(),
	}
	if err := db.Create(&rev).Error; err != nil {
		t.Fatalf("建丹方版本失败: %v", err)
	}
	item := model.PillItem{RecipeRevisionID: rev.UUID.String(), State: model.PillAvailable}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("建金丹实例失败: %v", err)
	}
	return agentID, item.UUID.String()
}

// doJSON 发送 JSON 请求并解析响应包络；key 非空时携带 Idempotency-Key 头
func doJSON(t *testing.T, r *gin.Engine, method, path, body, key string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var envelope map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析响应包络失败: %v, body: %s", err, w.Body.String())
	}
	return w.Code, envelope
}

// dataString 从响应 data 取字符串字段
func dataString(t *testing.T, envelope map[string]interface{}, key string) string {
	t.Helper()
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("响应缺少 data 对象: %v", envelope)
	}
	v, ok := data[key].(string)
	if !ok || v == "" {
		t.Fatalf("响应 data.%s 缺失: %v", key, envelope)
	}
	return v
}

// TestConsumeMissingKeyRejected 缺 Idempotency-Key：400，库存零变更
func TestConsumeMissingKeyRejected(t *testing.T) {
	db := setupTestDB(t)
	agentID, itemID := seedRecipeAndItem(t, db)
	r := setupRouter()

	status, envelope := doJSON(t, r, http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/consume", agentID),
		fmt.Sprintf(`{"item_id":%q,"weight":1,"sort_order":1}`, itemID), "")

	if status != http.StatusBadRequest {
		t.Fatalf("缺 key 期望 400, 实际 %d, body: %v", status, envelope)
	}
	if ec, _ := envelope["error_code"].(string); ec == "" {
		t.Fatalf("缺 key 应携带稳定 error_code: %v", envelope)
	}
	var item model.PillItem
	db.Where("uuid = ?", itemID).First(&item)
	if item.State != model.PillAvailable {
		t.Fatalf("缺 key 拒绝后实例状态 = %s, 期望 available", item.State)
	}
	var effects int64
	db.Model(&model.AgentPillEffect{}).Count(&effects)
	var ops int64
	db.Model(&model.PillOperation{}).Count(&ops)
	if effects != 0 || ops != 0 {
		t.Fatalf("缺 key 拒绝后不应有任何写入: effects=%d ops=%d", effects, ops)
	}
}

// TestConsumeHappyPath 合法服用：200，data 含 operation_id/effect_id；DB 状态迁移
func TestConsumeHappyPath(t *testing.T) {
	db := setupTestDB(t)
	agentID, itemID := seedRecipeAndItem(t, db)
	r := setupRouter()

	status, envelope := doJSON(t, r, http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/consume", agentID),
		fmt.Sprintf(`{"item_id":%q,"weight":1,"sort_order":1}`, itemID), uuid.NewString())

	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body: %v", status, envelope)
	}
	opID := dataString(t, envelope, "operation_id")
	effectID := dataString(t, envelope, "effect_id")
	if _, err := uuid.Parse(opID); err != nil {
		t.Fatalf("operation_id 非 UUID: %s", opID)
	}
	if _, err := uuid.Parse(effectID); err != nil {
		t.Fatalf("effect_id 非 UUID: %s", effectID)
	}

	var item model.PillItem
	db.Where("uuid = ?", itemID).First(&item)
	if item.State != model.PillConsumedByAgent {
		t.Fatalf("实例状态 = %s, 期望 consumed_by_agent", item.State)
	}
	var effect model.AgentPillEffect
	if err := db.Where("uuid = ?", effectID).First(&effect).Error; err != nil {
		t.Fatalf("能力未落库: %v", err)
	}
	if effect.RemovedAt != nil {
		t.Fatalf("新能力不应被标记移除: %+v", effect)
	}
	var op model.PillOperation
	if err := db.Where("uuid = ?", opID).First(&op).Error; err != nil {
		t.Fatalf("操作记录未落库: %v", err)
	}
}

// TestListEffectsCarriesRevision GET /effects 必须携带 effects_revision
// （前端 PUT 全量编排的乐观锁输入；缺它前端永远 409）
func TestListEffectsCarriesRevision(t *testing.T) {
	db := setupTestDB(t)
	agentID, itemID := seedRecipeAndItem(t, db)
	r := setupRouter()

	// 服用一次让 EffectsRevision 递增（0 → 1）
	if status, _ := doJSON(t, r, http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/consume", agentID),
		fmt.Sprintf(`{"item_id":%q,"weight":2,"sort_order":1}`, itemID), uuid.NewString()); status != http.StatusOK {
		t.Fatalf("服用失败: %d", status)
	}

	status, envelope := doJSON(t, r, http.MethodGet,
		fmt.Sprintf("/api/v1/agents/%s/effects", agentID), "", "")
	if status != http.StatusOK {
		t.Fatalf("GET effects 期望 200, 实际 %d, body: %v", status, envelope)
	}
	data, _ := envelope["data"].(map[string]interface{})
	rev, ok := data["effects_revision"].(float64)
	if !ok || rev <= 0 {
		t.Fatalf("effects_revision 缺失或非正数: %v", envelope)
	}
	items, ok := data["effects"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("effects 应含 1 条服用能力: %v", envelope)
	}
	first, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("effects[0] 非对象: %v", items[0])
	}
	if first["item_id"] != itemID {
		t.Fatalf("effects[0].item_id = %v, 期望消耗实例 %s", first["item_id"], itemID)
	}
	for _, key := range []string{"id", "name", "revision_id"} {
		if s, ok := first[key].(string); !ok || s == "" {
			t.Fatalf("effects[0].%s 缺失: %v", key, first)
		}
	}
	if w, ok := first["weight"].(float64); !ok || w != 2 {
		t.Fatalf("effects[0].weight = %v, 期望 2", first["weight"])
	}
}

// TestConsumeSameKeyIdempotent 同 key 重试：返回同一结果，不重复消耗/生成
func TestConsumeSameKeyIdempotent(t *testing.T) {
	db := setupTestDB(t)
	agentID, itemID := seedRecipeAndItem(t, db)
	r := setupRouter()
	key := uuid.NewString()
	path := fmt.Sprintf("/api/v1/agents/%s/consume", agentID)
	body := fmt.Sprintf(`{"item_id":%q,"weight":1,"sort_order":1}`, itemID)

	status1, env1 := doJSON(t, r, http.MethodPost, path, body, key)
	status2, env2 := doJSON(t, r, http.MethodPost, path, body, key)
	if status1 != http.StatusOK || status2 != http.StatusOK {
		t.Fatalf("重试应 200: %d %d (%v / %v)", status1, status2, env1, env2)
	}
	if !bytes.Equal(mustMarshal(env1["data"]), mustMarshal(env2["data"])) {
		t.Fatalf("同 key 重试结果不一致: %v vs %v", env1["data"], env2["data"])
	}
	var effects int64
	db.Model(&model.AgentPillEffect{}).Count(&effects)
	var ops int64
	db.Model(&model.PillOperation{}).Count(&ops)
	if effects != 1 || ops != 1 {
		t.Fatalf("同 key 重试后应各 1 条: effects=%d ops=%d", effects, ops)
	}
}

// TestConsumeDifferentKeyConflict 换 key 服用同实例：409 pill.not_available
func TestConsumeDifferentKeyConflict(t *testing.T) {
	db := setupTestDB(t)
	agentID, itemID := seedRecipeAndItem(t, db)
	r := setupRouter()
	path := fmt.Sprintf("/api/v1/agents/%s/consume", agentID)
	body := fmt.Sprintf(`{"item_id":%q,"weight":1,"sort_order":1}`, itemID)

	if status, _ := doJSON(t, r, http.MethodPost, path, body, uuid.NewString()); status != http.StatusOK {
		t.Fatalf("首次服用失败: %d", status)
	}
	status, envelope := doJSON(t, r, http.MethodPost, path, body, uuid.NewString())
	if status != http.StatusConflict {
		t.Fatalf("换 key 期望 409, 实际 %d, body: %v", status, envelope)
	}
	if envelope["error_code"] != "pill.not_available" {
		t.Fatalf("期望 error_code=pill.not_available, 实际 %v", envelope)
	}
}

// TestSaveRecipeCraftOne 合法丹方创建 + 同事务炼制：200，data 含 recipe/revision/item
func TestSaveRecipeCraftOne(t *testing.T) {
	db := setupTestDB(t)
	r := setupRouter()

	status, envelope := doJSON(t, r, http.MethodPost, "/api/v1/recipes",
		`{"name":"红炉丹","description":"试炼","skill_schema":{"expression_dna":{"sentence_length":"mixed"}},"craft_one":true}`,
		uuid.NewString())

	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body: %v", status, envelope)
	}
	recipeID := dataString(t, envelope, "recipe_id")
	revisionID := dataString(t, envelope, "revision_id")
	data, _ := envelope["data"].(map[string]interface{})
	items, ok := data["item_ids"].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatalf("craft_one 应返回 item_ids: %v", envelope)
	}
	var recipe model.PillRecipe
	if err := db.Where("uuid = ?", recipeID).First(&recipe).Error; err != nil {
		t.Fatalf("丹方未落库: %v", err)
	}
	var rev model.PillRecipeRevision
	if err := db.Where("uuid = ?", revisionID).First(&rev).Error; err != nil {
		t.Fatalf("版本未落库: %v", err)
	}
	if rev.Revision != 1 {
		t.Fatalf("新丹方应 v1, 实际 %d", rev.Revision)
	}
	var itemCount int64
	db.Model(&model.PillItem{}).Count(&itemCount)
	if itemCount != 1 {
		t.Fatalf("应恰好 1 枚可用实例, 实际 %d", itemCount)
	}
}

// TestConfirmFusionExpiredPreviewRejected 过期预览确认：410 fusion.preview_expired，材料不动
func TestConfirmFusionExpiredPreviewRejected(t *testing.T) {
	db := setupTestDB(t)
	r := setupRouter()
	seedAgent(t, db)

	// 造两枚同版本可用实例（真实 SaveRecipe/Craft 链路）
	recipeRes := saveRecipeCraftOne(t, r)
	craft := craftOne(t, r, recipeRes)
	items := []string{recipeRes["item_ids"].([]interface{})[0].(string), craft["item_ids"].([]interface{})[0].(string)}

	// 预览（真实 fusion_service + 假引擎）
	status, envelope := doJSON(t, r, http.MethodPost, "/api/v1/fusion/previews",
		fmt.Sprintf(`{"item_ids":[%q,%q]}`, items[0], items[1]), "")
	if status != http.StatusOK {
		t.Fatalf("预览失败: %d, body: %v", status, envelope)
	}
	previewID := dataString(t, envelope, "preview_id")

	// 将预览改为过期
	if err := db.Model(&model.FusionPreview{}).Where("uuid = ?", previewID).
		Update("expires_at", fixedNow.Add(-time.Minute)).Error; err != nil {
		t.Fatalf("改过期失败: %v", err)
	}

	// 确认 → 410
	status, envelope = doJSON(t, r, http.MethodPost, "/api/v1/fusion/confirm",
		fmt.Sprintf(`{"preview_id":%q,"name":"融合新丹"}`, previewID), uuid.NewString())
	if status != http.StatusGone {
		t.Fatalf("过期预览期望 410, 实际 %d, body: %v", status, envelope)
	}
	if envelope["error_code"] != "fusion.preview_expired" {
		t.Fatalf("期望 error_code=fusion.preview_expired, 实际 %v", envelope)
	}
	// 材料未消耗、无产物
	for _, uid := range items {
		var item model.PillItem
		db.Where("uuid = ?", uid).First(&item)
		if item.State != model.PillAvailable {
			t.Fatalf("过期拒绝后材料 %s 状态 = %s, 期望 available", uid, item.State)
		}
	}
	var recipeCount int64
	db.Model(&model.PillRecipe{}).Count(&recipeCount)
	if recipeCount != 1 {
		t.Fatalf("过期拒绝后丹方数 = %d, 期望 1", recipeCount)
	}
}

// TestRemoveEffectWrongAgentRejected 跨道人移除能力：404，能力保持活跃
func TestRemoveEffectWrongAgentRejected(t *testing.T) {
	db := setupTestDB(t)
	agentID, itemID := seedRecipeAndItem(t, db)
	r := setupRouter()

	status, envelope := doJSON(t, r, http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/consume", agentID),
		fmt.Sprintf(`{"item_id":%q,"weight":1,"sort_order":1}`, itemID), uuid.NewString())
	if status != http.StatusOK {
		t.Fatalf("预置服用失败: %d, body: %v", status, envelope)
	}
	effectID := dataString(t, envelope, "effect_id")

	// 用另一个道人 UUID 移除该能力
	status, envelope = doJSON(t, r, http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/effects/%s/remove", uuid.NewString(), effectID),
		"{}", uuid.NewString())
	if status != http.StatusNotFound {
		t.Fatalf("跨道人移除期望 404, 实际 %d, body: %v", status, envelope)
	}
	var effect model.AgentPillEffect
	if err := db.Where("uuid = ?", effectID).First(&effect).Error; err != nil {
		t.Fatalf("能力不应被删除: %v", err)
	}
	if effect.RemovedAt != nil {
		t.Fatalf("跨道人移除后能力不应被标记移除: %+v", effect)
	}
}

// TestGetOperationAfterConsume 断线恢复：GET /pill-operations/:id 返回已提交结果
func TestGetOperationAfterConsume(t *testing.T) {
	db := setupTestDB(t)
	agentID, itemID := seedRecipeAndItem(t, db)
	r := setupRouter()
	key := uuid.NewString()
	path := fmt.Sprintf("/api/v1/agents/%s/consume", agentID)
	body := fmt.Sprintf(`{"item_id":%q,"weight":1,"sort_order":1}`, itemID)

	_, envelope := doJSON(t, r, http.MethodPost, path, body, key)
	opID := dataString(t, envelope, "operation_id")

	status, envelope := doJSON(t, r, http.MethodGet, "/api/v1/pill-operations/"+opID, "", "")
	if status != http.StatusOK {
		t.Fatalf("查操作期望 200, 实际 %d, body: %v", status, envelope)
	}
	if dataString(t, envelope, "operation_id") != opID {
		t.Fatalf("操作结果 operation_id 不一致: %v", envelope)
	}
	if dataString(t, envelope, "effect_id") == "" {
		t.Fatalf("操作结果缺少 effect_id: %v", envelope)
	}

	// 未知操作 404
	status, _ = doJSON(t, r, http.MethodGet, "/api/v1/pill-operations/"+uuid.NewString(), "", "")
	if status != http.StatusNotFound {
		t.Fatalf("未知操作期望 404, 实际 %d", status)
	}
}

// TestDesktopGuardBlocksInventoryWrites 未通过 DesktopGuard：401 且库存零变更
func TestDesktopGuardBlocksInventoryWrites(t *testing.T) {
	db := setupTestDB(t)
	agentID, itemID := seedRecipeAndItem(t, db)

	gin.SetMode(gin.TestMode)
	guarded := gin.New()
	guarded.Use(middleware.DesktopGuard("test-token", "127.0.0.1:34567"))
	inventory := pill_inventory_service.New(db, func() time.Time { return fixedNow })
	fusion := fusion_service.NewWithClock(db, fakeFusionClient{}, nil, func() time.Time { return fixedNow })
	agent := agent_service.New(dao.NewAgentDao(), dao.NewModelDao(), inventory)
	h := New(inventory, fusion, agent)
	guarded.POST("/api/v1/agents/:id/consume", router.Wrapper(h.ConsumePill))

	// httptest 默认 Host=example.com 不匹配守卫地址 → 401
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/agents/%s/consume", agentID),
		bytes.NewBufferString(fmt.Sprintf(`{"item_id":%q,"weight":1,"sort_order":1}`, itemID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	w := httptest.NewRecorder()
	guarded.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("未过守卫期望 401, 实际 %d, body: %s", w.Code, w.Body.String())
	}
	var item model.PillItem
	db.Where("uuid = ?", itemID).First(&item)
	if item.State != model.PillAvailable {
		t.Fatalf("守卫拦截后实例状态 = %s, 期望 available（库存不可被绕过操作）", item.State)
	}
	var effects int64
	db.Model(&model.AgentPillEffect{}).Count(&effects)
	if effects != 0 {
		t.Fatalf("守卫拦截后不应产生能力: %d", effects)
	}
}

// ---------- 造数辅助（真实服务链路） ----------

// saveRecipeCraftOne 走真实 HTTP 链路创建丹方并炼制一枚，返回 data 对象
func saveRecipeCraftOne(t *testing.T, r *gin.Engine) map[string]interface{} {
	t.Helper()
	status, envelope := doJSON(t, r, http.MethodPost, "/api/v1/recipes",
		`{"name":"融合原料丹","skill_schema":{"expression_dna":{"sentence_length":"mixed"}},"craft_one":true}`,
		uuid.NewString())
	if status != http.StatusOK {
		t.Fatalf("造原料丹失败: %d, body: %v", status, envelope)
	}
	data, _ := envelope["data"].(map[string]interface{})
	return data
}

// craftOne 按版本再炼一枚，返回 data 对象
func craftOne(t *testing.T, r *gin.Engine, recipe map[string]interface{}) map[string]interface{} {
	t.Helper()
	revisionID, _ := recipe["revision_id"].(string)
	status, envelope := doJSON(t, r, http.MethodPost,
		"/api/v1/recipes/"+recipe["recipe_id"].(string)+"/craft",
		fmt.Sprintf(`{"revision_id":%q}`, revisionID), uuid.NewString())
	if status != http.StatusOK {
		t.Fatalf("再炼失败: %d, body: %v", status, envelope)
	}
	data, _ := envelope["data"].(map[string]interface{})
	return data
}

// mustMarshal 序列化用于结果比较（同 key 幂等断言）
func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// TestGetPillItemCarriesTags 实例详情必须携带来源版本的标签
// （hero spotlight 的「丹性」标签行依赖它；前端 PillItemDetail.tags）
func TestGetPillItemCarriesTags(t *testing.T) {
	db := setupTestDB(t)
	r := setupRouter()

	recipe := model.PillRecipe{}
	if err := db.Create(&recipe).Error; err != nil {
		t.Fatalf("建丹方失败: %v", err)
	}
	rev := model.PillRecipeRevision{
		RecipeID:    recipe.UUID.String(),
		Revision:    1,
		Name:        "浩然方",
		SkillSchema: minSchema(),
		Tags:        model.JSONList{"内丹", "心法"},
	}
	if err := db.Create(&rev).Error; err != nil {
		t.Fatalf("建丹方版本失败: %v", err)
	}
	item := model.PillItem{RecipeRevisionID: rev.UUID.String(), State: model.PillAvailable}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("建金丹实例失败: %v", err)
	}

	status, envelope := doJSON(t, r, http.MethodGet,
		fmt.Sprintf("/api/v1/pill-items/%s", item.UUID.String()), "", "")
	if status != http.StatusOK {
		t.Fatalf("GET 实例详情期望 200, 实际 %d, body: %v", status, envelope)
	}
	data, _ := envelope["data"].(map[string]interface{})
	tags, ok := data["tags"].([]interface{})
	if !ok || len(tags) != 2 || tags[0] != "内丹" || tags[1] != "心法" {
		t.Fatalf("tags 应携带来源版本标签(内丹/心法): %v", data)
	}
}
