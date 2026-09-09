package behavior

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
)

// markerSkillSchema 构造一颗每个字段都带唯一标记的金丹(spec §14.1):
// 九个标记必须同时存活在档案与最终提示词中;future_key_2026 模拟未来新增键。
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

func markerPillInput() synthesis.PillInput {
	return synthesis.PillInput{
		ID:          "pill-marker-1",
		Name:        "标记金丹",
		Weight:      1.0,
		SortOrder:   0,
		SkillSchema: markerSkillSchema(),
	}
}

// TestCompileProfileKeepsAllMarkers spec §14.1:档案 JSON 必须保留全部九个标记
func TestCompileProfileKeepsAllMarkers(t *testing.T) {
	profile := CompileProfile("沉稳内敛", []synthesis.PillInput{markerPillInput()})

	raw, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("档案序列化失败: %v", err)
	}
	serialized := string(raw)
	for _, marker := range []string{
		"IDENTITY_MARKER", "DNA_MARKER", "MENTAL_MODEL_MARKER", "HEURISTIC_MARKER",
		"VALUE_MARKER", "ANTI_PATTERN_MARKER", "HONEST_LIMIT_MARKER",
		"EXAMPLE_MARKER", "UNKNOWN_FIELD_MARKER",
	} {
		if !strings.Contains(serialized, marker) {
			t.Errorf("档案缺少标记 %s;完整档案: %s", marker, serialized)
		}
	}

	if profile.Version != ProfileVersion {
		t.Errorf("Version = %d, want %d", profile.Version, ProfileVersion)
	}
	if profile.BasePersonality != "沉稳内敛" {
		t.Errorf("BasePersonality = %q", profile.BasePersonality)
	}
	p := profile.Pills[0]
	if p.PillID != "pill-marker-1" || p.Name != "标记金丹" || p.Weight != 1.0 || p.SortOrder != 0 {
		t.Errorf("金丹元数据编译错误: %+v", p)
	}
	if p.UnknownFields["future_key_2026"] != "UNKNOWN_FIELD_MARKER" {
		t.Errorf("未知键必须原值进入 UnknownFields: %+v", p.UnknownFields)
	}
	if len(p.UnknownFields) != 1 {
		t.Errorf("UnknownFields 应只有 future_key_2026,实际: %+v", p.UnknownFields)
	}
}

// TestCompileProfileTypeAnomalyGoesToUnknownFields spec §12:类型异常的原值进入
// UnknownFields 继续确定性渲染,不丢数据、不静默修复
func TestCompileProfileTypeAnomalyGoesToUnknownFields(t *testing.T) {
	pill := markerPillInput()
	pill.SkillSchema["mental_models"] = "not-a-list"                    // 类型异常
	pill.SkillSchema["identity_card"] = map[string]any{"name": "结构化身份"} // 类型异常

	profile := CompileProfile("", []synthesis.PillInput{pill})
	p := profile.Pills[0]

	if len(p.MentalModels) != 0 {
		t.Errorf("类型异常时 MentalModels 应为空,实际: %+v", p.MentalModels)
	}
	if p.UnknownFields["mental_models"] != "not-a-list" {
		t.Errorf("类型异常原值必须进 UnknownFields: %+v", p.UnknownFields)
	}
	if p.IdentityCard != "" {
		t.Errorf("类型异常时 IdentityCard 应为空,实际: %q", p.IdentityCard)
	}
	if p.UnknownFields["identity_card"] == nil {
		t.Error("identity_card 类型异常应进 UnknownFields")
	}
}

// TestCompileProfileRealSeedKeys 真实种子金丹键:agentic_protocol 是既有未知顶层键,
// 必须进 UnknownFields(它在渲染时由【扩展字段】分区兜住,不丢失)
func TestCompileProfileRealSeedKeys(t *testing.T) {
	pill := markerPillInput()
	pill.SkillSchema["agentic_protocol"] = map[string]any{"mode": "先辨问题之体"}

	profile := CompileProfile("", []synthesis.PillInput{pill})
	p := profile.Pills[0]
	if p.UnknownFields["agentic_protocol"] == nil {
		t.Error("agentic_protocol 应进 UnknownFields(它不属于已知九键)")
	}
}

// TestCompileProfileEmptyPills 空金丹列表:档案只有基础性格
func TestCompileProfileEmptyPills(t *testing.T) {
	profile := CompileProfile("测试性格", nil)
	if len(profile.Pills) != 0 {
		t.Errorf("Pills 应为空,实际: %+v", profile.Pills)
	}
	if profile.BasePersonality != "测试性格" {
		t.Errorf("BasePersonality = %q", profile.BasePersonality)
	}
}

// TestCompileProfileKeepsPillOrder 金丹按输入顺序编译(PillInput 已由调用方按
// (sort_order, uuid) 排序,编译本身保持输入序)
func TestCompileProfileKeepsPillOrder(t *testing.T) {
	a := markerPillInput()
	a.ID = "aaa"
	b := markerPillInput()
	b.ID = "bbb"
	profile := CompileProfile("", []synthesis.PillInput{a, b})
	if profile.Pills[0].PillID != "aaa" || profile.Pills[1].PillID != "bbb" {
		t.Errorf("金丹顺序未保持: %+v", profile.Pills)
	}
}

// TestWithEmergenceMergesAndClearsOnDegraded 涌现合并与降级清空
func TestWithEmergenceMergesAndClearsOnDegraded(t *testing.T) {
	profile := CompileProfile("", []synthesis.PillInput{markerPillInput()})

	profile.WithEmergence(
		model.JSONList{"涌现规则甲"},
		[]synthesis.InnerTension{{Dimension: "formality", Description: "正式程度相冲", Severity: "high"}},
		false, "",
	)
	if len(profile.EmergenceRules) != 1 || profile.EmergenceRules[0] != "涌现规则甲" {
		t.Errorf("EmergenceRules = %+v", profile.EmergenceRules)
	}
	if len(profile.InnerTensions) != 1 {
		t.Errorf("InnerTensions = %+v", profile.InnerTensions)
	}
	if profile.EmergenceDegraded {
		t.Error("非降级路径不应标记 EmergenceDegraded")
	}

	// 降级:清空涌现层并记录原因
	profile.WithEmergence(nil, nil, true, "llm_error")
	if len(profile.EmergenceRules) != 0 || len(profile.InnerTensions) != 0 {
		t.Errorf("降级后涌现层应清空: %+v / %+v", profile.EmergenceRules, profile.InnerTensions)
	}
	if !profile.EmergenceDegraded || profile.DegradedReason != "llm_error" {
		t.Errorf("降级标记错误: %+v / %q", profile.EmergenceDegraded, profile.DegradedReason)
	}
}

// TestProfileToJSONMapRoundTrip 档案持久化 JSON 往返
func TestProfileToJSONMapRoundTrip(t *testing.T) {
	profile := CompileProfile("测试", []synthesis.PillInput{markerPillInput()})
	profile.WithEmergence(model.JSONList{"规则"}, nil, false, "")

	m, err := ProfileToJSONMap(profile)
	if err != nil {
		t.Fatalf("ProfileToJSONMap 失败: %v", err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("JSONMap 序列化失败: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "IDENTITY_MARKER") || !strings.Contains(s, "UNKNOWN_FIELD_MARKER") {
		t.Errorf("JSONMap 往返丢失标记: %s", s)
	}
	if !strings.Contains(s, `"version":3`) {
		t.Errorf("JSONMap 缺少 version: %s", s)
	}
}
