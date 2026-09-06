package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/router"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// createGroupStub 只覆盖 CreateGroupSession;ListMembers 等其余方法经嵌入接口 nil-panic,
// 若 handler 仍在提交后二次查询成员会立即暴露
type createGroupStub struct {
	service.Chat
	session  *model.ChatSession
	err      errors.Error
	gotUIDs  []uuid.UUID
	gotTitle string
}

func (s *createGroupStub) CreateGroupSession(_ context.Context, uids []uuid.UUID, title string) (*model.ChatSession, errors.Error) {
	s.gotUIDs = uids
	s.gotTitle = title
	return s.session, s.err
}

// createSingleStub 只覆盖 CreateSession(单聊创建)
type createSingleStub struct {
	service.Chat
	session *model.ChatSession
	err     errors.Error
	gotUID  uuid.UUID
}

func (s *createSingleStub) CreateSession(_ context.Context, agentUID uuid.UUID) (*model.ChatSession, errors.Error) {
	s.gotUID = agentUID
	return s.session, s.err
}

// 单聊创建: 201 响应必须携带道人真实身份(名称/头像/状态), 前端无需再按 agent_id 查名录
func TestCreateSingleSessionResponseCarriesDaoistIdentity(t *testing.T) {
	agentUID := uuid.New()
	agentID := agentUID.String()
	session := &model.ChatSession{ChatSessionID: uuid.New().String(), Type: model.SessionTypeSingle, AgentID: &agentID}
	session.Agent = model.DaoAgent{DaoAgentID: agentUID.String(), Name: "太上老君", Avatar: "https://example.com/laojun.png", Status: "active"}
	stub := &createSingleStub{session: session}

	// 单聊请求即使带 title 也忽略(不消费客户端标题)
	body := fmt.Sprintf(`{"type":"single","agent_id":%q,"title":"丹道夜话"}`, agentUID)
	status, envelope := performCreateSession(t, New(stub), body)

	if status != http.StatusCreated {
		t.Fatalf("期望 HTTP 201, 实际 %d, body: %v", status, envelope)
	}
	if stub.gotUID != agentUID {
		t.Fatalf("handler 应转发 agent uuid, got = %v", stub.gotUID)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data 字段缺失: %v", envelope)
	}
	if data["agent_name"] != "太上老君" || data["agent_avatar"] != "https://example.com/laojun.png" {
		t.Fatalf("创建响应未携带道人身份: %v", data)
	}
	if data["agent_status"] != "active" {
		t.Fatalf("创建响应 agent_status = %v, want active", data["agent_status"])
	}
}

func performCreateSession(t *testing.T, h *Chat, body string) (int, map[string]interface{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/chat/sessions", router.Wrapper(h.CreateSession))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var envelope map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析响应包络失败: %v, body: %s", err, w.Body.String())
	}
	return w.Code, envelope
}

func TestCreateGroupSessionResponseCarriesMembersWithoutSecondLookup(t *testing.T) {
	u1, u2 := uuid.New(), uuid.New()
	session := &model.ChatSession{ChatSessionID: uuid.New().String(), Type: model.SessionTypeGroup}
	session.Members = []model.SessionMember{
		{AgentID: u1.String(), SortOrder: 0, Agent: model.DaoAgent{DaoAgentID: u1.String(), Name: "太上老君", Status: "active"}},
		{AgentID: u2.String(), SortOrder: 1, Agent: model.DaoAgent{DaoAgentID: u2.String(), Name: "孙悟空", Status: "active"}},
	}
	stub := &createGroupStub{session: session}

	// 邀请含重复 uuid;service 去重后 [u1,u2],handler 原样转发入参
	body := fmt.Sprintf(`{"type":"group","member_agent_ids":[%q,%q,%q]}`, u1, u2, u1)
	status, envelope := performCreateSession(t, New(stub), body)

	if status != http.StatusCreated {
		t.Fatalf("期望 HTTP 201, 实际 %d, body: %v", status, envelope)
	}
	if len(stub.gotUIDs) != 3 || stub.gotUIDs[0] != u1 || stub.gotUIDs[1] != u2 || stub.gotUIDs[2] != u1 {
		t.Fatalf("handler 应原样转发邀请名单, gotUIDs = %v", stub.gotUIDs)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data 字段缺失: %v", envelope)
	}
	members, ok := data["members"].([]interface{})
	if !ok || len(members) != 2 {
		t.Fatalf("响应成员 = %v, want 2 deduped members", data["members"])
	}
	first := members[0].(map[string]interface{})
	second := members[1].(map[string]interface{})
	if first["agent_id"].(string) != u1.String() || second["agent_id"].(string) != u2.String() {
		t.Fatalf("响应成员 UUID 顺序 = [%v %v], want [%s %s]", first["agent_id"], second["agent_id"], u1, u2)
	}
	if first["name"].(string) != "太上老君" || second["name"].(string) != "孙悟空" {
		t.Fatalf("响应成员未携带道人信息: %v %v", first["name"], second["name"])
	}
}

func TestCreateGroupSessionRejectsMalformedMemberUUID(t *testing.T) {
	stub := &createGroupStub{}
	status, envelope := performCreateSession(t, New(stub), `{"type":"group","member_agent_ids":["not-a-uuid"]}`)

	if status != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400, 实际 %d, body: %v", status, envelope)
	}
	if envelope["error_code"].(string) != "handler.chat.create_member_uuid" {
		t.Fatalf("error_code = %v", envelope["error_code"])
	}
	if stub.gotUIDs != nil {
		t.Fatalf("参数错误不应触达 service, gotUIDs = %v", stub.gotUIDs)
	}
}

// handler 不 trim 标题,原样转发给 service 由 service 归一化(避免 handler/service 双重 trim 规则漂移)
func TestCreateGroupSessionForwardsTitleVerbatimToService(t *testing.T) {
	u1, u2 := uuid.New(), uuid.New()
	session := &model.ChatSession{ChatSessionID: uuid.New().String(), Type: model.SessionTypeGroup}
	session.Members = []model.SessionMember{
		{AgentID: u1.String(), SortOrder: 0, Agent: model.DaoAgent{DaoAgentID: u1.String(), Name: "太上老君", Status: "active"}},
		{AgentID: u2.String(), SortOrder: 1, Agent: model.DaoAgent{DaoAgentID: u2.String(), Name: "孙悟空", Status: "active"}},
	}
	stub := &createGroupStub{session: session}

	body := fmt.Sprintf(`{"type":"group","member_agent_ids":[%q,%q],"title":"  丹道夜话  "}`, u1, u2)
	status, envelope := performCreateSession(t, New(stub), body)

	if status != http.StatusCreated {
		t.Fatalf("期望 HTTP 201, 实际 %d, body: %v", status, envelope)
	}
	if stub.gotTitle != "  丹道夜话  " {
		t.Fatalf("handler 应原样转发标题(不 trim), gotTitle = %q", stub.gotTitle)
	}
}
