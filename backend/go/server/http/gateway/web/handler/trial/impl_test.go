// 试丹 handler 回归测试: 请求解析 + 真实 sqlite 库存 + 真实 handler + fake 合成客户端。
// 契约(任务 5): 试丹从指定丹方版本或未保存草稿构建临时输入,不消耗金丹、不写 AgentPillEffect。
package trial

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	iservice "github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/service/pill_inventory_service"
	"github.com/alchemy-furnace/server/internal/service/trial_service"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/alchemy-furnace/server/server/http/router"
)

// ---------- 解析单元测试 ----------

func TestParsePillInputs(t *testing.T) {
	goodUUID := uuid.NewString()
	cases := []struct {
		name    string
		input   PillInput
		want    func(t *testing.T, item iservice.TrialPillInput)
		errCode string
	}{
		{
			name:  "丹方 recipe_id",
			input: PillInput{RecipeID: goodUUID},
			want: func(t *testing.T, item iservice.TrialPillInput) {
				if item.RecipeID.String() != goodUUID || item.RevisionID != uuid.Nil {
					t.Fatalf("解析错误: %+v", item)
				}
			},
		},
		{
			name:  "指定版本 recipe_id+revision_id",
			input: PillInput{RecipeID: goodUUID, RevisionID: uuid.NewString()},
			want: func(t *testing.T, item iservice.TrialPillInput) {
				if item.RecipeID.String() != goodUUID || item.RevisionID == uuid.Nil {
					t.Fatalf("解析错误: %+v", item)
				}
			},
		},
		{
			name:  "未保存草稿",
			input: PillInput{Name: " 草稿金丹 ", SkillSchema: model.JSONMap{"identity_card": "x"}},
			want: func(t *testing.T, item iservice.TrialPillInput) {
				if item.Draft == nil || item.Draft.Name != "草稿金丹" || item.Draft.SkillSchema["identity_card"] != "x" {
					t.Fatalf("解析错误: %+v", item)
				}
			},
		},
		{name: "无目标", input: PillInput{}, errCode: "handler.trial.input_target"},
		{name: "双重目标", input: PillInput{RecipeID: goodUUID, Name: "草稿", SkillSchema: model.JSONMap{"identity_card": "x"}}, errCode: "handler.trial.input_target"},
		{name: "版本无丹方", input: PillInput{RevisionID: goodUUID}, errCode: "handler.trial.revision_requires_recipe"},
		{name: "非法 recipe_id", input: PillInput{RecipeID: "not-a-uuid"}, errCode: "handler.trial.recipe_id_parse"},
		{name: "非法 revision_id", input: PillInput{RecipeID: goodUUID, RevisionID: "not-a-uuid"}, errCode: "handler.trial.revision_id_parse"},
		{name: "草稿缺 schema", input: PillInput{Name: "只有名字"}, errCode: "handler.trial.draft_invalid"},
		{name: "草稿缺名字", input: PillInput{SkillSchema: model.JSONMap{}}, errCode: "handler.trial.draft_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, aerr := parsePillInputs([]PillInput{tc.input})
			if tc.errCode != "" {
				if aerr == nil {
					t.Fatal("期望报错,实际通过")
				}
				if aerr.GetCode() != tc.errCode {
					t.Fatalf("error_code = %q, want %q", aerr.GetCode(), tc.errCode)
				}
				return
			}
			if aerr != nil {
				t.Fatalf("解析失败: %v", aerr)
			}
			tc.want(t, got[0])
		})
	}
}

// ---------- HTTP 集成测试 ----------

// setupTrialTestDB 初始化 sqlite 内存库(完整库存模型)
func setupTrialTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:trial%d?mode=memory&cache=shared", time.Now().UnixNano())
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
		&model.PillRecipe{}, &model.PillRecipeRevision{},
		&model.PillItem{}, &model.AgentPillEffect{}, &model.PillOperation{},
		&model.FusionPreview{}, &model.PillStarterGrant{},
	); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	return db
}

type fakeTrialSynth struct {
	synthesis.Client
	received []synthesis.PillInput
	resp     *synthesis.CombineResponse
}

func (f *fakeTrialSynth) Combine(ctx context.Context, personality string, pills []synthesis.PillInput, creds *credential.ModelCredentials) (*synthesis.CombineResponse, error) {
	f.received = pills
	if f.resp == nil {
		f.resp = &synthesis.CombineResponse{}
	}
	return f.resp, nil
}

type fakeTrialCreds struct{ credential.Resolver }

func (fakeTrialCreds) ResolveSynthesisCredentials(context.Context) (*credential.ModelCredentials, error) {
	return &credential.ModelCredentials{Model: "fake"}, nil
}

func setupTrialRouter(db *gorm.DB, synth synthesis.Client) (*gin.Engine, *fakeTrialSynth) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	fake, _ := synth.(*fakeTrialSynth)
	inventory := pill_inventory_service.New(db, time.Now)
	h := New(trial_service.New(inventory, synth, fakeTrialCreds{}))
	r.POST("/api/v1/trial/synthesis", router.Wrapper(h.Synthesize))
	return r, fake
}

