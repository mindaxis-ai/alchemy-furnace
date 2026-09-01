package turnpolicy

import "strings"

// TurnMode 是本轮对话的主导意图。导演层依据它决定回复形态、
// 长度预算和群聊发言人数，道人性格只决定「怎么说」。
type TurnMode string

const (
	// TurnModeCasual 闲聊寒暄：短、自然、不端官腔。
	TurnModeCasual TurnMode = "casual"
	// TurnModeVent 情绪倾诉：先接住情绪，少讲道理，不建议不列条。
	TurnModeVent TurnMode = "vent"
	// TurnModeFactual 事实问答：直接给答案，附一句依据即可。
	TurnModeFactual TurnMode = "factual"
	// TurnModeAdvice 寻求建议：给出明确倾向和建议，可带理由。
	TurnModeAdvice TurnMode = "advice"
	// TurnModeTask 明确任务：用户要的是执行或产出，不是讨论。
	TurnModeTask TurnMode = "task"
	// TurnModeDeepDive 深入分析：用户明确要求详细展开时才进入。
	TurnModeDeepDive TurnMode = "deep_dive"
)

// ResponseShape 是本轮回复的结构形态。
type ResponseShape string

const (
	// ResponseShapeChat 自然对话：无标题、无列表。
	ResponseShapeChat ResponseShape = "chat"
	// ResponseShapeAcknowledge 只接住，不展开（安慰、确认收到）。
	ResponseShapeAcknowledge ResponseShape = "acknowledge"
	// ResponseShapeAnswer 直接回答，简短带依据。
	ResponseShapeAnswer ResponseShape = "answer"
	// ResponseShapeSuggest 给建议和倾向。
	ResponseShapeSuggest ResponseShape = "suggest"
	// ResponseShapeSteps 分步方案，仅在用户明确要方案/步骤时使用。
	ResponseShapeSteps ResponseShape = "steps"
	// ResponseShapeAnalyze 结构化分析，仅在用户明确要求时使用。
	ResponseShapeAnalyze ResponseShape = "analyze"
)

// TurnIntent 是导演层对当前轮的完整判断，决定回复形态与权限边界。
type TurnIntent struct {
	Mode                   TurnMode
	Shape                  ResponseShape
	AllowList              bool
	AllowUnsolicitedAdvice bool
	AllowFollowUp          bool
}

// 意图信号标记（包内只读，规则优先，不使用正则和外部模型）。
// 求助信号：用户明确要建议或帮做决定。
var adviceMarkers = []string{"怎么办", "建议", "该不该", "如何选择", "帮我决定", "如何处理"}

// 任务信号：用户明确要执行或产出。
var taskMarkers = []string{"写", "生成", "修改", "排查", "修复", "实现", "整理", "列出"}

// 事实信号：用户要的是事实或简单判断。
var factualMarkers = []string{"什么是", "是谁", "多少", "何时", "为什么", "是否", "能不能"}

// containsAny 报告 s 是否包含任意一个标记。
func containsAny(s string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// ClassifyTurnIntent 按当前轮用户约束分类对话意图。
//
// 分类优先级固定为 Detailed → Vent → Advice → Task → Factual → Casual：
//  1. 用户明确要求详细展开 → deep_dive；
//  2. 明确求助信号（怎么办/建议/该不该/如何选择/帮我决定/如何处理）→ advice，
//     求助信号必须覆盖单纯烦躁（「烦死了，我该怎么办」→ advice）；
//  3. 任务信号（写/生成/修改/排查/修复/实现/整理/列出）→ task，
//     任务信号同样覆盖烦躁（「这个依赖很烦，帮我排查构建失败」→ task，
//     对客观对象的抱怨不是情绪倾诉）；
//  4. 事实信号（什么是/是谁/多少/何时/为什么/是否/能不能）→ factual；
//  5. 烦躁且无其他明确信号 → vent，只接住情绪；
//  6. 其余 → casual。
//
// AllowList/AllowUnsolicitedAdvice 按上表：deep_dive/advice/task 允许列条和
// 主动建议，vent/factual/casual 不允许；所有意图都允许自然追问。
func ClassifyTurnIntent(c UserTurnConstraints) TurnIntent {
	msg := strings.ToLower(strings.TrimSpace(c.LatestQuestion))

	if c.Detailed {
		return TurnIntent{Mode: TurnModeDeepDive, Shape: ResponseShapeAnalyze,
			AllowList: true, AllowUnsolicitedAdvice: true, AllowFollowUp: true}
	}
	if containsAny(msg, adviceMarkers) {
		return TurnIntent{Mode: TurnModeAdvice, Shape: ResponseShapeSuggest,
			AllowList: true, AllowUnsolicitedAdvice: true, AllowFollowUp: true}
	}
	if containsAny(msg, taskMarkers) {
		return TurnIntent{Mode: TurnModeTask, Shape: ResponseShapeSteps,
			AllowList: true, AllowUnsolicitedAdvice: true, AllowFollowUp: true}
	}
	if containsAny(msg, factualMarkers) {
		return TurnIntent{Mode: TurnModeFactual, Shape: ResponseShapeAnswer,
			AllowFollowUp: true}
	}
	if c.Frustration == FrustrationAnnoyed {
		return TurnIntent{Mode: TurnModeVent, Shape: ResponseShapeAcknowledge,
			AllowFollowUp: true}
	}
	return TurnIntent{Mode: TurnModeCasual, Shape: ResponseShapeChat,
		AllowFollowUp: true}
}
