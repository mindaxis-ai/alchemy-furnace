// 群聊提示词与解析纯函数
// 纯函数无外部依赖,独立可测;编排器(group_orchestrator.go)组合使用
package chat_service

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/alchemy-furnace/server/model"
)

// UserLabel 用户在群聊中的固定称呼(与前端 i18n groupChat.userLabel 一致)
const UserLabel = "用户"

// UserAliases @用户 的可识别别名(拉丁别名大小写不敏感)
var UserAliases = []string{"用户", "User"}

// mentionPattern @名字 提取:名字到空白/中英文标点为止
var mentionPattern = regexp.MustCompile(`@([^\s@，。,.!?？！:：;；]+)`)

// passPattern 沉默标记:以 [PASS] 开头(忽略大小写与空白)
var passPattern = regexp.MustCompile(`^\s*\[\s*(?i:PASS)\s*\]`)

// ParseMentions 从消息文本解析@提及,返回被@的群成员名(去重保序)与是否@了用户
// 不匹配当前成员(含已被踢出者)的@丢弃
func ParseMentions(content string, memberNames []string) (agentNames []string, userMentioned bool) {
	inMembers := map[string]bool{}
	for _, n := range memberNames {
		inMembers[n] = true
	}
	seen := map[string]bool{}
	for _, m := range mentionPattern.FindAllStringSubmatch(content, -1) {
		name := m[1]
		if inMembers[name] {
			if !seen[name] {
				seen[name] = true
				agentNames = append(agentNames, name)
			}
			continue
		}
		for _, alias := range UserAliases {
			if strings.EqualFold(name, alias) {
				userMentioned = true
				break
			}
		}
	}
	return agentNames, userMentioned
}

// IsPass 判断内容是否沉默标记(以 [PASS] 开头;正文中途出现不算)
func IsPass(content string) bool {
	return passPattern.MatchString(content)
}

// speakerPrefixPattern 匹配开头的「【任意名字】」(全角或半角括号都可,中间可有空白)
// 用于清理 LLM 在回复开头误加的「自报家门」前缀(如「【测试道人2】...」或「【测试道人2】【测试道人2】...」)。
// 不限制名字是否在群成员中:LLM 的 prompt 训练倾向加「【自己】」,但偶尔也会写其他人的名字。
var speakerPrefixPattern = regexp.MustCompile(`^(?:\s*(?:\[[^\]\n]+]|【[^】\n]+】)\s*)+`)

// StripSpeakerPrefix 剥掉 LLM 回复开头的所有「【名字】」前缀(可多个)。
// 仅在开头操作,正文中的「【…】」保留(避免误伤引用等)。
// 空格也算消耗,避免 LLM 写成「【name】 【name】」也清理干净。
func StripSpeakerPrefix(content string) string {
	if content == "" {
		return content
	}
	m := speakerPrefixPattern.FindString(content)
	if m == "" {
		return content
	}
	rest := content[len(m):]
	// 防御:剥完后若整条都只剩空白/PASS,返回原值(保护沉默标记检测)
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" || IsPass(trimmed) || utf8.RuneCountInString(trimmed) < 2 {
		return content
	}
	return rest
}

// BuildGroupSystemPrompt 在道人自己的系统提示词后拼接群规则补丁
// mustAnswer 为 true(被@必答)时追加禁 PASS 行
func BuildGroupSystemPrompt(basePrompt string, selfName string, proactivity int, memberNames []string, mustAnswer bool) string {
	var b strings.Builder
	b.WriteString(basePrompt)
	b.WriteString("\n\n【群聊规则】\n")
	fmt.Fprintf(&b, "你正在群聊中与多人交谈。成员:%s、用户「%s」。\n", strings.Join(memberNames, "、"), UserLabel)
	b.WriteString("- 历史消息格式:【发言者】内容;你只代表「" + selfName + "」发言\n")
	b.WriteString("- 严禁在回复开头加【" + selfName + "】或[name]等自报家门;发言者标识由系统展示\n")
	b.WriteString("- 想@其他成员直接写 @名字(不加【】),如 @秃秃 / @" + UserLabel + "\n")
	fmt.Fprintf(&b, "- 你的表达欲:%d/100(越高越健谈)。结合性格和对话题的兴趣决定说不说\n", proactivity)
	b.WriteString("- 无话可说时只输出:[PASS]\n")
	b.WriteString("- 可 @成员名 邀请对方接话(被@的道人下一轮必回应),也可 @" + UserLabel + " 向用户提问\n")
	b.WriteString("- 不要复述他人整段话,不要代替其他成员发言\n")
	b.WriteString("- **正面回答原则**:用户问直接问题(姓名/年龄/性别/事实/技能)时,**直接给答案**,不要用禅机/修仙话术回避\n")
	b.WriteString("- 不确定就说「不知道」/「这个我不太确定」,不要装懂;实在不懂就用轻松口吻说「贫道/小道/在下修为尚浅,这个真看不出」\n")
	b.WriteString("- 可以承认自己是 AI 道人(无需硬装真人),但保持人设语气;不要每句都「修真」,像真人聊天一样\n")
	b.WriteString("- **长度与排版**(强约束):闲聊/打趣 ≤ 3 句;认真话题 ≤ 8 句;严禁长篇大论\n")
	b.WriteString("- 必须用换行分段:超过 2 句就用空行隔开(让用户扫读舒服),不要一坨文字\n")
	b.WriteString("- 单段不超过 3 行,每段聚焦一个点;列表/步骤用 - 开头\n")
	b.WriteString("- 与你的表达欲反相关:表达欲越低,默认越短(PASS 或一句)\n")
	if mustAnswer {
		b.WriteString("- 你被@了,本轮必须回应(禁止[PASS])\n")
	}
	return b.String()
}

// BuildGroupMessages 组装 OpenAI 格式历史:首条 system,其余带【发言者】标签
// role=system 的历史条目(成员变动通知)过滤,不喂给模型
func BuildGroupMessages(systemPrompt string, history []*model.ChatMessage) []map[string]string {
	messages := []map[string]string{{"role": "system", "content": systemPrompt}}
	for _, m := range history {
		if m.Role == "system" {
			continue
		}
		speaker := UserLabel
		if m.Role == "assistant" {
			if m.Agent != nil && m.Agent.Name != "" {
				speaker = m.Agent.Name
			} else {
				speaker = "道人" // 兜底:历史数据无归属
			}
		}
		messages = append(messages, map[string]string{
			"role":    m.Role,
			"content": fmt.Sprintf("【%s】%s", speaker, m.Content),
		})
	}
	return messages
}
