package behavior

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/alchemy-furnace/server/model"
)

// DialogueExample 是发给人物模型的完整示例对白；不会截断单边内容。
type DialogueExample struct {
	User      string
	Assistant string
}

// SelectDialogueExamples 按档案中的能力与示例顺序稳定选择少量完整对白。
func SelectDialogueExamples(profile model.JSONMap, maxPairs int, maxRunes int) []DialogueExample {
	result := []DialogueExample{}
	if len(profile) == 0 || maxPairs <= 0 || maxRunes <= 0 {
		return result
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return result
	}
	var compiled DaoistBehaviorProfile
	if err := json.Unmarshal(raw, &compiled); err != nil {
		return result
	}
	used := 0
	for _, pill := range compiled.Pills {
		for _, row := range pill.ExampleDialogues {
			user, userOK := row["user"].(string)
			assistant, assistantOK := row["assistant"].(string)
			user = strings.TrimSpace(user)
			assistant = strings.TrimSpace(assistant)
			if !userOK || !assistantOK || user == "" || assistant == "" {
				continue
			}
			size := utf8.RuneCountInString(user) + utf8.RuneCountInString(assistant)
			if used+size > maxRunes {
				continue
			}
			result = append(result, DialogueExample{User: user, Assistant: assistant})
			used += size
			if len(result) == maxPairs {
				return result
			}
		}
	}
	return result
}
