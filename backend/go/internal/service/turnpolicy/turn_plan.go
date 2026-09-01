package turnpolicy

// MemorySnippet 注入提示词的单条本地记忆(spec §10.4;P3 填充)
type MemorySnippet struct {
	Kind    string
	Content string
}

// TurnPlan 当轮策略合并结果(spec §8.2)
type TurnPlan struct {
	Stop           bool
	MustAnswer     bool
	LatestQuestion string // 用户最新问题原文(供动态分区渲染,§8.1)
	Concise        bool
	Detailed       bool
	Frustration    FrustrationLevel
	Intent         TurnIntent // 导演层意图:决定预算档位(Task 4)
	MaxSentences   int
	MaxTokens      int
	MaxTurnTokens  int
	MaxSpeakers    int
	MaxRounds      int
	OneEach        bool
	ActivatedRules []ActivatedPillRule
	Memories       []MemorySnippet
}

// 预算常量(spec §8.2 行为表逐字)
const (
	budgetConciseTokens = 256
	budgetOneEachTokens = 160
	budgetTurnNormal    = 1280
	hardTokenCeiling    = 8192 // Python 契约上限(任何路径不得突破)
)

// intentBudget 意图固定预算档位(Task 4 表,禁止配置化)。
// 表达欲不再决定单条回复长度,只影响发言概率与追问倾向(Task 5 解耦)。
type intentBudget struct {
	maxSentences int
	maxTokens    int
	maxSpeakers  int
	maxRounds    int
}

var intentBudgets = map[TurnMode]intentBudget{
	TurnModeCasual:   {maxSentences: 2, maxTokens: 128, maxSpeakers: 1, maxRounds: 1},
	TurnModeVent:     {maxSentences: 2, maxTokens: 128, maxSpeakers: 1, maxRounds: 1},
	TurnModeFactual:  {maxSentences: 3, maxTokens: 256, maxSpeakers: 1, maxRounds: 1},
	TurnModeAdvice:   {maxSentences: 4, maxTokens: 384, maxSpeakers: 1, maxRounds: 1},
	TurnModeTask:     {maxSentences: 6, maxTokens: 640, maxSpeakers: 2, maxRounds: 1},
	TurnModeDeepDive: {maxSentences: 8, maxTokens: 896, maxSpeakers: 2, maxRounds: 2},
}

// BuildTurnPlan 合并用户约束、导演意图与显式用户覆盖。
// memberCount:群聊成员数;单聊传 1。套用顺序固定:
// 意图默认值 → Stop → Concise/烦躁 → OneEach(OneEach 必须晚于烦躁,
// 以便用户明确「每人一句」时按其要求执行)。Detailed 只进入 deep_dive 档,
// 不再提升到 2048/3072 tokens。
func BuildTurnPlan(constraints UserTurnConstraints, policy ResponsePolicy, memberCount int, activated []ActivatedPillRule) *TurnPlan {
	if memberCount < 1 {
		memberCount = 1
	}
	b := intentBudgets[constraints.Intent.Mode]
	plan := &TurnPlan{
		MustAnswer:     memberCount == 1, // 单聊始终必答(§7.1)
		LatestQuestion: constraints.LatestQuestion,
		Concise:        constraints.Concise,
		Detailed:       constraints.Detailed,
		Frustration:    constraints.Frustration,
		Intent:         constraints.Intent,
		MaxSentences:   b.maxSentences,
		MaxTokens:      b.maxTokens,
		MaxSpeakers:    b.maxSpeakers,
		MaxRounds:      b.maxRounds,
		MaxTurnTokens:  budgetTurnNormal,
		ActivatedRules: activated,
	}
	if constraints.WantsStop {
		plan.Stop = true
		plan.MaxSentences, plan.MaxTokens, plan.MaxTurnTokens = 0, 0, 0
		plan.MaxSpeakers, plan.MaxRounds = 0, 0
		return plan
	}
	switch {
	case constraints.Concise || constraints.Frustration == FrustrationAnnoyed:
		plan.MaxSpeakers, plan.MaxRounds = 1, 1
		plan.MaxSentences = minInt(plan.MaxSentences, 2)
		plan.MaxTokens = minInt(plan.MaxTokens, budgetConciseTokens)
		plan.MaxTurnTokens = plan.MaxTokens
	case constraints.OneEach:
		plan.MaxSpeakers, plan.MaxRounds = memberCount, 1
		plan.OneEach = true
		plan.MaxSentences, plan.MaxTokens = 1, budgetOneEachTokens
		plan.MaxTurnTokens = minInt(memberCount*budgetOneEachTokens, 4096)
	}
	return plan
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
