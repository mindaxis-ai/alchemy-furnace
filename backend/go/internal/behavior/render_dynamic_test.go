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
		"本轮激活丹性", "本地记忆事实", "用户当轮要求", "回答与群聊预算",
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
	if !strings.Contains(prompt, "直接说结论") {
		t.Fatalf("用户当轮要求分区应含原文意图:\n%s", prompt)
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
