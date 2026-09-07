package memory_service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/internal/service/credential"
)

func TestDistillParsesAndPersists(t *testing.T) {
	svc, _ := newTestService(t)
	svc.llmJSON = func(ctx context.Context, baseURL string, creds *credential.ModelCredentials, model string, messages []map[string]string) (string, error) {
		if len(messages) != 2 || messages[0]["role"] != "system" {
			t.Fatalf("消息结构: %+v", messages)
		}
		out, _ := json.Marshal([]map[string]any{
			{"kind": "user_fact", "content": "用户喜欢围棋", "keywords": []string{"围棋"}, "importance": 4, "confidence": 0.9},
			{"kind": "bogus", "content": "应被跳过"},
			{"kind": "episode", "content": string(make([]rune, 501))}, // 超长,应跳过
			{"kind": "open_loop", "content": "答应帮用户查棋谱"},
		})
		return string(out), nil
	}
	ok := svc.EnqueueDistillation(context.Background(), service.DistillationSpec{
		SessionUUID: "sess-1",
		Model:       "gpt-4o-mini",
		UserMessage: "我想学围棋",
		Targets: []service.DistillTarget{{
			AgentID: testAgentUID,
			Messages: []service.DistillMessage{
				{Role: "user", Content: "我想学围棋"},
				{Role: "assistant", Content: "好,先讲布局。"},
			},
		}},
	})
	if !ok {
		t.Fatal("入队应成功")
	}
	// 等待 worker 处理(轮询 ≤2s)
	deadline := 40
	for deadline > 0 {
		all, _ := svc.ListMemories(context.Background(), testAgentUID, "", true)
		if len(all) == 2 {
			break
		}
		deadline--
		time.Sleep(50 * time.Millisecond)
	}
	all, _ := svc.ListMemories(context.Background(), testAgentUID, "", true)
	if len(all) != 2 {
		t.Fatalf("应持久化 2 条合法候选(非法 2 条被跳过): %+v", all)
	}
	found := false
	for _, m := range all {
		if m.Content == "用户喜欢围棋" {
			found = true
			if m.Importance != 4 || len(m.Keywords) != 1 || m.Keywords[0] != "围棋" {
				t.Fatalf("字段透传: %+v", m)
			}
		}
	}
	if !found {
		t.Fatal("未找到 user_fact 记忆")
	}
	svc.Close()
}

func TestDistillQueueFullNonBlocking(t *testing.T) {
	svc, _ := newTestService(t)
	// 阻塞 worker:占住唯一处理通道
	block := make(chan struct{})
	svc.llmJSON = func(ctx context.Context, baseURL string, creds *credential.ModelCredentials, model string, messages []map[string]string) (string, error) {
		<-block
		return "[]", nil
	}
	spec := service.DistillationSpec{Model: "m", UserMessage: "x", Targets: []service.DistillTarget{{AgentID: testAgentUID, Messages: []service.DistillMessage{{Role: "user", Content: "x"}}}}}
	if !svc.EnqueueDistillation(context.Background(), spec) {
		t.Fatal("首个任务应入队")
	}
	// 填满队列(容量 32):worker 阻塞在 llmJSON 无法出队,连续入队直至非阻塞拒绝
	// worker 是否已取走首个任务存在调度竞争:入队成功数允许 32(首个仍在队列)或 33(首个已取出)
	queued := 1
	for queued < 40 {
		if !svc.EnqueueDistillation(context.Background(), spec) {
			break
		}
		queued++
	}
	if queued != 32 && queued != 33 {
		t.Fatalf("容量 32 应入队 32-33 个后拒绝,实际 %d", queued)
	}
	if svc.EnqueueDistillation(context.Background(), spec) {
		t.Fatal("队列满后入队应返回 false(非阻塞)")
	}
	close(block)
	svc.Close() // 有限关闭:处理完当前任务后退出,不 panic
}
