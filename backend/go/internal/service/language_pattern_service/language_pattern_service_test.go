package language_pattern_service

import (
	"context"
	std "errors"
	"reflect"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/behavior"
	appErrors "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/dao"
	"github.com/alchemy-furnace/server/internal/service/credential"
	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// ---------- 测试替身 ----------

type fakeAgentDAO struct {
	dao.Agent
	agent *model.DaoAgent
	saved []*model.LanguagePattern
	// 缓存保护: savedRevisions 记录每次保存携带的 expectedEffectsRevision;
	// conflictRemaining>0 时保存被拒(模拟并发编排变更),onConflict 在拒绝时回调
	savedRevisions    []int
	conflictRemaining int
	onConflict        func()
}

func (f *fakeAgentDAO) TakeAgentDetailByUUID(ctx context.Context, uid uuid.UUID) (*model.DaoAgent, appErrors.Error) {
	if f.agent == nil {
		return nil, appErrors.ErrorRecordNotFound("fake.agent.missing")
	}
	return f.agent, nil
}

func (f *fakeAgentDAO) SaveLanguagePattern(ctx context.Context, pattern *model.LanguagePattern) appErrors.Error {
	f.saved = append(f.saved, pattern)
	return nil
}

func (f *fakeAgentDAO) SaveLanguagePatternIfRevision(ctx context.Context, pattern *model.LanguagePattern, expectedEffectsRevision int) appErrors.Error {
	if f.conflictRemaining > 0 {
		f.conflictRemaining--
		if f.onConflict != nil {
			f.onConflict()
		}
		return appErrors.ErrorConflict("agent.effects_conflict", "fake: effects revision changed")
	}
	f.saved = append(f.saved, pattern)
	f.savedRevisions = append(f.savedRevisions, expectedEffectsRevision)
	return nil
}

type fakeSynthesis struct {
	synthesis.Client
	resp      *synthesis.CombineResponse
	err       error
	calls     int
	lastPills []synthesis.PillInput
	lastCreds *credential.ModelCredentials
}

func (f *fakeSynthesis) Combine(ctx context.Context, personality string, pills []synthesis.PillInput, creds *credential.ModelCredentials) (*synthesis.CombineResponse, error) {
	f.calls++
	f.lastPills = pills
	f.lastCreds = creds
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

type fakeCreds struct {
	credential.Resolver
	err error
}

func (f *fakeCreds) ResolveSynthesisCredentials(ctx context.Context) (*credential.ModelCredentials, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &credential.ModelCredentials{Model: "fake-synthesis-model"}, nil
}

// ---------- 夹具 ----------

func markerSkillSchema() model.JSONMap {
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
	}
}

// newMarkerAgent 构建带已吸收能力(AgentPillEffects 快照)的道人;
// 身份=被服用实例 UUID,内容=NameSnapshot/SchemaSnapshot(任务 3 起的事实来源)
func newMarkerAgent() *model.DaoAgent {
	item := model.PillItem{UUID: uuid.New()}
	return &model.DaoAgent{
		ID:              1,
		Name:            "测试道人",
		Personality:     "沉稳内敛",
		EffectsRevision: 3,
		AgentPillEffects: []model.AgentPillEffect{
			{
				NameSnapshot:   "标记金丹",
				SchemaSnapshot: markerSkillSchema(),
				Weight:         1.0,
				SortOrder:      0,
				Item:           item,
			},
		},
	}
}

// allMarkers 静态提示词应渲染的标记。心智模型/决策启发式/示例对话自 e76f6fc 起
// 移出永久区(仅入 P2 激活区或 few-shot,见 behavior/render_test.go),不再断言出现。
var allMarkers = []string{
	"IDENTITY_MARKER", "DNA_MARKER", "VALUE_MARKER", "ANTI_PATTERN_MARKER",
	"HONEST_LIMIT_MARKER", "UNKNOWN_FIELD_MARKER",
}

func assertPromptHasAllMarkers(t *testing.T, prompt string) {
	t.Helper()
	for _, marker := range allMarkers {
		if !strings.Contains(prompt, marker) {
			t.Errorf("渲染提示词缺少标记 %s;提示词:\n%s", marker, prompt)
		}
	}
}

// ---------- 用例 ----------

// TestGetOrBuildPatternCacheHitNewFormat 新格式缓存(behavior_profile 非空 + 版本匹配 + 指纹一致)直接命中,不调合成
func TestGetOrBuildPatternCacheHitNewFormat(t *testing.T) {
	agent := newMarkerAgent()
	fp, err := computeFingerprint(agent.Personality, buildPillInputs(agent))
	if err != nil {
		t.Fatalf("computeFingerprint: %v", err)
	}
	agent.LanguagePattern = &model.LanguagePattern{
		AgentID:           agent.UUID.String(),
		SystemPrompt:      "缓存的提示词",
		BehaviorProfile:   model.JSONMap{"version": 1},
		ProfileVersion:    behavior.ProfileVersion,
		SourceFingerprint: fp,
		IsValid:           true,
	}
	fakeAgent := &fakeAgentDAO{agent: agent}
	fakeSynth := &fakeSynthesis{}

	svc := New(fakeAgent, fakeSynth, &fakeCreds{})
	got, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String())
	if err != nil {
		t.Fatalf("GetOrBuildPattern: %v", err)
	}
	if got != agent.LanguagePattern {
		t.Error("缓存命中应返回原对象")
	}
	if fakeSynth.calls != 0 {
		t.Errorf("缓存命中不应调用合成,实际 %d 次", fakeSynth.calls)
	}
}

