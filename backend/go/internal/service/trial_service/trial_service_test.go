package trial_service

import (
	"context"
	"encoding/json"
	std "errors"
	"io"
	"net/http"
	"strings"
	"testing"

	appErrors "github.com/alchemy-furnace/server/internal/errors"
	iservice "github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// fakeInventory 丹方库存读接口桩(试丹只用 GetRecipe/GetRecipeRevision;
// 其余方法由嵌入接口兜底,误用即 nil panic)
type fakeInventory struct {
	iservice.PillInventory
	recipes   map[string]*model.PillRecipe
	revisions map[string]*model.PillRecipeRevision
	byUID     map[string]*model.PillRecipeRevision // 版本 UUID 文本 → 版本(当前版本指针已翻转为 UUID 文本)
}

func (f *fakeInventory) GetRecipe(ctx context.Context, uid uuid.UUID) (*model.PillRecipe, *model.PillRecipeRevision, appErrors.Error) {
	recipe, ok := f.recipes[uid.String()]
	if !ok || recipe.CurrentRevisionID == nil {
		return nil, nil, appErrors.ErrorRecordNotFound("recipe.not_found")
	}
	rev, ok := f.byUID[*recipe.CurrentRevisionID]
	if !ok {
		return nil, nil, appErrors.ErrorRecordNotFound("recipe.revision_not_found")
	}
	return recipe, rev, nil
}

func (f *fakeInventory) GetRecipeRevision(ctx context.Context, recipeID, revisionID uuid.UUID) (*model.PillRecipeRevision, appErrors.Error) {
	rev, ok := f.revisions[revisionID.String()]
	recipe, rOK := f.recipes[recipeID.String()]
	if !ok || !rOK || rev.RecipeID != recipe.PillRecipeID {
		return nil, appErrors.ErrorRecordNotFound("recipe.revision_not_found")
	}
	return rev, nil
}

type fakeSynth struct {
	synthesis.Client
	resp *synthesis.CombineResponse
	err  error
}

func (f *fakeSynth) Combine(ctx context.Context, personality string, pills []synthesis.PillInput, creds *credential.ModelCredentials) (*synthesis.CombineResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

type fakeCreds struct {
	credential.Resolver
}

func (f *fakeCreds) ResolveSynthesisCredentials(ctx context.Context) (*credential.ModelCredentials, error) {
	return &credential.ModelCredentials{Model: "fake"}, nil
}

func (f *fakeCreds) ResolveCredentials(ctx context.Context, name string) (*credential.ModelCredentials, error) {
	return &credential.ModelCredentials{Model: name}, nil
}

type fakeHTTP struct {
	lastBody string
}

func (f *fakeHTTP) Do(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	f.lastBody = string(body)
	return &http.Response{
		StatusCode: 200,
		Body: io.NopCloser(strings.NewReader(
			`{"code":0,"message":"success","data":{"content":"回复内容","model":"m","usage":{}}}`,
		)),
	}, nil
}

// ---------- 夹具 ----------

const (
	recipeUUIDStr = "11111111-1111-4111-8111-111111111111"
	rev1UUIDStr   = "22222222-2222-4222-8222-222222222222"
	rev2UUIDStr   = "33333333-3333-4333-8333-333333333333"
)

// trialMarkerSchema 带全部已知字段 + 未知字段标记的能力内容
func trialMarkerSchema(versionMarker string) model.JSONMap {
	return model.JSONMap{
		"identity_card":       "IDENTITY_MARKER",
		"description":         "DESCRIPTION_MARKER",
		"expression_dna":      model.JSONMap{"vocabulary": "DNA_MARKER"},
		"mental_models":       []any{map[string]any{"name": "MENTAL_MODEL_MARKER"}},
		"decision_heuristics": []any{map[string]any{"condition": "HEURISTIC_MARKER"}},
		"values":              []any{"VALUE_MARKER"},
		"anti_patterns":       []any{"ANTI_PATTERN_MARKER"},
		"honest_limits":       []any{"HONEST_LIMIT_MARKER"},
		"example_dialogues":   []any{map[string]any{"user": "EXAMPLE_MARKER"}},
		"future_key_2026":     "UNKNOWN_FIELD_MARKER",
		"version_marker":      versionMarker,
	}
}

// trialMarkers 静态提示词应渲染的标记。心智模型/决策启发式/示例对话自 e76f6fc 起
// 移出永久区(仅入 P2 激活区或 few-shot,见 behavior/render_test.go),不再断言出现。
var trialMarkers = []string{
	"IDENTITY_MARKER", "DNA_MARKER", "VALUE_MARKER", "ANTI_PATTERN_MARKER",
	"HONEST_LIMIT_MARKER", "UNKNOWN_FIELD_MARKER",
}

// trialInventory 造一份丹方: v1/v2 两个版本,当前指向 v2
func trialInventory() *fakeInventory {
	recipeUID := recipeUUIDStr
	rev1 := &model.PillRecipeRevision{
		PillRecipeRevisionID: rev1UUIDStr, RecipeID: recipeUID, Revision: 1,
		Name: "丹方 v1", SkillSchema: trialMarkerSchema("V1_MARKER"),
	}
	rev2 := &model.PillRecipeRevision{
		PillRecipeRevisionID: rev2UUIDStr, RecipeID: recipeUID, Revision: 2,
		Name: "丹方 v2", SkillSchema: trialMarkerSchema("V2_MARKER"),
	}
	curUID := rev2.PillRecipeRevisionID
	return &fakeInventory{
		recipes:   map[string]*model.PillRecipe{recipeUID: {PillRecipeID: recipeUID, CurrentRevisionID: &curUID}},
		revisions: map[string]*model.PillRecipeRevision{rev1UUIDStr: rev1, rev2UUIDStr: rev2},
		byUID:     map[string]*model.PillRecipeRevision{rev1UUIDStr: rev1, rev2UUIDStr: rev2},
	}
}

func newTrialService(inv iservice.PillInventory, synth synthesis.Client) *Trial {
	return &Trial{
		inventory:  inv,
		synthesis:  synth,
		credential: &fakeCreds{},
		httpClient: &fakeHTTP{},
	}
}

func assertTrialPromptMarkers(t *testing.T, prompt string) {
	t.Helper()
	for _, marker := range trialMarkers {
		if !strings.Contains(prompt, marker) {
			t.Errorf("提示词缺少标记 %s", marker)
		}
	}
}

// ---------- 用例 ----------

// TestSynthesizeRendersLosslessPrompt 试丹合成: recipe_id 取当前版本,渲染提示词含全部金丹字段 + 涌现规则
func TestSynthesizeRendersLosslessPrompt(t *testing.T) {
	svc := newTrialService(trialInventory(), &fakeSynth{resp: &synthesis.CombineResponse{
		EmergenceRules: model.JSONList{"涌现规则甲"},
		Fingerprint:    "sha256:fp",
		Model:          "fake",
	}})

	result, err := svc.Synthesize(context.Background(), "沉稳内敛", []iservice.TrialPillInput{
		{RecipeID: uuid.MustParse(recipeUUIDStr), Weight: 1.0, SortOrder: 0},
	}, "")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	assertTrialPromptMarkers(t, result.SystemPrompt)
	if !strings.Contains(result.SystemPrompt, "V2_MARKER") {
		t.Error("recipe_id 单独应取当前版本 v2")
	}
	if !strings.Contains(result.SystemPrompt, "〔综合表达规则〕") || !strings.Contains(result.SystemPrompt, "涌现规则甲") {
		t.Error("涌现规则必须渲染进提示词")
	}
	if result.Degraded {
		t.Error("非降级路径 Degraded 应为 false")
	}
	if result.Fingerprint != "sha256:fp" || result.Model != "fake" {
		t.Errorf("Fingerprint/Model 透传错误: %+v", result)
	}
}

// TestSynthesizeDegradesOnCombineError 合成失败:返回无损渲染(degraded),不返回错误
func TestSynthesizeDegradesOnCombineError(t *testing.T) {
	svc := newTrialService(trialInventory(), &fakeSynth{err: std.New("engine down")})

	result, err := svc.Synthesize(context.Background(), "沉稳内敛", []iservice.TrialPillInput{
		{RecipeID: uuid.MustParse(recipeUUIDStr), Weight: 1.0, SortOrder: 0},
	}, "")
	if err != nil {
		t.Fatalf("合成失败不应返回错误: %v", err)
	}
	if !result.Degraded || result.DegradedReason != "combine_error" {
		t.Errorf("降级标记错误: %+v", result)
	}
	assertTrialPromptMarkers(t, result.SystemPrompt)
	if strings.Contains(result.SystemPrompt, "〔综合表达规则〕") {
		t.Error("失败路径不应有涌现规则子节")
	}
}

// TestChatUsesRenderedSystemPrompt 试丹对话:system 消息是行为引擎渲染的提示词(含全部标记)
func TestChatUsesRenderedSystemPrompt(t *testing.T) {
	fakeHTTPClient := &fakeHTTP{}
	svc := &Trial{
		inventory:  trialInventory(),
		synthesis:  &fakeSynth{resp: &synthesis.CombineResponse{EmergenceRules: model.JSONList{"涌现规则甲"}}},
		credential: &fakeCreds{},
		httpClient: fakeHTTPClient,
	}

	resp, err := svc.Chat(context.Background(), &iservice.TrialChatRequest{
		Personality: "沉稳内敛",
		Pills:       []iservice.TrialPillInput{{RecipeID: uuid.MustParse(recipeUUIDStr), Weight: 1.0, SortOrder: 0}},
		Messages:    []map[string]string{{"role": "user", "content": "求道"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "回复内容" {
		t.Errorf("Content = %q", resp.Content)
	}

	var sent struct {
		Messages []map[string]string `json:"messages"`
	}
	if err := json.Unmarshal([]byte(fakeHTTPClient.lastBody), &sent); err != nil {
		t.Fatalf("解析发送体失败: %v", err)
	}
	if len(sent.Messages) != 2 || sent.Messages[0]["role"] != "system" {
		t.Fatalf("消息结构错误: %+v", sent.Messages)
	}
	assertTrialPromptMarkers(t, sent.Messages[0]["content"])
	if !strings.Contains(sent.Messages[0]["content"], "涌现规则甲") {
		t.Error("system 提示词应含涌现规则")
	}
}

// TestSynthesizeFromSpecifiedRevision recipe_id+revision_id 引用指定旧版本(v1),当前 v2 不影响
func TestSynthesizeFromSpecifiedRevision(t *testing.T) {
	svc := newTrialService(trialInventory(), &fakeSynth{resp: &synthesis.CombineResponse{}})

	result, err := svc.Synthesize(context.Background(), "沉稳内敛", []iservice.TrialPillInput{
		{RecipeID: uuid.MustParse(recipeUUIDStr), RevisionID: uuid.MustParse(rev1UUIDStr), SortOrder: 0},
	}, "")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if !strings.Contains(result.SystemPrompt, "V1_MARKER") {
		t.Error("应引用指定版本 v1")
	}
	if strings.Contains(result.SystemPrompt, "V2_MARKER") {
		t.Error("指定 v1 时不得混入当前 v2 内容")
	}
}

// TestSynthesizeFromDraft 未保存草稿内联内容试丹:不查询库存
func TestSynthesizeFromDraft(t *testing.T) {
	// 空库存:若服务误走 DB 路径必然 404,草稿模式必须完全内联
	svc := newTrialService(&fakeInventory{}, &fakeSynth{resp: &synthesis.CombineResponse{}})

	result, err := svc.Synthesize(context.Background(), "沉稳内敛", []iservice.TrialPillInput{
		{Draft: &iservice.TrialPillDraft{Name: "草稿金丹", SkillSchema: trialMarkerSchema("DRAFT_MARKER")}, SortOrder: 0},
	}, "")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	assertTrialPromptMarkers(t, result.SystemPrompt)
	if !strings.Contains(result.SystemPrompt, "DRAFT_MARKER") {
		t.Error("草稿内容必须渲染进提示词")
	}
}

// TestSynthesizeRejectsNoTarget 无目标的输入 → 400
func TestSynthesizeRejectsNoTarget(t *testing.T) {
	svc := newTrialService(trialInventory(), &fakeSynth{resp: &synthesis.CombineResponse{}})

	_, err := svc.Synthesize(context.Background(), "沉稳内敛", []iservice.TrialPillInput{{SortOrder: 0}}, "")
	if err == nil {
		t.Fatal("无目标输入应报错")
	}
	if appErrors.HTTPStatus(err) != 400 {
		t.Errorf("HTTP 状态 = %d, want 400", appErrors.HTTPStatus(err))
	}
}

// TestSynthesizeRejectsRevisionWithoutRecipe 指定版本必须携带所属丹方
func TestSynthesizeRejectsRevisionWithoutRecipe(t *testing.T) {
	svc := newTrialService(trialInventory(), &fakeSynth{resp: &synthesis.CombineResponse{}})

	_, err := svc.Synthesize(context.Background(), "沉稳内敛", []iservice.TrialPillInput{
		{RevisionID: uuid.MustParse(rev1UUIDStr), SortOrder: 0},
	}, "")
	if err == nil {
		t.Fatal("只有版本没有丹方应报错")
	}
	if appErrors.HTTPStatus(err) != 400 {
		t.Errorf("HTTP 状态 = %d, want 400", appErrors.HTTPStatus(err))
	}
}

// TestSynthesizeUnknownRecipe recipe_id 不存在 → 404(服务层透传,不猜测默认内容)
func TestSynthesizeUnknownRecipe(t *testing.T) {
	svc := newTrialService(trialInventory(), &fakeSynth{resp: &synthesis.CombineResponse{}})

	_, err := svc.Synthesize(context.Background(), "沉稳内敛", []iservice.TrialPillInput{
		{RecipeID: uuid.New(), SortOrder: 0},
	}, "")
	if err == nil {
		t.Fatal("不存在的丹方应报错")
	}
	if appErrors.HTTPStatus(err) != 404 {
		t.Errorf("HTTP 状态 = %d, want 404", appErrors.HTTPStatus(err))
	}
}

// TestSynthesizeRevisionOfOtherRecipe 版本不属于该丹方 → 404
func TestSynthesizeRevisionOfOtherRecipe(t *testing.T) {
	otherRecipeUUID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	inv := trialInventory()
	rev2UID := rev2UUIDStr
	inv.recipes[otherRecipeUUID.String()] = &model.PillRecipe{
		PillRecipeID: otherRecipeUUID.String(), CurrentRevisionID: &rev2UID,
	}
	svc := newTrialService(inv, &fakeSynth{resp: &synthesis.CombineResponse{}})

	_, err := svc.Synthesize(context.Background(), "沉稳内敛", []iservice.TrialPillInput{
		{RecipeID: otherRecipeUUID, RevisionID: uuid.MustParse(rev1UUIDStr), SortOrder: 0},
	}, "")
	if err == nil {
		t.Fatal("跨丹方版本应报错")
	}
	if appErrors.HTTPStatus(err) != 404 {
		t.Errorf("HTTP 状态 = %d, want 404", appErrors.HTTPStatus(err))
	}
}
