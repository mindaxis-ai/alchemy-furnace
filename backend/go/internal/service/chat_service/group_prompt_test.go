package chat_service

import (
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/model"
)

func TestParseMentions(t *testing.T) {
	members := []string{"太上老君", "孙悟空"}
	agents, user := ParseMentions("@孙悟空 你怎么看?@太上老君 呢", members)
	if len(agents) != 2 || agents[0] != "孙悟空" || agents[1] != "太上老君" || user {
		t.Fatalf("基本@解析失败: %v %v", agents, user)
	}
	// @用户 / @User 都识别(拉丁大小写不敏感)
	if _, user = ParseMentions("@用户 你觉得呢", members); !user {
		t.Fatal("@用户 未识别")
	}
	if _, user = ParseMentions("@user what", members); !user {
		t.Fatal("@user 大小写未识别")
	}
	// 非成员(含被踢者)丢弃;重复@去重保序
	agents, _ = ParseMentions("@红孩儿 @孙悟空 @孙悟空", members)
	if len(agents) != 1 || agents[0] != "孙悟空" {
		t.Fatalf("非成员过滤/去重失败: %v", agents)
	}
	// 中文标点截断
	agents, _ = ParseMentions("@太上老君,来聊聊", members)
	if len(agents) != 1 {
		t.Fatalf("标点截断失败: %v", agents)
	}
}

func TestIsPass(t *testing.T) {
	for _, s := range []string{"[PASS]", " [pass] ", "[ PASS ]", "\n[PASS]"} {
		if !IsPass(s) {
			t.Fatalf("应判沉默: %q", s)
		}
	}
	for _, s := range []string{"", "我觉得[PASS]这个不对", "[PASSX]", "放行"} {
		if IsPass(s) {
			t.Fatalf("不应判沉默: %q", s)
		}
	}
}

func TestBuildGroupSystemPrompt(t *testing.T) {
	p := BuildGroupSystemPrompt("你是太上老君,清静无为。", "太上老君", 25, []string{"太上老君", "孙悟空"}, false)
	for _, want := range []string{
		"你是太上老君", "【群聊规则】", "太上老君、孙悟空", "表达欲:25/100", "[PASS]", "@用户",
		// 长度与排版(新增)
		"长度与排版", "闲聊/打趣 ≤ 3 句", "必须用换行分段", "单段不超过 3 行",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("补丁缺少 %q:\n%s", want, p)
		}
	}
	if strings.Contains(p, "禁止[PASS]") {
		t.Fatal("非必答者不应含必答行")
	}
	p2 := BuildGroupSystemPrompt("base", "孙悟空", 80, []string{"太上老君", "孙悟空"}, true)
	if !strings.Contains(p2, "禁止[PASS]") {
		t.Fatal("必答者必须含禁PASS行")
	}
}

func TestBuildGroupMessages(t *testing.T) {
	agentID := uint(7)
	history := []*model.ChatMessage{
		{Role: "user", Content: "诸位怎么看金丹?"},
		{Role: "assistant", Content: "贫道以为…", AgentID: &agentID, Agent: &model.DaoAgent{Name: "太上老君"}},
		{Role: "system", Content: "你邀请了 孙悟空 入群"},
	}
	msgs := BuildGroupMessages("系统提示", history)
	if len(msgs) != 3 { // system 提示 + 2 条(通知被过滤)
		t.Fatalf("消息条数不对: %d", len(msgs))
	}
	if msgs[0]["role"] != "system" || msgs[0]["content"] != "系统提示" {
		t.Fatalf("首条应为 system: %+v", msgs[0])
	}
	if msgs[1]["role"] != "user" || msgs[1]["content"] != "【用户】诸位怎么看金丹?" {
		t.Fatalf("用户消息标签不对: %+v", msgs[1])
	}
	if msgs[2]["role"] != "assistant" || msgs[2]["content"] != "【太上老君】贫道以为…" {
		t.Fatalf("道人消息标签不对: %+v", msgs[2])
	}
}

func TestStripSpeakerPrefix(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		// 单重 prefix
		{"【测试道人2】贫道已用过斋饭。", "贫道已用过斋饭。"},
		// 双重 prefix(LLM 偶尔会重复)
		{"【测试道人2】【测试道人2】善。", "善。"},
		// 半角 []
		{"[Test]hello world", "hello world"},
		// 半角 + 全角混合
		{"[Test]【Test】body", "body"},
		// prefix 后紧跟 @
		{"【秃秃】@测试道人2 道友。", "@测试道人2 道友。"},
		// 无 prefix
		{"普通回复,无自报家门。", "普通回复,无自报家门。"},
		// 整条只有 prefix(应保留原值,避免误伤)
		{"【测试道人2】", "【测试道人2】"},
		// prefix 后仅空白
		{"【测试道人2】  ", "【测试道人2】  "},
		// 正文中含【...】(不应被剥)
		{"回复中含【引用】的内容不应动。", "回复中含【引用】的内容不应动。"},
		// 空字符串
		{"", ""},
		// 前导空白 + prefix
		{"  【测试道人2】你好", "你好"},
	} {
		got := StripSpeakerPrefix(tc.in)
		if got != tc.want {
			t.Errorf("StripSpeakerPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
