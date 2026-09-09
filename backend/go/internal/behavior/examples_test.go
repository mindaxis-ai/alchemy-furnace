package behavior

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alchemy-furnace/server/model"
)

func dialogueProfile(pills ...any) model.JSONMap {
	return model.JSONMap{
		"version":            3,
		"base_personality":   "",
		"pills":              pills,
		"emergence_rules":    []any{},
		"inner_tensions":     []any{},
		"emergence_degraded": false,
	}
}

func dialoguePill(name string, examples ...any) map[string]any {
	return map[string]any{
		"pill_id": name, "name": name, "weight": 1, "sort_order": 0,
		"example_dialogues": examples,
	}
}

func TestSelectDialogueExamplesHandlesEmptyAndMalformedProfiles(t *testing.T) {
	if got := SelectDialogueExamples(nil, 2, 400); len(got) != 0 {
		t.Fatalf("nil profile = %+v, want empty", got)
	}
	profile := dialogueProfile(dialoguePill("能力", map[string]any{"user": "缺回答"}))
	if got := SelectDialogueExamples(profile, 2, 400); len(got) != 0 {
		t.Fatalf("malformed examples = %+v, want empty", got)
	}
}

func TestSelectDialogueExamplesKeepsStableAbilityOrderAndWholePairs(t *testing.T) {
	profile := dialogueProfile(
		dialoguePill("能力甲",
			map[string]any{"user": "甲问", "assistant": "甲答"},
			map[string]any{"user": "乙问", "assistant": "乙答"},
		),
		dialoguePill("能力乙", map[string]any{"user": "丙问", "assistant": "丙答"}),
	)

	got := SelectDialogueExamples(profile, 2, 400)

	if len(got) != 2 || got[0].User != "甲问" || got[1].User != "乙问" {
		t.Fatalf("examples = %+v, want first two pairs in stored order", got)
	}
}

func TestSelectDialogueExamplesOmitsPairThatCrossesRuneBudget(t *testing.T) {
	long := strings.Repeat("长", 398)
	profile := dialogueProfile(dialoguePill("能力",
		map[string]any{"user": long, "assistant": "回答超过总预算"},
		map[string]any{"user": "短问", "assistant": "短答"},
	))

	got := SelectDialogueExamples(profile, 2, 400)

	if len(got) != 1 || got[0].User != "短问" {
		t.Fatalf("examples = %+v, want only the complete short pair", got)
	}
	if utf8.RuneCountInString(got[0].User+got[0].Assistant) > 400 {
		t.Fatal("selected examples exceed rune budget")
	}
}