// TestGetOrBuildPatternOldCacheRebuilds 旧缓存(无 behavior_profile)视为失效,自动重建
func TestGetOrBuildPatternOldCacheRebuilds(t *testing.T) {
	agent := newMarkerAgent()
	fp, _ := computeFingerprint(agent.Personality, buildPillInputs(agent))
	agent.LanguagePattern = &model.LanguagePattern{
		AgentID:           agent.UUID.String(),
		SystemPrompt:      "旧格式提示词",
		SourceFingerprint: fp,
		IsValid:           true,
	}
	fakeAgent := &fakeAgentDAO{agent: agent}
	fakeSynth := &fakeSynthesis{resp: &synthesis.CombineResponse{
		EmergenceRules: model.JSONList{"涌现规则甲"},
		InnerTensions:  []synthesis.InnerTension{{Dimension: "formality", Description: "正式程度相冲", Severity: "high"}},
		Fingerprint:    fp,
		Model:          "fake-synthesis-model",
	}}

	svc := New(fakeAgent, fakeSynth, &fakeCreds{})
	got, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String())
	if err != nil {
		t.Fatalf("GetOrBuildPattern: %v", err)
	}
	if fakeSynth.calls != 1 {
		t.Fatalf("旧缓存应触发重建,实际 %d 次", fakeSynth.calls)
	}
	if len(fakeAgent.saved) != 1 {
		t.Fatal("重建结果应落库")
	}
	if got.BehaviorProfile == nil || got.ProfileVersion != behavior.ProfileVersion {
		t.Error("落库缓存缺少新档案字段")
	}
	assertPromptHasAllMarkers(t, got.SystemPrompt)
	if !strings.Contains(got.SystemPrompt, "涌现规则甲") {
		t.Error("涌现规则必须渲染进提示词")
	}
}

// TestGetOrBuildPatternFingerprintMismatchRebuilds 指纹不一致触发重建
func TestGetOrBuildPatternFingerprintMismatchRebuilds(t *testing.T) {
	agent := newMarkerAgent()
	agent.LanguagePattern = &model.LanguagePattern{
		AgentID:           agent.UUID.String(),
		SystemPrompt:      "过期缓存",
		BehaviorProfile:   model.JSONMap{"version": 1},
		ProfileVersion:    behavior.ProfileVersion,
		SourceFingerprint: "sha256:stale",
		IsValid:           true,
	}
	fakeAgent := &fakeAgentDAO{agent: agent}
	fakeSynth := &fakeSynthesis{resp: &synthesis.CombineResponse{
		EmergenceRules: model.JSONList{},
		Fingerprint:    "sha256:stale",
		Model:          "fake-synthesis-model",
	}}

	svc := New(fakeAgent, fakeSynth, &fakeCreds{})
	_, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String())
	if err != nil {
		t.Fatalf("GetOrBuildPattern: %v", err)
	}
	if fakeSynth.calls != 1 {
		t.Errorf("指纹不一致应重建,实际 %d 次", fakeSynth.calls)
	}
}

