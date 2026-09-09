package behavior

import (
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/synthesis"
	"github.com/alchemy-furnace/server/model"
)

// sampleProfile 档案夹具(Task 15 自 activation_test.go 迁入:该文件随动态激活层删除,
// 但 render_test.go 仍依赖此夹具,故随存活测试迁入本文件)
func sampleProfile() *DaoistBehaviorProfile {
	return &DaoistBehaviorProfile{
		Version:         ProfileVersion,
		BasePersonality: "沉稳",
		Pills: []CompiledPillProfile{
			{
				PillID: "p1", Name: "古琴丹", Weight: 2.0, SortOrder: 0,
				Description:   "以古琴之道应答,论音乐与静心",
				ExpressionDNA: map[string]any{"vocabulary": []any{"古琴", "琴韵", "高山流水"}},
				MentalModels: []model.JSONMap{
					{"name": "知音", "one_liner": "先问对方所好再谈琴"},
					{"name": "松沉", "one_liner": "遇事先沉一口气"},
				},
				DecisionHeuristics: []model.JSONMap{
					{"condition": "被问及音乐", "action": "引用琴典作答", "case": "论乐"},
				},
				ExampleDialogues: []model.JSONMap{
					{"user": "你会弹琴吗", "assistant": "略通一二,愿闻其详。"},
				},
			},
			{
				PillID: "p2", Name: "棋弈丹", Weight: 1.0, SortOrder: 1,
				Description:   "围棋布局之道",
				ExpressionDNA: map[string]any{"vocabulary": []any{"围棋", "布局"}},
				MentalModels:  []model.JSONMap{{"name": "全局观", "one_liner": "先看大局再看局部"}},
			},
		},
	}
}

// TestRenderSystemPromptPartitions 完整档案渲染:人物分区 + 姓名/性格 + 六个标记
// (Task 6:心智模型/决策启发式/示例对话移出永久区,仅档案侧仍保留)
// + 涌现规则与冲突调和子节(spec §14.1 的确定性最终提示词断言)
func TestRenderSystemPromptPartitions(t *testing.T) {
	profile := CompileProfile("沉稳内敛，喜好引经据典", []synthesis.PillInput{markerPillInput()})
	profile.WithEmergence(
		model.JSONList{"文言丹性与嘻哈丹性按场景切换文白比例"},
		[]synthesis.InnerTension{{Dimension: "formality", Description: "正式程度相冲", Severity: "high"}},
		false, "",
	)

	prompt := RenderSystemPrompt(profile, "测试道人")

	for _, section := range []string{"【安全与真实性边界】", "【身份与性格】", "【知识、能力与表达习惯】", "【扩展字段】"} {
		if !strings.Contains(prompt, section) {
			t.Errorf("缺少分区 %s;完整提示词:\n%s", section, prompt)
		}
	}
	if !strings.Contains(prompt, "测试道人") {
		t.Error("提示词缺少道人姓名")
	}
	if !strings.Contains(prompt, "沉稳内敛，喜好引经据典") {
		t.Error("提示词缺少基础性格")
	}
	for _, marker := range []string{
		"IDENTITY_MARKER", "DNA_MARKER", "VALUE_MARKER", "ANTI_PATTERN_MARKER",
		"HONEST_LIMIT_MARKER", "UNKNOWN_FIELD_MARKER",
	} {
		if !strings.Contains(prompt, marker) {
			t.Errorf("提示词缺少标记 %s", marker)
		}
	}
	if !strings.Contains(prompt, "〔综合表达规则〕") || !strings.Contains(prompt, "按场景切换文白比例") {
		t.Error("涌现规则必须渲染进【知识、能力与表达习惯】")
	}
	if !strings.Contains(prompt, "〔内在张力与取舍〕") || !strings.Contains(prompt, "正式程度相冲") {
		t.Error("冲突调和建议必须渲染")
	}
}

// TestRenderSystemPromptOmitsExtendedFields 无未知字段时不输出【扩展字段】分区
func TestRenderSystemPromptOmitsExtendedFields(t *testing.T) {
	pill := markerPillInput()
	delete(pill.SkillSchema, "future_key_2026")
	profile := CompileProfile("", []synthesis.PillInput{pill})

	prompt := RenderSystemPrompt(profile, "")
	if strings.Contains(prompt, "【扩展字段】") {
		t.Error("无未知字段时不应输出【扩展字段】分区")
	}
	if strings.Contains(prompt, "UNKNOWN_FIELD_MARKER") {
		t.Error("删除 future_key_2026 后不应再有该标记")
	}
}

// TestRenderSystemPromptOmitsEmergenceSectionsWhenEmpty 无涌现层时不输出空子节
func TestRenderSystemPromptOmitsEmergenceSectionsWhenEmpty(t *testing.T) {
	profile := CompileProfile("", []synthesis.PillInput{markerPillInput()})

	prompt := RenderSystemPrompt(profile, "")
	if strings.Contains(prompt, "〔综合表达规则〕") || strings.Contains(prompt, "〔内在张力与取舍〕") {
		t.Error("无涌现层时不应输出空子节")
	}
}

