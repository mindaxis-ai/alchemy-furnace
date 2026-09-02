package behavior

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// BuildFewShotMessages 从金丹示例对话中挑选少量真实 role 消息注入 few-shot。
// 按金丹权重降序稳定遍历(同权重保持服用顺序);每组必须同时有非空 user/assistant;
// 输出 role 顺序严格为 user、assistant;最多 maxPairs 组、总计不超 maxRunes rune;
// 超预算的整组丢弃,不截断示例。nil profile 或 maxPairs<=0 返回 nil。
// 不得修改 profile、不得随机选择、不得把示例重新拼成 JSON。
func BuildFewShotMessages(profile *DaoistBehaviorProfile, maxPairs int, maxRunes int) []map[string]string {
	if profile == nil || maxPairs <= 0 {
		return nil
	}
	pills := make([]CompiledPillProfile, len(profile.Pills))
	copy(pills, profile.Pills) // 排序副本,不改原档案
	sort.SliceStable(pills, func(i, j int) bool {
		if pills[i].Weight != pills[j].Weight {
			return pills[i].Weight > pills[j].Weight
		}
		return pills[i].SortOrder < pills[j].SortOrder // 同权重:服用顺序优先
	})
	out := make([]map[string]string, 0, maxPairs*2)
	used := 0
	for _, pill := range pills {
		if len(out)/2 >= maxPairs {
			break
		}
		for _, d := range pill.ExampleDialogues {
			user, okU := d["user"].(string)
			assistant, okA := d["assistant"].(string)
			if !okU || !okA || strings.TrimSpace(user) == "" || strings.TrimSpace(assistant) == "" {
				continue // 非空校验失败:整组跳过
			}
			cost := utf8.RuneCountInString(user) + utf8.RuneCountInString(assistant)
			if maxRunes > 0 && used+cost > maxRunes {
				continue // 超预算:整组丢弃,不截断示例
			}
			out = append(out,
				map[string]string{"role": "user", "content": user},
				map[string]string{"role": "assistant", "content": assistant},
			)
			used += cost
			if len(out)/2 >= maxPairs {
				break
			}
		}
	}
	return out
}