// TestGetOrBuildPatternProfileVersionMismatchRebuilds 档案版本不一致触发重建
func TestGetOrBuildPatternProfileVersionMismatchRebuilds(t *testing.T) {
	agent := newMarkerAgent()
	fp, _ := computeFingerprint(agent.Personality, buildPillInputs(agent))
	agent.LanguagePattern = &model.LanguagePattern{
		AgentID:           agent.UUID.String(),
		SystemPrompt:      "旧版本档案",
		BehaviorProfile:   model.JSONMap{"version": 0},
		ProfileVersion:    0,
		SourceFingerprint: fp,
		IsValid:           true,
	}
	fakeAgent := &fakeAgentDAO{agent: agent}
	fakeSynth := &fakeSynthesis{resp: &synthesis.CombineResponse{
		EmergenceRules: model.JSONList{},
		Fingerprint:    fp,
		Model:          "fake-synthesis-model",
	}}

	svc := New(fakeAgent, fakeSynth, &fakeCreds{})
	if _, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String()); err != nil {
		t.Fatalf("GetOrBuildPattern: %v", err)
	}
	if fakeSynth.calls != 1 {
		t.Errorf("版本不一致应重建,实际 %d 次", fakeSynth.calls)
	}
}

// TestGetOrBuildPatternSuccessPersistsLossless 合成成功:无损渲染落库,档案与版本齐备
func TestGetOrBuildPatternSuccessPersistsLossless(t *testing.T) {
	agent := newMarkerAgent()
	fakeAgent := &fakeAgentDAO{agent: agent}
	fakeSynth := &fakeSynthesis{resp: &synthesis.CombineResponse{
		EmergenceRules: model.JSONList{"涌现规则甲", "涌现规则乙"},
		InnerTensions:  []synthesis.InnerTension{{Dimension: "sentence_length", Description: "句式相冲", Severity: "medium"}},
		Fingerprint:    "sha256:fp",
		Model:          "fake-synthesis-model",
	}}

	svc := New(fakeAgent, fakeSynth, &fakeCreds{})
	got, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String())
	if err != nil {
		t.Fatalf("GetOrBuildPattern: %v", err)
	}

	if len(fakeAgent.saved) != 1 {
		t.Fatal("成功路径应落库")
	}
	saved := fakeAgent.saved[0]
	if !saved.IsValid {
		t.Error("成功路径 IsValid 应为 true")
	}
	if saved.ProfileVersion != behavior.ProfileVersion {
		t.Errorf("ProfileVersion = %d, want %d", saved.ProfileVersion, behavior.ProfileVersion)
	}
	if saved.BehaviorProfile == nil {
		t.Error("BehaviorProfile 必须写入")
	}
	if len(saved.EmergenceRules) != 2 {
		t.Errorf("EmergenceRules = %+v", saved.EmergenceRules)
	}
	if !strings.Contains(saved.SystemPrompt, "〔冲突调和〕") || !strings.Contains(saved.SystemPrompt, "句式相冲") {
		t.Error("冲突调和建议必须渲染")
	}
	assertPromptHasAllMarkers(t, saved.SystemPrompt)
	if got != saved {
		t.Error("返回值应为落库对象")
	}
}

