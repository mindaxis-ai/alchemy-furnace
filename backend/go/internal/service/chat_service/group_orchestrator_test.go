package chat_service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// scriptEngine 按调用次序回放预设 SSE 响应;"|" 分隔 chunk
// completionReply 为非流式 /completions 端点返回的 content(为空时该端点返回 500)
type scriptEngine struct {
	server          *httptest.Server
	replies         []string
	calls           int
	completionReply string
}

func newScriptEngine(replies []string) *scriptEngine {
	e := &scriptEngine{replies: replies}
	e.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/chat/completions/stream") {
			reply := ""
			if e.calls < len(e.replies) {
				reply = e.replies[e.calls]
			}
			e.calls++
			w.Header().Set("Content-Type", "text/event-stream")
			if reply != "" {
				for _, ch := range strings.Split(reply, "|") {
					fmt.Fprintf(w, "data: {\"content\": %q}\n\n", ch)
				}
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		// 非流式 /chat/completions(模拟 Python 端 BaseResponse 包络)
		if e.completionReply == "" {
			http.Error(w, `{"error":"no completion reply configured"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":0,"message":"ok","data":{"content": %q, "model": "test", "usage": {}}}`, e.completionReply)
	}))
	return e
}

type fakePattern struct{}

func (fakePattern) GetOrBuildPattern(ctx context.Context, agentID uint) (*model.LanguagePattern, errors.Error) {
	return &model.LanguagePattern{SystemPrompt: "你是道人。"}, nil
}

func newGroupSvc(t *testing.T, replies []string) (*Chat, *fakeChatDao, *scriptEngine, *model.ChatSession) {
	return newGroupSvcWithCompletion(t, replies, "")
}

func newGroupSvcWithCompletion(t *testing.T, replies []string, completionReply string) (*Chat, *fakeChatDao, *scriptEngine, *model.ChatSession) {
	t.Helper()
	engine := newScriptEngine(replies)
	engine.completionReply = completionReply
	t.Cleanup(engine.server.Close)
	u1, u2 := uuid.New(), uuid.New()
	agents := &fakeAgentDao{agents: map[string]*model.DaoAgent{
		u1.String(): {ID: 1, UUID: u1, Name: "太上老君", Status: "active", Proactivity: 50, ModelName: "test-model"},
		u2.String(): {ID: 2, UUID: u2, Name: "孙悟空", Status: "active", Proactivity: 90, ModelName: "test-model"},
	}}
	agentByID := map[uint]*model.DaoAgent{
		1: agents.agents[u1.String()],
		2: agents.agents[u2.String()],
	}
	chats := &fakeChatDao{
		sessions:  map[string]*model.ChatSession{},
		members:   map[uint][]*model.SessionMember{},
		agentByID: agentByID,
	}
	svc := New(chats, agents, fakePattern{}, availableCredentialResolver("test-model"), engine.server.URL)
	s, err := svc.CreateGroupSession(context.Background(), []uuid.UUID{u1, u2})
	if err != nil {
		t.Fatalf("建群: %v", err)
	}
	return svc, chats, engine, s
}

type eventLog struct{ events []string }

func (l *eventLog) emit(event string, payload any) { l.events = append(l.events, event) }

func countEvent(l *eventLog, name string) int {
	n := 0
	for _, e := range l.events {
		if e == name {
			n++
		}
	}
	return n
}

func TestGroupTurnAllPass(t *testing.T) {
	svc, chats, engine, s := newGroupSvc(t, []string{"[PASS]", "[PASS]"})
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "诸位怎么看?", log.emit)

	if countEvent(log, "speaker_start") != 0 {
		t.Fatal("全员沉默不应有 speaker_start")
	}
	if countEvent(log, "turn_done") != 1 {
		t.Fatal("必须有 turn_done")
	}
	// 整轮沉默提前 break:引擎仅 1 轮 × 2 人 = 2 次调用,零 assistant 落库
	if engine.calls != 2 {
		t.Fatalf("引擎调用次数=%d, 应=2", engine.calls)
	}
	for _, m := range chats.messages {
		if m.Role == "assistant" {
			t.Fatal("沉默不应落库 assistant 消息")
		}
	}
}

func TestGroupTurnMustAnswer(t *testing.T) {
	svc, chats, _, s := newGroupSvc(t, []string{"俺老孙来也", "[PASS]"})
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "@孙悟空 你怎么看?", log.emit)

	if countEvent(log, "speaker_start") != 1 {
		t.Fatalf("应只有孙悟空发言: %v", log.events)
	}
	var reply *model.ChatMessage
	for _, m := range chats.messages {
		if m.Role == "assistant" {
			reply = m
		}
	}
	if reply == nil || reply.AgentID == nil || *reply.AgentID != 2 {
		t.Fatalf("发言归属应为孙悟空(ID=2): %+v", reply)
	}
}

func TestGroupTurnChainMention(t *testing.T) {
	// round1: 老君发言并@悟空,悟空 PASS; round2: 悟空必答
	svc, _, engine, s := newGroupSvc(t, []string{
		"此事应问@孙悟空", "[PASS]",
		"俺来了", "[PASS]",
	})
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "聊聊金丹", log.emit)

	if countEvent(log, "speaker_start") != 2 {
		t.Fatalf("应有2人次发言: %v", log.events)
	}
	if engine.calls != 4 {
		t.Fatalf("应调引擎4次(2轮×2人), 实际%d", engine.calls)
	}
}

func TestGroupTurnMaxThreeRounds(t *testing.T) {
	replies := []string{"甲1", "乙1", "甲2", "乙2", "甲3", "乙3"}
	svc, _, engine, s := newGroupSvc(t, replies)
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "热烈讨论", log.emit)
	if engine.calls != 6 {
		t.Fatalf("3轮硬上限: 引擎应调6次, 实际%d", engine.calls)
	}
	if countEvent(log, "speaker_done") != 6 {
		t.Fatalf("应有6条发言: %v", log.events)
	}
}

func TestGroupTurnAutoTitle(t *testing.T) {
	svc, chats, _, s := newGroupSvcWithCompletion(t, []string{"金丹妙不可言", "[PASS]"}, "丹道夜话")
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "什么是金丹?", log.emit)

	if countEvent(log, "title") != 1 {
		t.Fatalf("应触发一次 title 事件: %v", log.events)
	}
	if chats.sessions[s.UUID.String()].Title != "丹道夜话" {
		t.Fatalf("标题未落库: %q", chats.sessions[s.UUID.String()].Title)
	}
}

func TestGroupTurnAutoTitleSkipsWhenRenamed(t *testing.T) {
	svc, chats, _, s := newGroupSvcWithCompletion(t, []string{"金丹妙不可言", "[PASS]"}, "丹道夜话")
	chats.sessions[s.UUID.String()].Title = "用户改的名"
	log := &eventLog{}
	svc.RunGroupTurn(context.Background(), s.UUID, "什么是金丹?", log.emit)
	if countEvent(log, "title") != 0 || chats.sessions[s.UUID.String()].Title != "用户改的名" {
		t.Fatal("已手动改名不应被覆盖")
	}
}
