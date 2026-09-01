package turnpolicy

import (
	"testing"
)

func TestBuildTurnPlanStopZeroCalls(t *testing.T) {
	c := ExtractUserTurnConstraints("够了，别说了")
	p := BuildTurnPlan(c, PolicyForProactivity(50), 3, nil)
	if !p.Stop {
		t.Fatal("明确停止应标记 Stop")
	}
	if p.MaxSpeakers != 0 || p.MaxRounds != 0 || p.MaxTokens != 0 || p.MaxTurnTokens != 0 {
		t.Fatalf("停止时预算应为 0: %+v", p)
	}
}

func TestBuildTurnPlanConciseOrAnnoyed(t *testing.T) {
	for _, msg := range []string{"烦，直接说结论", "说重点"} {
		c := ExtractUserTurnConstraints(msg)
		p := BuildTurnPlan(c, PolicyForProactivity(90), 3, nil)
		if p.MaxSpeakers != 1 || p.MaxRounds != 1 {
			t.Fatalf("%q → speakers=%d rounds=%d, want 1/1", msg, p.MaxSpeakers, p.MaxRounds)
		}
		if p.MaxSentences > 2 || p.MaxTokens > 256 {
			t.Fatalf("%q → 预算超限: %+v", msg, p)
		}
	}
}

func TestBuildTurnPlanOneEach(t *testing.T) {
	c := ExtractUserTurnConstraints("大家每人一句")
	p := BuildTurnPlan(c, PolicyForProactivity(50), 4, nil)
	if !p.OneEach || p.MaxSpeakers != 4 || p.MaxRounds != 1 {
		t.Fatalf("每人一句 → %+v, want speakers=4 rounds=1", p)
	}
	if p.MaxSentences > 1 || p.MaxTokens > 160 {
		t.Fatalf("每人一句预算超限: %+v", p)
	}
	if p.MaxTurnTokens > 4096 {
		t.Fatalf("MaxTurnTokens = %d, want ≤4096", p.MaxTurnTokens)
	}
}

func TestBuildTurnPlanNormalDiscussion(t *testing.T) {
	c := ExtractUserTurnConstraints("你怎么看这件事")
	p := BuildTurnPlan(c, PolicyForProactivity(50), 3, nil)
	if p.Intent.Mode != TurnModeCasual {
		t.Fatalf("普通讨论 → intent=%s, want casual", p.Intent.Mode)
	}
	// Task 4:预算来自意图档,不再是表达欲档
	if p.MaxSpeakers != 1 || p.MaxRounds != 1 {
		t.Fatalf("普通讨论 → %+v, want casual 档 1/1", p)
	}
	if p.MaxSentences != 2 || p.MaxTokens != 128 {
		t.Fatalf("casual 预算 → %+v, want 2句/128tok", p)
	}
	if p.MaxTurnTokens != 1280 {
		t.Fatalf("MaxTurnTokens = %d, want 1280", p.MaxTurnTokens)
	}
}

func TestBuildTurnPlanDetailed(t *testing.T) {
	// Task 4:Detailed 只进入 deep_dive 档(8句/896tok/2说话人/2轮),
	// 不再提升到 2048/3072 tokens
	c := ExtractUserTurnConstraints("详细讲讲")
	p := BuildTurnPlan(c, PolicyForProactivity(50), 1, nil)
	if p.Intent.Mode != TurnModeDeepDive {
		t.Fatalf("详细 → intent=%s, want deep_dive", p.Intent.Mode)
	}
	if p.MaxSentences != 8 || p.MaxTokens != 896 || p.MaxSpeakers != 2 || p.MaxRounds != 2 {
		t.Fatalf("单聊详细 → %+v, want 8句/896tok/2人/2轮", p)
	}
	if p.MaxTurnTokens != 1280 {
		t.Fatalf("详细 MaxTurnTokens = %d, want 1280(不再提升到 3072)", p.MaxTurnTokens)
	}
	// 群聊同档:成员数不再放大单人预算
	p = BuildTurnPlan(c, PolicyForProactivity(50), 4, nil)
	if p.MaxSentences != 8 || p.MaxTokens != 896 || p.MaxSpeakers != 2 || p.MaxRounds != 2 {
		t.Fatalf("群聊详细 → %+v, want 同 deep_dive 档", p)
	}
}

func TestBuildTurnPlanSingleChatMustAnswer(t *testing.T) {
	// 单聊始终 must_answer=true(§7.1);群聊由编排器对被@者设 true
	p := BuildTurnPlan(ExtractUserTurnConstraints("你好"), PolicyForProactivity(10), 1, nil)
	if !p.MustAnswer {
		t.Fatal("单聊应始终 MustAnswer")
	}
	// Task 4:表达欲不再决定单条回复长度——低表达欲档(quiet)不压缩 casual 意图预算
	if p.MaxSentences != 2 || p.MaxTokens != 128 {
		t.Fatalf("casual 单聊预算 → %+v, want 2句/128tok", p)
	}
}

func TestBuildTurnPlanPassesActivatedRules(t *testing.T) {
	rule := ActivatedPillRule{PillID: "p1", PillName: "古琴丹"}
	c := ExtractUserTurnConstraints("聊音乐")
	p := BuildTurnPlan(c, PolicyForProactivity(50), 2, []ActivatedPillRule{rule})
	if len(p.ActivatedRules) != 1 || p.ActivatedRules[0].PillID != "p1" {
		t.Fatalf("ActivatedRules 未透传: %+v", p.ActivatedRules)
	}
}

// Task 4：意图预算固定档位——六档 × 四个字段逐项断言，且 Intent 透传。
func TestBuildTurnPlanIntentBudgets(t *testing.T) {
	tests := []struct {
		name        string
		msg         string
		wantMode    TurnMode
		maxSentence int
		maxTokens   int
		maxSpeakers int
		maxRounds   int
	}{
		{"casual", "今天天气不错", TurnModeCasual, 2, 128, 1, 1},
		{"vent", "烦死了，今天又被领导骂了", TurnModeVent, 2, 128, 1, 1},
		{"factual", "什么是向量数据库", TurnModeFactual, 3, 256, 1, 1},
		// 注意:烦躁前缀会触发 Concise/烦躁覆盖(晚于意图默认值),故此处用干净求助消息
		{"advice", "我该怎么办", TurnModeAdvice, 4, 384, 1, 1},
		{"task", "帮我整理一个发布计划", TurnModeTask, 6, 640, 2, 1},
		{"deep_dive", "详细分析一下这套架构", TurnModeDeepDive, 8, 896, 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := BuildTurnPlan(ExtractUserTurnConstraints(tt.msg), PolicyForProactivity(50), 3, nil)
			if p.Intent.Mode != tt.wantMode {
				t.Fatalf("%q → intent mode=%s, want %s", tt.msg, p.Intent.Mode, tt.wantMode)
			}
			if p.MaxSentences != tt.maxSentence || p.MaxTokens != tt.maxTokens ||
				p.MaxSpeakers != tt.maxSpeakers || p.MaxRounds != tt.maxRounds {
				t.Fatalf("%q → %+v, want %d句/%dtok/%d说话人/%d轮",
					tt.msg, p, tt.maxSentence, tt.maxTokens, tt.maxSpeakers, tt.maxRounds)
			}
		})
	}
}
