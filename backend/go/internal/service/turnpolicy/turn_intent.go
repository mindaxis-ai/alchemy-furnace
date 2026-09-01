package turnpolicy

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

// ClassifyTurnIntent 按当前轮用户约束分类对话意图。
//
// Task 1 只实现最小默认行为：没有可识别信号时一律归为闲聊（casual/chat），
// 允许自然追问，不允许列条，不允许未经请求的建议。规则优先的完整
// 分类器由后续任务接入。
func ClassifyTurnIntent(c UserTurnConstraints) TurnIntent {
	return TurnIntent{
		Mode:          TurnModeCasual,
		Shape:         ResponseShapeChat,
		AllowFollowUp: true,
	}
}
