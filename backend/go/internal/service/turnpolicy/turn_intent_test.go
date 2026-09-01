package turnpolicy

import "testing"

// Task 1：意图分类的默认行为——没有可识别信号时归为闲聊，
// 以 chat 形态回复，不主动给建议、不列条。
func TestClassifyTurnIntentDefaultIsCasual(t *testing.T) {
	c := UserTurnConstraints{LatestQuestion: "今天天气不错"}
	got := ClassifyTurnIntent(c)
	if got.Mode != TurnModeCasual || got.Shape != ResponseShapeChat || got.AllowList {
		t.Fatalf("got=%+v", got)
	}
}

// Task 2：规则优先的意图分类——优先级 Detailed → Vent → Advice →
// Task → Factual → Casual，vent 必须让明确求助信号覆盖单纯烦躁。
func TestClassifyTurnIntent(t *testing.T) {
	tests := []struct {
		name      string
		msg       string
		wantMode  TurnMode
		wantShape ResponseShape
	}{
		{"vent", "烦死了，今天又被领导骂了", TurnModeVent, ResponseShapeAcknowledge},
		{"vent asks advice", "烦死了，我该怎么办", TurnModeAdvice, ResponseShapeSuggest},
		{"fact", "什么是向量数据库", TurnModeFactual, ResponseShapeAnswer},
		{"task", "帮我整理一个发布计划", TurnModeTask, ResponseShapeSteps},
		{"deep", "详细分析一下这套架构", TurnModeDeepDive, ResponseShapeAnalyze},
		{"casual", "今天还挺开心", TurnModeCasual, ResponseShapeChat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyTurnIntent(ExtractUserTurnConstraints(tt.msg))
			if got.Mode != tt.wantMode || got.Shape != tt.wantShape {
				t.Fatalf("ClassifyTurnIntent(%q)=mode=%s shape=%s, want mode=%s shape=%s",
					tt.msg, got.Mode, got.Shape, tt.wantMode, tt.wantShape)
			}
		})
	}
}

// Task 2：误判保护——对客观对象的抱怨（烦躁 + 任务信号）不是情绪倾诉，
// 必须归为任务意图。
func TestClassifyTurnIntentDoesNotTreatObjectComplaintAsVent(t *testing.T) {
	c := ExtractUserTurnConstraints("这个依赖很烦，帮我排查构建失败")
	if got := ClassifyTurnIntent(c); got.Mode != TurnModeTask {
		t.Fatalf("got=%+v", got)
	}
}
