// 群聊提示词与解析纯函数
// 纯函数无外部依赖,独立可测;编排器(group_orchestrator.go)组合使用
package chat_service

import (
	"fmt"
	"regexp"
	"strings"

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

// BuildGroupSystemPrompt 在道人自己的系统提示词后拼接群规则补丁
// mustAnswer 为 true(被@必答)时追加禁 PASS 行
func BuildGroupSystemPrompt(basePrompt string, selfName string, proactivity int, memberNames []string, mustAnswer bool) string {
	var b strings.Builder
	b.WriteString(basePrompt)
	b.WriteString("\n\n【群聊规则】\n")
	fmt.Fprintf(&b, "你正在群聊中与多人交谈。成员:%s、用户「%s」。\n", strings.Join(memberNames, "、"), UserLabel)
	b.WriteString("- 历史消息格式:【发言者】内容;你只代表「" + selfName + "」发言\n")
	fmt.Fprintf(&b, "- 你的表达欲:%d/100(越高越健谈)。结合性格和对话题的兴趣决定说不说\n", proactivity)
	b.WriteString("- 无话可说时只输出:[PASS]\n")
	b.WriteString("- 可 @成员名 邀请对方接话(被@的道人下一轮必回应),也可 @" + UserLabel + " 向用户提问\n")
	b.WriteString("- 不要复述他人整段话,不要代替其他成员发言\n")
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