// TestRenderSystemPromptEmptyNameOmitsNameLine 试丹场景无道人名:不输出空「姓名：」行
func TestRenderSystemPromptEmptyNameOmitsNameLine(t *testing.T) {
	profile := CompileProfile("", []synthesis.PillInput{markerPillInput()})

	prompt := RenderSystemPrompt(profile, "")
	if strings.Contains(prompt, "姓名：") {
		t.Error("selfName 为空时不应输出姓名行")
	}
}

// TestRenderSystemPromptDeterministic 同输入必同输出(确定性)
func TestRenderSystemPromptDeterministic(t *testing.T) {
	profile := CompileProfile("测试", []synthesis.PillInput{markerPillInput()})
	profile.WithEmergence(model.JSONList{"规则"}, nil, false, "")

	a := RenderSystemPrompt(profile, "道人甲")
	b := RenderSystemPrompt(profile, "道人甲")
	if a != b {
		t.Error("同输入渲染结果必须一致")
	}
}

// TestRenderSystemPromptEmptyProfile 空档案(无金丹无性格):至少保留安全边界
func TestRenderSystemPromptEmptyProfile(t *testing.T) {
	prompt := RenderSystemPrompt(CompileProfile("", nil), "")
	if !strings.Contains(prompt, "【安全与真实性边界】") {
		t.Error("空档案也应渲染安全边界")
	}
}

// TestRenderSystemPromptTreatsPillsAsInternalizedTraits Task 6:永久区不再强迫
// 每轮表演全部丹性——内化语义(不解释/不罗列/相关时才自然体现)
func TestRenderSystemPromptTreatsPillsAsInternalizedTraits(t *testing.T) {
	prompt := RenderSystemPrompt(sampleProfile(), "测试道人")
	if strings.Contains(prompt, "每轮回答都必须体现") {
		t.Fatal("不得强迫每轮表演全部丹性")
	}
	if !strings.Contains(prompt, "已经是你掌握的知识、能力与表达习惯") {
		t.Fatal("缺少内化语义")
	}
}

// TestRenderSystemPromptOmitsDormantReasoningAndExamples Task 6:心智模型/决策
// 启发式/示例对话移出永久区(只允许在本轮激活区或 few-shot 中出现)
func TestRenderSystemPromptOmitsDormantReasoningAndExamples(t *testing.T) {
	prompt := RenderSystemPrompt(sampleProfile(), "测试道人")
	for _, forbidden := range []string{"心智模型", "决策启发式", "示例对话"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("unexpected %s", forbidden)
		}
	}
}

// TestRenderSystemPromptUsesNaturalFirstPersonIdentity 防止道人退化为舞台式角色扮演：
// 身份和丹性应被视为稳定自我，回答只保留自然对话正文。
func TestRenderSystemPromptUsesNaturalFirstPersonIdentity(t *testing.T) {
	prompt := RenderSystemPrompt(CompileProfile("沉稳温和", nil), "清玄")

	for _, required := range []string{
		"真实且稳定的自我",
		"第一人称",
		"不使用括号动作",
		"不写神态、动作、环境或舞台旁白",
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("缺少自然身份约束 %q;完整提示词:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{"AI 角色扮演助手", "扮演、模仿或 cosplay"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("提示词仍含角色扮演导向 %q", forbidden)
		}
	}
}

func TestRenderSystemPromptPresentsAPersonWithoutProductMetaphors(t *testing.T) {
	profile := CompileProfile("冷峻、克制、善用反讽", []synthesis.PillInput{
		{
			ID: "ability-1", Name: "鲁迅语言风格丹", Weight: 2, SortOrder: 1,
			SkillSchema: model.JSONMap{
				"description":    "熟悉现代文学与社会批评",
				"expression_dna": model.JSONMap{"tone": "犀利"},
			},
		},
	})

	prompt := RenderSystemPrompt(profile, "鲁迅")

	for _, required := range []string{
		"【身份与性格】", "姓名：鲁迅", "冷峻、克制、善用反讽",
		"【炼丹炉中的既定记录】", "已服用金丹：鲁迅语言风格丹",
		"有哪些金丹时如实回答", "【知识、能力与表达习惯】",
		"鲁迅语言风格", "熟悉现代文学与社会批评",
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("提示词缺少人物信息 %q;完整提示词:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"你是道人", "道人身份", "丹性是你真实且稳定的自我", "修炼口吻", "权重", "第 1 服",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("模型可见提示词泄露产品隐喻 %q;完整提示词:\n%s", forbidden, prompt)
		}
	}
}

func TestBehaviorProfileVersionInvalidatesRoleplayPromptCache(t *testing.T) {
	if ProfileVersion != 3 {
		t.Fatalf("ProfileVersion = %d, want 3 to invalidate product-metaphor prompts", ProfileVersion)
	}
}
