package behavior

import (
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/model"
)

func eqMsgs(a, b []map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i]["role"] != b[i]["role"] || a[i]["content"] != b[i]["content"] {
			return false
		}
	}
	return true
}

// Task 13:few-shot 示例对话选择器——权重降序稳定遍历;每组非空 user/assistant;
// role 顺序严格 user→assistant;最多 maxPairs 组;超预算整组丢弃不截断;nil profile 返回 nil
func TestBuildFewShotMessages(t *testing.T) {
	profile := &DaoistBehaviorProfile{
		Version: ProfileVersion,
		Pills: []CompiledPillProfile{
			{PillID: "low", Name: "低权丹", Weight: 1.0, SortOrder: 0,
				ExampleDialogues: []model.JSONMap{
					{"user": "低权提问", "assistant": "低权回答。"},
				}},
			{PillID: "empty", Name: "空字段丹", Weight: 2.0, SortOrder: 1,
				ExampleDialogues: []model.JSONMap{
					{"user": "", "assistant": "无主句"},
				}},
			{PillID: "high", Name: "高权丹", Weight: 3.0, SortOrder: 2,
				ExampleDialogues: []model.JSONMap{
					{"user": "高权提问", "assistant": "高权回答。"},
					{"user": "第二组提问", "assistant": "第二组回答。"},
				}},
		},
	}

	msgs := BuildFewShotMessages(profile, 2, 400)
	// 权重降序:高权丹先;空字段整组跳过;最多 2 组;role 顺序 user→assistant
	want := []map[string]string{
		{"role": "user", "content": "高权提问"},
		{"role": "assistant", "content": "高权回答。"},
		{"role": "user", "content": "第二组提问"},
		{"role": "assistant", "content": "第二组回答。"},
	}
	if !eqMsgs(msgs, want) {
		t.Fatalf("msgs = %+v, want 高权丹两组优先、空字段跳过、user/assistant 严格交替", msgs)
	}
}

func TestBuildFewShotMessagesStableByIntakeOrder(t *testing.T) {
	// 同权重:服用顺序(SortOrder)稳定优先
	stable := &DaoistBehaviorProfile{
		Pills: []CompiledPillProfile{
			{PillID: "second", Weight: 2.0, SortOrder: 1,
				ExampleDialogues: []model.JSONMap{{"user": "后服用", "assistant": "后答。"}}},
			{PillID: "first", Weight: 2.0, SortOrder: 0,
				ExampleDialogues: []model.JSONMap{{"user": "先服用", "assistant": "先答。"}}},
		},
	}
	msgs := BuildFewShotMessages(stable, 2, 400)
	if len(msgs) != 4 || msgs[0]["content"] != "先服用" {
		t.Fatalf("同权重应按服用顺序稳定,先服用在前(2组4条): %+v", msgs)
	}
}

func TestBuildFewShotMessagesDropsOversizeGroupWhole(t *testing.T) {
	// 第二组加入后超 400 rune:整组丢弃不截断,保留第一组合格组
	big := &DaoistBehaviorProfile{
		Pills: []CompiledPillProfile{
			{PillID: "a", Weight: 3.0, SortOrder: 0,
				ExampleDialogues: []model.JSONMap{{"user": "短问", "assistant": "短答。"}}},
			{PillID: "b", Weight: 2.0, SortOrder: 1,
				ExampleDialogues: []model.JSONMap{{"user": strings.Repeat("长", 300), "assistant": strings.Repeat("长", 200)}}},
		},
	}
	msgs := BuildFewShotMessages(big, 2, 400)
	if len(msgs) != 2 || msgs[0]["content"] != "短问" {
		t.Fatalf("超预算应整组丢弃、保留第一组: %+v", msgs)
	}
}

func TestBuildFewShotMessagesNilProfile(t *testing.T) {
	if msgs := BuildFewShotMessages(nil, 2, 400); msgs != nil {
		t.Fatalf("nil profile 应返回 nil, got %+v", msgs)
	}
	if msgs := BuildFewShotMessages(sampleProfile(), 0, 400); msgs != nil {
		t.Fatalf("maxPairs<=0 应返回 nil, got %+v", msgs)
	}
}
