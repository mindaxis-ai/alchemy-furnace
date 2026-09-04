// Skill 导出接口回归测试: POST /api/v1/distillation/skill-export
// 真实 sqlite 内存库 + 真实 service + 真实 handler,fake client 打桩 Python 引擎
package distillation

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
	nudist "github.com/alchemy-furnace/server/internal/distillation"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/distillation_service"
	"github.com/alchemy-furnace/server/internal/service/pill_inventory_service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/middleware"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// setupTestDB 初始化 sqlite 内存库并注入全局 dao.DB
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:distilltest%d?mode=memory&cache=shared", time.Now().UnixNano())
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
		&model.PillRecipe{}, &model.PillRecipeRevision{}, &model.PillLegacyMap{},
		&model.PillItem{}, &model.AgentPillEffect{}, &model.PillOperation{},
		&model.FusionPreview{}, &model.PillMigrationState{}, &model.PillStarterGrant{},
	); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	dao.DB = db
	t.Cleanup(func() { dao.DB = nil })
	return db
}

type fakeExportClient struct {
	skill  *nudist.ExportableSkill
	format string
	result *nudist.ExportResult
	err    error
}

func (f *fakeExportClient) Distill(_ context.Context, _, _, _ string, _ *credential.ModelCredentials) (*nudist.Response, error) {
	return nil, nil
}

func (f *fakeExportClient) SkillExport(_ context.Context, skill *nudist.ExportableSkill, format string) (*nudist.ExportResult, error) {
	f.skill = skill
	f.format = format
	return f.result, f.err
}

type fakeExportResolver struct{}

func (fakeExportResolver) ResolveCredentials(context.Context, string) (*credential.ModelCredentials, error) {
	return nil, nil
}

func (fakeExportResolver) ResolveSynthesisCredentials(context.Context) (*credential.ModelCredentials, error) {
	return nil, nil
}

func (fakeExportResolver) ResolveFusionCredentials(context.Context) (*credential.ModelCredentials, error) {
	return nil, nil
}

func setupSkillExportRouter(db *gorm.DB, client nudist.Client) (*gin.Engine, *fakeExportClient) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	fake, _ := client.(*fakeExportClient)
	inventory := pill_inventory_service.New(db, time.Now)
	h := New(distillation_service.New(client, fakeExportResolver{}, inventory))
	r.POST("/api/v1/distillation/skill-export", h.SkillExport)
	return r, fake
}

// postSkillExport 发起导出请求,返回 (状态码, 原始响应体)
func postSkillExport(t *testing.T, r *gin.Engine, body string) (int, []byte, http.Header) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/distillation/skill-export", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes(), w.Header()
}

func decodeEnvelope(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("响应不是 JSON 信封: %v, body=%s", err, raw)
	}
	return envelope
}

const validStructuredBody = `{
	"skill": {
		"name": "结构化金丹",
		"description": "一份结构化的语言风格技能包",
		"skillSchema": {"identity_card": "我是金丹"},
		"tags": ["语言"],
		"sources": [{"title": "公开资料", "url": "https://example.com/intro", "dimension": "decision_heuristics"}],
		"generatedAt": "2026-08-27T10:00:00Z"
	},
	"format": "codex"
}`

func TestSkillExport_StructuredModeReturnsZip(t *testing.T) {
	db := setupTestDB(t)
	client := &fakeExportClient{result: &nudist.ExportResult{Filename: "alchemy-skill-structure-codex.zip", Content: []byte("PK\x03\x04zip")}}
	r, fake := setupSkillExportRouter(db, client)

	status, raw, headers := postSkillExport(t, r, validStructuredBody)

	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body=%s", status, raw)
	}
	if headers.Get("Content-Type") != "application/zip" {
		t.Fatalf("Content-Type = %q, want application/zip", headers.Get("Content-Type"))
	}
	if headers.Get("Content-Disposition") != `attachment; filename="alchemy-skill-structure-codex.zip"` {
		t.Fatalf("Content-Disposition = %q", headers.Get("Content-Disposition"))
	}
	if string(raw) != "PK\x03\x04zip" {
		t.Fatalf("ZIP 字节 = %q", raw)
	}
	if fake.skill == nil || fake.skill.Name != "结构化金丹" || fake.format != "codex" {
		t.Fatalf("service 未透传 skill/format: skill=%+v format=%q", fake.skill, fake.format)
	}
}

