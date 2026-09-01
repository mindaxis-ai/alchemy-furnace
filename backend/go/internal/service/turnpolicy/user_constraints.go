package turnpolicy

import (
	"strings"
)

// FrustrationLevel 用户烦躁级别(§8.1)
type FrustrationLevel string

const (
	FrustrationCalm    FrustrationLevel = "calm"
	FrustrationAnnoyed FrustrationLevel = "annoyed"
)

// UserTurnConstraints 用户当轮要求(§8.1;只保留可信布尔值与最新问题原文)
type UserTurnConstraints struct {
	LatestQuestion string
	Concise        bool
	Detailed       bool
	WantsStop      bool
	Frustration    FrustrationLevel
	OneEach        bool
	// Intent 是本轮对话意图，由导演层在约束提取完成后统一分类，
	// 调用方无需关心分类细节。
	Intent TurnIntent
}

// explicitStopCommands 只接受整句明确停止命令。任意位置的停止词
// 会误伤「他对我说别说了」和「预算够了吗」等正常问题。
var explicitStopCommands = map[string]bool{
	"停": true, "够了": true, "别说了": true, "不用说了": true,
	"不用再说了": true, "到此为止": true, "到此为止吧": true,
	"别再说了": true, "不用回了": true, "不用回复了": true,
	"不聊了": true, "好了别说了": true, "够了别说了": true,
	"够了，别说了": true,
}

// 简短标记
var conciseMarkers = []string{"直接说结论", "说重点", "简短", "别啰嗦", "少说", "简洁", "一句话说清", "简而言之"}

// 详细标记
var detailedMarkers = []string{"详细", "展开", "多说", "说清楚", "完整分析", "具体讲讲", "深入"}

// 烦躁标记
var annoyedMarkers = []string{"烦", "够了", "别废话", "不耐烦", "气死", "无语"}

// 否定前缀(「不/别/莫/无须」+ 立即跟随的停止/烦躁词 → 否定其意义)
var negationPrefixes = []string{"不要", "别", "不", "莫", "无须", "无需"}

// ExtractUserTurnConstraints 无模型纯函数提取用户当轮要求(§8.1)。
// 顺序:1) 否定与继续表达优先(「不要停,继续说」→ 不停止);2) 再识别停止;3) 简短/详细/烦躁/每人一句。
func ExtractUserTurnConstraints(userMessage string) UserTurnConstraints {
	c := UserTurnConstraints{
		LatestQuestion: strings.TrimSpace(userMessage),
		Frustration:    FrustrationCalm,
	}
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		c.Intent = ClassifyTurnIntent(c)
		return c
	}
	lower := strings.ToLower(msg)

	// 每人一句
	for _, m := range []string{"每人一句", "每人都说一句", "每人说一句", "一人一句", "一个一个来"} {
		if strings.Contains(lower, m) {
			c.OneEach = true
			break
		}
	}

	// 继续表达(「继续说/不要停/别停/继续」)优先否定停止
	continueSignal := strings.Contains(lower, "继续说") ||
		strings.Contains(lower, "接着说") ||
		strings.Contains(lower, "不要停") ||
		strings.Contains(lower, "别停") ||
		strings.Contains(lower, "别断")

	// 停止:只识别整句明确命令，不能用子串匹配。
	if !continueSignal {
		command := strings.Trim(strings.TrimSpace(lower), "，。,.!?！？；;：:")
		c.WantsStop = explicitStopCommands[command]
	}

	// 简短
	for _, marker := range conciseMarkers {
		if strings.Contains(lower, marker) {
			c.Concise = true
			break
		}
	}
	// 详细
	for _, marker := range detailedMarkers {
		if strings.Contains(lower, marker) {
			c.Detailed = true
			break
		}
	}
	// 烦躁(排除「不烦」)
	for _, marker := range annoyedMarkers {
		if strings.Contains(lower, marker) {
			negated := false
			for _, neg := range negationPrefixes {
				if strings.Contains(lower, neg+marker) {
					negated = true
					break
				}
			}
			if !negated {
				c.Frustration = FrustrationAnnoyed
				break
			}
		}
	}
	// 意图：所有布尔提取完成后统一分类一次，不在各分支重复调用。
	c.Intent = ClassifyTurnIntent(c)
	return c
}
