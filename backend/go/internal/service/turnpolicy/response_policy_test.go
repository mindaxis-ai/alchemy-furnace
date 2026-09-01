package turnpolicy

import (
	"testing"
)

// Task 5:表达欲档位不再携带长度预算(MaxSentences/MaxTokens 已删除),
// 只保留主动发言概率与追问倾向(FollowUpPercent)
func TestPolicyForProactivityFixedMapping(t *testing.T) {
	tests := []struct {
		proactivity     int
		band            string
		followUpPercent int
	}{
		{0, "quiet", 0},
		{20, "quiet", 0},
		{21, "reserved", 10},
		{40, "reserved", 10},
		{41, "balanced", 20},
		{60, "balanced", 20},
		{61, "talkative", 35},
		{80, "talkative", 35},
		{81, "expansive", 50},
		{100, "expansive", 50},
	}
	for _, tt := range tests {
		p := PolicyForProactivity(tt.proactivity)
		if p.Band != tt.band || p.FollowUpPercent != tt.followUpPercent {
			t.Fatalf("proactivity=%d → %+v, want band=%s/followup=%d", tt.proactivity, p, tt.band, tt.followUpPercent)
		}
		// VolunteerPercent = 原始值(§7.1)
		if p.VolunteerPercent != tt.proactivity {
			t.Fatalf("proactivity=%d VolunteerPercent=%d, 应等于原始值", tt.proactivity, p.VolunteerPercent)
		}
	}
}

func TestPolicyForProactivityClamps(t *testing.T) {
	if p := PolicyForProactivity(-5); p.Band != "quiet" {
		t.Fatalf("negative → %+v", p)
	}
	if p := PolicyForProactivity(120); p.Band != "expansive" {
		t.Fatalf("over → %+v", p)
	}
}

func TestWantsToVolunteerStableBucket(t *testing.T) {
	policy := PolicyForProactivity(50)
	// 同一输入必同输出(确定性)
	first := WantsToVolunteer(policy, "sess-1", 1, "msg-1", 1)
	for i := 0; i < 10; i++ {
		if got := WantsToVolunteer(policy, "sess-1", 1, "msg-1", 1); got != first {
			t.Fatalf("不稳定桶: first=%v got=%v", first, got)
		}
	}
	// 不同输入应产生分布(会话/道人/消息/轮次任一变化都可能改变结果)
	sawVariance := false
	inputs := []struct {
		session string
		agent   uint
		msg     string
		round   int
	}{
		{"sess-2", 1, "msg-1", 1}, {"sess-1", 2, "msg-1", 1},
		{"sess-1", 1, "msg-2", 1}, {"sess-1", 1, "msg-1", 2},
	}
	for _, in := range inputs {
		if got := WantsToVolunteer(policy, in.session, in.agent, in.msg, in.round); got != first {
			sawVariance = true
		}
	}
	if !sawVariance {
		t.Fatal("桶对输入变化应产生分布(4 个不同输入至少一个不同)")
	}
	// 百分比边界:0 永不发言,100 永远发言
	if WantsToVolunteer(PolicyForProactivity(0), "s", 1, "m", 1) {
		t.Fatal("proactivity=0 不应自愿发言")
	}
	if !WantsToVolunteer(PolicyForProactivity(100), "s", 1, "m", 1) {
		t.Fatal("proactivity=100 应总是自愿发言")
	}
}

func TestWantsToVolunteerRoughDistribution(t *testing.T) {
	// 统计分布:1000 个固定回合(§14.3),proactivity 20/50/80 的自愿发言次数严格单调
	count := func(proactivity int) int {
		n := 0
		for i := 0; i < 1000; i++ {
			if WantsToVolunteer(PolicyForProactivity(proactivity), "sess-fixed", 7, "msg", i) {
				n++
			}
		}
		return n
	}
	c20, c50, c80 := count(20), count(50), count(80)
	if !(c20 < c50 && c50 < c80) {
		t.Fatalf("自愿发言次数应严格单调: 20→%d 50→%d 80→%d", c20, c50, c80)
	}
}