// TestGetOrBuildPatternDegradedNotPersisted 降级(无凭证/涌现不可用):无损渲染返回,不落库
func TestGetOrBuildPatternDegradedNotPersisted(t *testing.T) {
	agent := newMarkerAgent()
	fakeAgent := &fakeAgentDAO{agent: agent}
	fakeSynth := &fakeSynthesis{resp: &synthesis.CombineResponse{
		EmergenceRules: model.JSONList{},
		Degraded:       true,
		DegradedReason: "no_credentials",
	}}

	svc := New(fakeAgent, fakeSynth, &fakeCreds{})
	got, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String())
	if err != nil {
		t.Fatalf("降级路径不应返回错误: %v", err)
	}
	if got.IsValid {
		t.Error("降级结果 IsValid 应为 false(不落库)")
	}
	if len(fakeAgent.saved) != 0 {
		t.Error("降级结果不得落库")
	}
	assertPromptHasAllMarkers(t, got.SystemPrompt)
	if strings.Contains(got.SystemPrompt, "〔涌现规则〕") {
		t.Error("降级路径不应有涌现规则子节")
	}
}

// TestGetOrBuildPatternCombineErrorLosslessTemp 合成调用失败:无损渲染返回,不落库,不返回错误
func TestGetOrBuildPatternCombineErrorLosslessTemp(t *testing.T) {
	agent := newMarkerAgent()
	fakeAgent := &fakeAgentDAO{agent: agent}
	fakeSynth := &fakeSynthesis{err: std.New("python engine down")}

	svc := New(fakeAgent, fakeSynth, &fakeCreds{})
	got, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String())
	if err != nil {
		t.Fatalf("合成失败不应阻断聊天(返回无损渲染): %v", err)
	}
	if got.IsValid {
		t.Error("合成失败结果 IsValid 应为 false")
	}
	if len(fakeAgent.saved) != 0 {
		t.Error("合成失败不得落库")
	}
	assertPromptHasAllMarkers(t, got.SystemPrompt)
}

// TestGetOrBuildPatternNoCredentialsStillCallsCombine 凭证解析失败不阻断:以 nil 凭证调合成
func TestGetOrBuildPatternNoCredentialsStillCallsCombine(t *testing.T) {
	agent := newMarkerAgent()
	fakeAgent := &fakeAgentDAO{agent: agent}
	fakeSynth := &fakeSynthesis{resp: &synthesis.CombineResponse{
		Degraded:       true,
		DegradedReason: "no_credentials",
	}}
	creds := &fakeCreds{err: std.New("no synthesis model configured")}

	svc := New(fakeAgent, fakeSynth, creds)
	if _, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String()); err != nil {
		t.Fatalf("GetOrBuildPattern: %v", err)
	}
	if fakeSynth.calls != 1 {
		t.Fatalf("应调用合成,实际 %d 次", fakeSynth.calls)
	}
	if fakeSynth.lastCreds != nil {
		t.Error("凭证解析失败时应以 nil 凭证调用合成")
	}
}

// ---------- 任务 3: 快照编译输入 + 缓存保护 ----------

// TestGetOrBuildPatternPillInputFromEffectSnapshot 合成输入必须来自已吸收能力快照:
// 身份=被服用实例 UUID,内容=NameSnapshot/SchemaSnapshot;写缓存携带读取时的 EffectsRevision
func TestGetOrBuildPatternPillInputFromEffectSnapshot(t *testing.T) {
	agent := newMarkerAgent()
	fakeAgent := &fakeAgentDAO{agent: agent}
	fakeSynth := &fakeSynthesis{resp: &synthesis.CombineResponse{
		EmergenceRules: model.JSONList{"涌现规则甲"},
		Fingerprint:    "sha256:fp",
		Model:          "fake-synthesis-model",
	}}

	svc := New(fakeAgent, fakeSynth, &fakeCreds{})
	if _, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String()); err != nil {
		t.Fatalf("GetOrBuildPattern: %v", err)
	}

	if fakeSynth.calls != 1 {
		t.Fatalf("合成调用=%d, want 1", fakeSynth.calls)
	}
	got := fakeSynth.lastPills
	if len(got) != 1 {
		t.Fatalf("pills=%d, want 1", len(got))
	}
	ef := agent.AgentPillEffects[0]
	if got[0].ID != ef.Item.UUID.String() {
		t.Errorf("PillInput.ID=%q, want 实例 UUID %q", got[0].ID, ef.Item.UUID.String())
	}
	if got[0].Name != ef.NameSnapshot {
		t.Errorf("Name=%q, want 快照 %q", got[0].Name, ef.NameSnapshot)
	}
	if !reflect.DeepEqual(got[0].SkillSchema, ef.SchemaSnapshot) {
		t.Errorf("SkillSchema 必须来自 SchemaSnapshot 快照")
	}
	if got[0].Weight != ef.Weight || got[0].SortOrder != ef.SortOrder {
		t.Errorf("Weight/SortOrder=%v/%d, want %v/%d", got[0].Weight, got[0].SortOrder, ef.Weight, ef.SortOrder)
	}
	// 写缓存必须携带读取时的 EffectsRevision(CAS 核对依据)
	if len(fakeAgent.savedRevisions) != 1 || fakeAgent.savedRevisions[0] != agent.EffectsRevision {
		t.Fatalf("保存时 expectedEffectsRevision=%v, want %d", fakeAgent.savedRevisions, agent.EffectsRevision)
	}
}

