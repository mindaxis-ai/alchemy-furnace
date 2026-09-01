package behavior

import (
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/service/turnpolicy"
)

func TestComposeSystemPromptIncludesAllSections(t *testing.T) {
	profile := sampleProfile()
	rule := turnpolicy.ActivatedPillRule{
		PillID: "p1", PillName: "古琴丹",
		MentalModels: []string{"知音：先问对方所好再谈琴"},
	}
	plan := turnpolicy.BuildTurnPlan(
		turnpolicy.ExtractUserTurnConstraints("烦，直接说结论"),
		turnpolicy.PolicyForProactivity(50), 2, []turnpolicy.ActivatedPillRule{rule},
	)
	prompt := ComposeSystemPrompt(profile, "测试道人", plan)

	for _, title := range []string{
		"安全与真实性边界", "道人身份", "永久丹性核心",
		"本轮激活丹性", "本地记忆事实", "用户当轮要求", "本轮对话策略", "回答与群聊预算",
	} {
		if !strings.Contains(prompt, "【"+title+"】") {
			t.Fatalf("缺少分区 %q:\n%s", title, prompt)
		}
	}
}

func TestComposeSystemPromptActivatedRuleRendered(t *testing.T) {
	profile := sampleProfile()
	rule := turnpolicy.ActivatedPillRule{PillID: "p1", PillName: "古琴丹", MentalModels: []string{"知音：先问对方所好再谈琴"}}
	plan := turnpolicy.BuildTurnPlan(turnpolicy.ExtractUserTurnConstraints("聊音乐"), turnpolicy.PolicyForProactivity(50), 1, []turnpolicy.ActivatedPillRule{rule})
	prompt := ComposeSystemPrompt(profile, "测试道人", plan)
	if !strings.Contains(prompt, "古琴丹") || !strings.Contains(prompt, "知音") {
		t.Fatalf("激活丹性分区应含规则:\n%s", prompt)
	}
}

func TestComposeSystemPromptUserRequirementRendered(t *testing.T) {
	profile := sampleProfile()
	c := turnpolicy.ExtractUserTurnConstraints("烦，直接说结论")
	plan := turnpolicy.BuildTurnPlan(c, turnpolicy.PolicyForProactivity(80), 2, nil)
	prompt := ComposeSystemPrompt(profile, "测试道人", plan)
	// Task 7:只渲染可信策略描述,不复制用户原文(Global Constraints)
	if strings.Contains(prompt, "直接说结论") {
		t.Fatalf("用户当轮要求分区不得复制用户原文:\n%s", prompt)
	}
	if !strings.Contains(prompt, "用户要求简短直接") {
		t.Fatalf("用户当轮要求分区应含可信策略描述:\n%s", prompt)
	}
	// Task 4:简短+烦躁覆盖 → casual 档 128 tokens(意图预算,非表达欲档 256)
	if !strings.Contains(prompt, "内容预算约 128 tokens") {
		t.Fatalf("回答与群聊预算分区应含 token 预算:\n%s", prompt)
	}
}

func TestComposeSystemPromptStopPlanStillRendersBase(t *testing.T) {
	profile := sampleProfile()
	c := turnpolicy.ExtractUserTurnConstraints("够了，别说了")
	plan := turnpolicy.BuildTurnPlan(c, turnpolicy.PolicyForProactivity(50), 2, nil)
	prompt := ComposeSystemPrompt(profile, "测试道人", plan)
	if !strings.Contains(prompt, "道人身份") {
		t.Fatalf("停止计划仍应渲染完整提示词(是否调用由编排器决定):\n%s", prompt)
	}
}

func TestComposeSystemPromptNilProfileOrPlan(t *testing.T) {
	if got := ComposeSystemPrompt(nil, "x", nil); got != "" {
		t.Fatalf("nil profile/plan 应返回空串, got %q", got)
	}
}

// TestComposeSystemPromptStrategyPerIntent Task 7:【本轮对话策略】分区按意图
// 渲染固定策略文案(至少覆盖 vent/factual/task/deep_dive)
func TestComposeSystemPromptStrategyPerIntent(t *testing.T) {
	tests := []struct {
		message string
		want    string
	}{
		{"我今天好烦，只想吐槽一下", "先接住情绪"},
		{"什么是金丹", "第一句直接回答"},
		{"帮我排查构建失败", "只有确有多个步骤时才用列表"},
		{"详细分析一下这个方案", "允许结构化分析，但先给结论"},
	}
	for _, tt := range tests {
		c := turnpolicy.ExtractUserTurnConstraints(tt.message)
		plan := turnpolicy.BuildTurnPlan(c, turnpolicy.PolicyForProactivity(50), 1, nil)
		prompt := ComposeSystemPrompt(sampleProfile(), "测试道人", plan)
		if !strings.Contains(prompt, "【本轮对话策略】") {
			t.Fatalf("%q:缺少策略分区:\n%s", tt.message, prompt)
		}
		if !strings.Contains(prompt, tt.want) {
			t.Fatalf("%q:策略分区应含 %q:\n%s", tt.message, tt.want, prompt)
		}
		if !strings.Contains(prompt, "不默认使用") {
			t.Fatalf("%q:策略分区应含通用规则:\n%s", tt.message, prompt)
		}
	}
}

// TestComposeSystemPromptDoesNotCopyUserText Task 7:system prompt 不得复制
// 用户原文(Global Constraints:只含可信枚举和预算)
func TestComposeSystemPromptDoesNotCopyUserText(t *testing.T) {
	raw := "帮我排查构建失败，顺便看看日志"
	c := turnpolicy.ExtractUserTurnConstraints(raw)
	plan := turnpolicy.BuildTurnPlan(c, turnpolicy.PolicyForProactivity(50), 1, nil)
	prompt := ComposeSystemPrompt(sampleProfile(), "测试道人", plan)
	if strings.Contains(prompt, "用户最新消息:") {
		t.Fatal("system prompt 不得包含「用户最新消息:」行")
	}
	if strings.Contains(prompt, raw) {
		t.Fatal("system prompt 不得复制用户原文")
	}
}
