package memory

import (
	"context"
	"testing"

	"github.com/alchemy-furnace/server/model"
	"github.com/google/uuid"
)

// setupChatStore 构造一个隔离的内存 ChatDao(store 字段未导出,同包可绕过 SharedStore 单例)
func setupChatStore(t *testing.T) *ChatDao {
	t.Helper()
	s := NewStore()
	s.agents[uuid.New().String()] = &model.DaoAgent{ID: 1, Name: "太上老君"}
	s.agents[uuid.New().String()] = &model.DaoAgent{ID: 2, Name: "孙悟空"}
	return &ChatDao{store: s}
}

func TestMembersSaveFindDelete(t *testing.T) {
	d := setupChatStore(t)
	ctx := context.Background()

	if err := d.SaveMembers(ctx, []*model.SessionMember{
		{SessionID: 100, AgentID: 1, SortOrder: 0},
		{SessionID: 100, AgentID: 2, SortOrder: 1},
	}); err != nil {
		t.Fatalf("SaveMembers: %v", err)
	}

	members, err := d.FindMembers(ctx, 100)
	if err != nil || len(members) != 2 {
		t.Fatalf("FindMembers: len=%d err=%v", len(members), err)
	}
	if members[0].Agent.Name != "太上老君" || members[1].Agent.Name != "孙悟空" {
		t.Fatalf("FindMembers 预加载 Agent 失败: %+v", members)
	}
	if members[0].SortOrder != 0 || members[1].SortOrder != 1 {
		t.Fatalf("SortOrder 排序不对: %+v", members)
	}

	if err := d.DeleteMember(ctx, 100, 1); err != nil {
		t.Fatalf("DeleteMember: %v", err)
	}
	members, _ = d.FindMembers(ctx, 100)
	if len(members) != 1 || members[0].AgentID != 2 {
		t.Fatalf("删除后成员不对: %+v", members)
	}

	// 删除不存在应返回错误
	if err := d.DeleteMember(ctx, 100, 999); err == nil {
		t.Fatal("删除不存在成员应返回错误")
	}
}

func TestFindMessagesAttachesAgent(t *testing.T) {
	d := setupChatStore(t)
	ctx := context.Background()
	agentID := uint(1)
	if err := d.SaveMessage(ctx, &model.ChatMessage{SessionID: 100, Role: "assistant", Content: "贫道有礼", AgentID: &agentID}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	// 用户消息(无 AgentID) 也得能正常返回
	if err := d.SaveMessage(ctx, &model.ChatMessage{SessionID: 100, Role: "user", Content: "请赐教"}); err != nil {
		t.Fatalf("SaveMessage user: %v", err)
	}
	_, msgs, err := d.FindMessages(ctx, 100, 1, 20)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("FindMessages: len=%d err=%v", len(msgs), err)
	}
	if msgs[0].Agent == nil || msgs[0].Agent.Name != "太上老君" {
		t.Fatalf("道人消息未带归属: %+v", msgs[0])
	}
	if msgs[1].Agent != nil {
		t.Fatalf("用户消息不应带 Agent: %+v", msgs[1])
	}
}