// seedRecipe 造一份丹方 + n 个版本(Revision 1..n),当前版本指向最新,返回 (丹方, 版本列表)
func seedRecipe(t *testing.T, db *gorm.DB, n int) (model.PillRecipe, []model.PillRecipeRevision) {
	t.Helper()
	recipe := model.PillRecipe{}
	if err := db.Create(&recipe).Error; err != nil {
		t.Fatalf("建丹方失败: %v", err)
	}
	revs := make([]model.PillRecipeRevision, 0, n)
	for i := 1; i <= n; i++ {
		rev := model.PillRecipeRevision{
			RecipeID:    recipe.UUID.String(),
			Revision:    i,
			Name:        fmt.Sprintf("丹方 v%d", i),
			Description: fmt.Sprintf("第 %d 版简介", i),
			SkillSchema: model.JSONMap{"identity_card": "x"},
			Tags:        model.JSONList{"语言"},
		}
		if err := db.Create(&rev).Error; err != nil {
			t.Fatalf("建版本失败: %v", err)
		}
		revs = append(revs, rev)
	}
	latest := revs[len(revs)-1]
	if err := db.Model(&recipe).Update("current_revision_id", latest.UUID.String()).Error; err != nil {
		t.Fatalf("指向当前版本失败: %v", err)
	}
	return recipe, revs
}

// TestSkillExport_LegacyPillIDResolvedViaMap 旧 pill ID 只经 LegacyMap 解析到丹方当前版本,
// 不读取可用库存;导出后丹方只读
func TestSkillExport_LegacyPillIDResolvedViaMap(t *testing.T) {
	db := setupTestDB(t)
	recipe, _ := seedRecipe(t, db, 1)
	legacyID := "550e8400-e29b-41d4-a716-446655440000"
	if err := db.Create(&model.PillLegacyMap{
		LegacyKind: "pill", LegacyID: legacyID, TargetUUID: recipe.UUID,
	}).Error; err != nil {
		t.Fatalf("建旧映射失败: %v", err)
	}
	client := &fakeExportClient{result: &nudist.ExportResult{Filename: "alchemy-skill-x-claude.zip", Content: []byte("PK")}}
	r, fake := setupSkillExportRouter(db, client)

	body := fmt.Sprintf(`{"pill_id": %q, "format": "claude"}`, legacyID)
	status, raw, _ := postSkillExport(t, r, body)

	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body=%s", status, raw)
	}
	if fake.skill == nil || fake.skill.Name != "丹方 v1" || fake.skill.Description != "第 1 版简介" {
		t.Fatalf("服务端未按 LegacyMap 解析到丹方版本: %+v", fake.skill)
	}
	if fake.skill.EvidenceLevel != "limited" || fake.skill.GeneratedAt == "" {
		t.Fatalf("版本投影字段异常: %+v", fake.skill)
	}
	// 接口只读: 丹方/版本不得被修改或删除
	var revCount int64
	db.Model(&model.PillRecipeRevision{}).Where("recipe_id = ?", recipe.UUID.String()).Count(&revCount)
	if revCount != 1 {
		t.Fatalf("导出后版本数 = %d, 期望 1", revCount)
	}
}

// TestSkillExport_RecipeIDExportsCurrentRevision recipe_id 单独 → 导出当前版本(v2)
func TestSkillExport_RecipeIDExportsCurrentRevision(t *testing.T) {
	db := setupTestDB(t)
	recipe, _ := seedRecipe(t, db, 2)
	client := &fakeExportClient{result: &nudist.ExportResult{Filename: "x.zip", Content: []byte("PK")}}
	r, fake := setupSkillExportRouter(db, client)

	body := fmt.Sprintf(`{"recipe_id": %q, "format": "codex"}`, recipe.UUID.String())
	status, raw, _ := postSkillExport(t, r, body)

	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body=%s", status, raw)
	}
	if fake.skill == nil || fake.skill.Name != "丹方 v2" {
		t.Fatalf("应导出当前版本 v2: %+v", fake.skill)
	}
}

// TestSkillExport_RevisionIDExportsSpecifiedRevision recipe_id+revision_id → 指定旧版本
func TestSkillExport_RevisionIDExportsSpecifiedRevision(t *testing.T) {
	db := setupTestDB(t)
	recipe, revs := seedRecipe(t, db, 2)
	client := &fakeExportClient{result: &nudist.ExportResult{Filename: "x.zip", Content: []byte("PK")}}
	r, fake := setupSkillExportRouter(db, client)

	body := fmt.Sprintf(`{"recipe_id": %q, "revision_id": %q, "format": "codex"}`,
		recipe.UUID.String(), revs[0].UUID.String())
	status, raw, _ := postSkillExport(t, r, body)

	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body=%s", status, raw)
	}
	if fake.skill == nil || fake.skill.Name != "丹方 v1" {
		t.Fatalf("应导出指定版本 v1: %+v", fake.skill)
	}
}

// TestSkillExport_RevisionOfOtherRecipe404 revision 不属于 URL 的丹方 → 404
func TestSkillExport_RevisionOfOtherRecipe404(t *testing.T) {
	db := setupTestDB(t)
	recipeA, _ := seedRecipe(t, db, 1)
	_, revsB := seedRecipe(t, db, 1)
	r, _ := setupSkillExportRouter(db, &fakeExportClient{})

	body := fmt.Sprintf(`{"recipe_id": %q, "revision_id": %q, "format": "codex"}`,
		recipeA.UUID.String(), revsB[0].UUID.String())
	status, raw, _ := postSkillExport(t, r, body)

	if status != http.StatusNotFound {
		t.Fatalf("跨丹方版本期望 404, 实际 %d, body=%s", status, raw)
	}
}