// TestGetOrBuildPatternRevisionConflictRetries 写缓存时编排版本已变(并发服用已提交) →
// 丢弃本次编译结果重读重试;重试时新缓存已就位则直接命中,不重复合成、不用过期结果覆盖
func TestGetOrBuildPatternRevisionConflictRetries(t *testing.T) {
	agent := newMarkerAgent()
	fakeAgent := &fakeAgentDAO{agent: agent, conflictRemaining: 1}
	fakeAgent.onConflict = func() {
		// 模拟并发服用已提交: 版本递增 + 新能力的新缓存已就位
		agent.EffectsRevision = 4
		fp, _ := computeFingerprint(agent.Personality, buildPillInputs(agent))
		agent.LanguagePattern = &model.LanguagePattern{
			AgentID:           agent.UUID.String(),
			SystemPrompt:      "并发服用后的新缓存",
			BehaviorProfile:   model.JSONMap{"version": 1},
			ProfileVersion:    behavior.ProfileVersion,
			SourceFingerprint: fp,
			IsValid:           true,
		}
	}
	fakeSynth := &fakeSynthesis{resp: &synthesis.CombineResponse{
		EmergenceRules: model.JSONList{},
		Fingerprint:    "sha256:fp",
		Model:          "fake-synthesis-model",
	}}

	svc := New(fakeAgent, fakeSynth, &fakeCreds{})
	got, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String())
	if err != nil {
		t.Fatalf("GetOrBuildPattern: %v", err)
	}
	if fakeSynth.calls != 1 {
		t.Errorf("重试应命中并发方的新缓存,不重复合成;实际 %d 次", fakeSynth.calls)
	}
	if got != agent.LanguagePattern {
		t.Error("应返回并发方写入的新缓存")
	}
	if len(fakeAgent.saved) != 0 {
		t.Error("冲突后不得用本次过期结果覆盖新缓存")
	}
}

// TestGetOrBuildPatternRevisionConflictExhausted 写缓存持续冲突(连续编排变更):
// 最多重试 2 次(共 3 次尝试)后返回明确可重试错误,不静默返回过期结果
func TestGetOrBuildPatternRevisionConflictExhausted(t *testing.T) {
	agent := newMarkerAgent()
	fakeAgent := &fakeAgentDAO{agent: agent, conflictRemaining: 100}
	fakeSynth := &fakeSynthesis{resp: &synthesis.CombineResponse{
		EmergenceRules: model.JSONList{},
		Fingerprint:    "sha256:fp",
		Model:          "fake-synthesis-model",
	}}

	svc := New(fakeAgent, fakeSynth, &fakeCreds{})
	_, err := svc.GetOrBuildPattern(context.Background(), agent.UUID.String())
	if err == nil {
		t.Fatal("持续冲突应返回错误")
	}
	if err.GetCode() != "agent.effects_conflict" {
		t.Fatalf("code=%s, want agent.effects_conflict", err.GetCode())
	}
	if fakeSynth.calls != 3 {
		t.Errorf("合成尝试=%d, want 3(1 次主 + 2 次重试)", fakeSynth.calls)
	}
	if len(fakeAgent.saved) != 0 {
		t.Error("冲突结果不得落库")
	}
}
