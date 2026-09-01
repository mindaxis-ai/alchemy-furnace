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