// TestSkillExport_UnknownRecipe404 未知丹方 → 404
func TestSkillExport_UnknownRecipe404(t *testing.T) {
	db := setupTestDB(t)
	r, _ := setupSkillExportRouter(db, &fakeExportClient{})

	body := fmt.Sprintf(`{"recipe_id": %q, "format": "codex"}`, uuid.NewString())
	status, raw, _ := postSkillExport(t, r, body)

	if status != http.StatusNotFound {
		t.Fatalf("未知丹方期望 404, 实际 %d, body=%s", status, raw)
	}
}

func TestSkillExport_InvalidFormat(t *testing.T) {
	db := setupTestDB(t)
	r, _ := setupSkillExportRouter(db, &fakeExportClient{})

	status, raw, _ := postSkillExport(t, r, `{"skill": {"name": "金丹", "description": "d", "skillSchema": {"identity_card": "x"}, "generatedAt": "2026-08-27T10:00:00Z"}, "format": "yaml"}`)

	if status != http.StatusBadRequest {
		t.Fatalf("期望 400, 实际 %d, body=%s", status, raw)
	}
}

func TestSkillExport_MissingTarget(t *testing.T) {
	db := setupTestDB(t)
	r, _ := setupSkillExportRouter(db, &fakeExportClient{})

	status, raw, _ := postSkillExport(t, r, `{"format": "codex"}`)

	if status != http.StatusBadRequest {
		t.Fatalf("期望 400, 实际 %d, body=%s", status, raw)
	}
}

func TestSkillExport_InvalidStructuredFields(t *testing.T) {
	db := setupTestDB(t)
	r, _ := setupSkillExportRouter(db, &fakeExportClient{})

	status, raw, _ := postSkillExport(t, r, `{
		"skill": {"name": "金丹", "description": "", "skillSchema": {"identity_card": "x"}, "generatedAt": "2026-08-27T10:00:00Z"},
		"format": "codex"
	}`)

	if status != http.StatusBadRequest {
		t.Fatalf("期望 400, 实际 %d, body=%s", status, raw)
	}
	envelope := decodeEnvelope(t, raw)
	if envelope["error_code"] != "service.skill_export.invalid" {
		t.Fatalf("error_code = %v, want service.skill_export.invalid", envelope["error_code"])
	}
}

func TestSkillExport_CredentialFieldsForbidden(t *testing.T) {
	db := setupTestDB(t)
	r, _ := setupSkillExportRouter(db, &fakeExportClient{})

	for _, body := range []string{
		`{"pill_id": "550e8400-e29b-41d4-a716-446655440000", "format": "codex", "api_key": "sk-secret"}`,
		`{"format": "codex", "token": "abc"}`,
	} {
		status, raw, _ := postSkillExport(t, r, body)
		if status != http.StatusForbidden {
			t.Fatalf("body=%s 期望 403, 实际 %d, resp=%s", body, status, raw)
		}
		envelope := decodeEnvelope(t, raw)
		if envelope["error_code"] != "skill_export_forbidden" {
			t.Fatalf("error_code = %v, want skill_export_forbidden", envelope["error_code"])
		}
	}
}

// TestSkillExport_PillNotFound 旧 pill ID 无 LegacyMap 映射 → 404,不读取可用库存
func TestSkillExport_PillNotFound(t *testing.T) {
	db := setupTestDB(t)
	r, _ := setupSkillExportRouter(db, &fakeExportClient{})

	body := fmt.Sprintf(`{"pill_id": %q, "format": "codex"}`, uuid.NewString())
	status, raw, _ := postSkillExport(t, r, body)

	if status != http.StatusNotFound {
		t.Fatalf("期望 404, 实际 %d, body=%s", status, raw)
	}
}

func TestSkillExport_RemoteUnavailableRetryable(t *testing.T) {
	db := setupTestDB(t)
	client := &fakeExportClient{err: &nudist.RemoteError{
		Status:    http.StatusServiceUnavailable,
		Code:      "skill_export_unavailable",
		Stage:     "export",
		Message:   "导出服务暂不可用",
		Retryable: true,
		Details:   map[string]any{"reason": "engine down"},
	}}
	r, _ := setupSkillExportRouter(db, client)

	status, raw, _ := postSkillExport(t, r, validStructuredBody)

	if status != http.StatusServiceUnavailable {
		t.Fatalf("期望 503, 实际 %d, body=%s", status, raw)
	}
	envelope := decodeEnvelope(t, raw)
	if envelope["error_code"] != "skill_export_unavailable" {
		t.Fatalf("error_code = %v, want skill_export_unavailable", envelope["error_code"])
	}
	if envelope["message"] != "导出服务暂不可用" {
		t.Fatalf("message = %v, want 公开 message 保留", envelope["message"])
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok || data["retryable"] != true || data["stage"] != "export" {
		t.Fatalf("data = %v, want {retryable:true stage:export}", envelope["data"])
	}
}