// seedTrialRecipe 造一份丹方 + n 个版本,当前指向最新;返回 (丹方, 版本列表)
func seedTrialRecipe(t *testing.T, db *gorm.DB, n int) (model.PillRecipe, []model.PillRecipeRevision) {
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
			SkillSchema: model.JSONMap{"identity_card": fmt.Sprintf("我是v%d", i)},
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

func postTrialSynthesis(t *testing.T, r *gin.Engine, body string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trial/synthesis", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

// TestSynthesizeRoute_RecipeMode 丹方当前版本试丹: 200 且提示词含版本内容
func TestSynthesizeRoute_RecipeMode(t *testing.T) {
	db := setupTrialTestDB(t)
	recipe, revs := seedTrialRecipe(t, db, 2)
	r, fake := setupTrialRouter(db, &fakeTrialSynth{})

	body := fmt.Sprintf(`{"personality":"沉稳内敛","pills":[{"recipe_id":%q}]}`, recipe.UUID.String())
	status, raw := postTrialSynthesis(t, r, body)

	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body=%s", status, raw)
	}
	var envelope struct {
		Data struct {
			SystemPrompt string `json:"system_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("响应不是 JSON 信封: %v, body=%s", err, raw)
	}
	if !bytes.Contains([]byte(envelope.Data.SystemPrompt), []byte("我是v2")) {
		t.Errorf("应取当前版本 v2: %q", envelope.Data.SystemPrompt)
	}
	if len(fake.received) != 1 || fake.received[0].ID != revs[1].UUID.String() {
		t.Fatalf("透传给合成引擎的输入错误: %+v", fake.received)
	}
}

// TestSynthesizeRoute_RevisionMode 指定旧版本试丹
func TestSynthesizeRoute_RevisionMode(t *testing.T) {
	db := setupTrialTestDB(t)
	recipe, revs := seedTrialRecipe(t, db, 2)
	r, fake := setupTrialRouter(db, &fakeTrialSynth{})

	body := fmt.Sprintf(`{"personality":"沉稳内敛","pills":[{"recipe_id":%q,"revision_id":%q}]}`,
		recipe.UUID.String(), revs[0].UUID.String())
	status, raw := postTrialSynthesis(t, r, body)

	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body=%s", status, raw)
	}
	if fake.received[0].ID != revs[0].UUID.String() {
		t.Fatalf("应引用指定版本 v1: %+v", fake.received)
	}
}

// TestSynthesizeRoute_DraftMode 未保存草稿试丹: 不落库直接合成
func TestSynthesizeRoute_DraftMode(t *testing.T) {
	db := setupTrialTestDB(t)
	r, fake := setupTrialRouter(db, &fakeTrialSynth{})

	body := `{"personality":"沉稳内敛","pills":[{"name":"草稿金丹","skill_schema":{"identity_card":"我是草稿"}}]}`
	status, raw := postTrialSynthesis(t, r, body)

	if status != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d, body=%s", status, raw)
	}
	if fake.received[0].ID != "" || fake.received[0].Name != "草稿金丹" {
		t.Fatalf("草稿应内联透传: %+v", fake.received)
	}
	// 草稿不落库
	var recipeCount int64
	db.Model(&model.PillRecipe{}).Count(&recipeCount)
	if recipeCount != 0 {
		t.Fatalf("草稿试丹不得落库,丹方数 = %d", recipeCount)
	}
}

// TestSynthesizeRoute_InvalidTarget 多目标 → 400 handler.trial.input_target
func TestSynthesizeRoute_InvalidTarget(t *testing.T) {
	db := setupTrialTestDB(t)
	r, _ := setupTrialRouter(db, &fakeTrialSynth{})

	body := fmt.Sprintf(`{"personality":"沉稳内敛","pills":[{"recipe_id":%q,"name":"草稿","skill_schema":{"identity_card":"x"}}]}`, uuid.NewString())
	status, raw := postTrialSynthesis(t, r, body)

	if status != http.StatusBadRequest {
		t.Fatalf("期望 400, 实际 %d, body=%s", status, raw)
	}
	var envelope map[string]interface{}
	_ = json.Unmarshal(raw, &envelope)
	if envelope["error_code"] != "handler.trial.input_target" {
		t.Fatalf("error_code = %v, want handler.trial.input_target", envelope["error_code"])
	}
}

// TestSynthesizeRoute_DoesNotConsume 试丹是模拟: 任何模式下都不写库存/能力/操作
func TestSynthesizeRoute_DoesNotConsume(t *testing.T) {
	db := setupTrialTestDB(t)
	recipe, _ := seedTrialRecipe(t, db, 1)
	r, _ := setupTrialRouter(db, &fakeTrialSynth{})

	bodies := []string{
		fmt.Sprintf(`{"personality":"沉稳内敛","pills":[{"recipe_id":%q}]}`, recipe.UUID.String()),
		`{"personality":"沉稳内敛","pills":[{"name":"草稿","skill_schema":{"identity_card":"x"}}]}`,
	}
	for _, body := range bodies {
		if status, raw := postTrialSynthesis(t, r, body); status != http.StatusOK {
			t.Fatalf("body=%s 期望 200, 实际 %d, resp=%s", body, status, raw)
		}
	}

	for _, m := range []interface{}{&model.PillItem{}, &model.AgentPillEffect{}, &model.PillOperation{}} {
		var count int64
		if err := db.Model(m).Count(&count).Error; err != nil {
			t.Fatalf("统计 %T 失败: %v", m, err)
		}
		if count != 0 {
			t.Errorf("试丹不得写 %T,实际 %d 行", m, count)
		}
	}
}
