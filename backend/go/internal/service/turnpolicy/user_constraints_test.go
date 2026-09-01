package turnpolicy

import (
	"testing"
)

// Task 3：用户当轮约束必须携带意图——纯情绪倾诉（烦躁且无求助/任务信号）
// 归为 vent。
func TestExtractUserTurnConstraintsIncludesIntent(t *testing.T) {
	c := ExtractUserTurnConstraints("我今天好烦，只想吐槽一下")
	if c.Intent.Mode != TurnModeVent {
		t.Fatalf("intent=%+v", c.Intent)
	}
}

func TestExtractUserTurnConstraintsNegationFirst(t *testing.T) {
	// 「不要停，继续说」不得误判停止(§8.1:先识别否定与继续表达,再识别停止)
	c := ExtractUserTurnConstraints("不要停，继续说，我想听完你的完整分析")
	if c.WantsStop {
		t.Fatal("「不要停,继续说」被误判为停止")
	}
	if !c.Detailed {
		t.Fatal("「继续」「完整分析」应标记详细")
	}
	// 「别停」同样不是停止
	if ExtractUserTurnConstraints("别停，继续讲").WantsStop {
		t.Fatal("「别停」被误判为停止")
	}
}

func TestExtractUserTurnConstraintsStop(t *testing.T) {
	for _, msg := range []string{
		"够了，别说了",
		"不用再说了",
		"别说了",
		"到此为止吧",
	} {
		if c := ExtractUserTurnConstraints(msg); !c.WantsStop {
			t.Fatalf("%q 应识别为停止: %+v", msg, c)
		}
	}
	// 单独「停」字
	if c := ExtractUserTurnConstraints("停"); !c.WantsStop {
		t.Fatalf("%q 应识别为停止", "停")
	}
}

func TestExtractUserTurnConstraintsConcise(t *testing.T) {
	for _, msg := range []string{
		"烦，直接说结论",
		"说重点",
		"简短一点",
		"别啰嗦",
	} {
		c := ExtractUserTurnConstraints(msg)
		if !c.Concise {
			t.Fatalf("%q 应识别为简短要求", msg)
		}
		if c.WantsStop {
			t.Fatalf("%q 不应识别为停止(烦≠停,§8.2)", msg)
		}
	}
}

func TestExtractUserTurnConstraintsFrustration(t *testing.T) {
	c := ExtractUserTurnConstraints("烦死了，每次都这样")
	if c.Frustration != FrustrationAnnoyed {
		t.Fatalf("烦躁级别 = %q, want annoyed", c.Frustration)
	}
	if ExtractUserTurnConstraints("今天心情不错").Frustration != FrustrationCalm {
		t.Fatal("无烦躁标记应为 calm")
	}
}

func TestExtractUserTurnConstraintsDetailed(t *testing.T) {
	c := ExtractUserTurnConstraints("详细讲讲这个方案，展开说")
	if !c.Detailed {
		t.Fatal("「详细」「展开」应标记详细")
	}
}

func TestExtractUserTurnConstraintsOneEach(t *testing.T) {
	for _, msg := range []string{
		"大家每人一句",
		"每人说一句就行",
		"每人都说一句",
	} {
		if c := ExtractUserTurnConstraints(msg); !c.OneEach {
			t.Fatalf("%q 应识别为每人一句", msg)
		}
	}
}

func TestExtractUserTurnConstraintsLatestQuestion(t *testing.T) {
	c := ExtractUserTurnConstraints("  先看看这个   ")
	if c.LatestQuestion != "先看看这个" {
		t.Fatalf("LatestQuestion = %q, want 去空白原文", c.LatestQuestion)
	}
	// 空输入安全
	if c := ExtractUserTurnConstraints(""); c.LatestQuestion != "" {
		t.Fatalf("空输入 LatestQuestion = %q", c.LatestQuestion)
	}
}

func TestExtractUserTurnConstraintsDoesNotTreatContextAsStop(t *testing.T) {
	for _, msg := range []string{
		"这个预算够了吗？",
		"他对我说‘别说了’，我该怎么办？",
		"你为什么不聊了？",
		"如果够了就告诉我，还不够请继续分析",
	} {
		if c := ExtractUserTurnConstraints(msg); c.WantsStop {
			t.Fatalf("普通疑问/转述不应停止: %q", msg)
		}
	}
}
