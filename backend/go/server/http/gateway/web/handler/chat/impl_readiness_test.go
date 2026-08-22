package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/server/http/router"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// readinessStub 只覆盖 GetReadiness 的 service.Chat 桩;其余方法经嵌入接口 nil-panic
type readinessStub struct {
	service.Chat
	readiness *service.ChatReadiness
	err       errors.Error
}

func (s *readinessStub) GetReadiness(context.Context) (*service.ChatReadiness, errors.Error) {
	return s.readiness, s.err
}

// setupReadinessRouter 同时注册静态 readiness 与动态 sessions/:uuid,验证两者共存不冲突
func setupReadinessRouter(stub service.Chat) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := New(stub)
	r.GET("/api/v1/chat/readiness", router.Wrapper(h.GetReadiness))
	r.GET("/api/v1/chat/sessions/:uuid", router.Wrapper(h.GetSession))
	return r
}

func performGetReadiness(t *testing.T, r *gin.Engine) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/readiness", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var envelope map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析响应包络失败: %v, body: %s", err, w.Body.String())
	}
	return w.Code, envelope
}

func readinessData(t *testing.T, envelope map[string]interface{}) map[string]interface{} {
	t.Helper()
	if envelope["code"].(float64) != 0 {
		t.Fatalf("期望 code=0, 实际 %v", envelope)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data 字段缺失或类型错误: %v", envelope)
	}
	return data
}

func TestReadinessZeroAgents(t *testing.T) {
	r := setupReadinessRouter(&readinessStub{readiness: &service.ChatReadiness{ActiveAgentCount: 0, ReadyAgentIDs: []uuid.UUID{}}})

	status, envelope := performGetReadiness(t, r)

	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}
	data := readinessData(t, envelope)
	if data["active_agent_count"].(float64) != 0 {
		t.Fatalf("active_agent_count = %v, want 0", data["active_agent_count"])
	}
	if ids, ok := data["ready_agent_ids"].([]interface{}); !ok || len(ids) != 0 {
		t.Fatalf("ready_agent_ids = %v, want empty array", data["ready_agent_ids"])
	}
	if data["can_create_single"].(bool) || data["can_create_group"].(bool) {
		t.Fatalf("zero agents must disable both creations: %v", data)
	}
}

func TestReadinessOneReadyAgent(t *testing.T) {
	uid := uuid.New()
	r := setupReadinessRouter(&readinessStub{readiness: &service.ChatReadiness{ActiveAgentCount: 3, ReadyAgentIDs: []uuid.UUID{uid}}})

	status, envelope := performGetReadiness(t, r)

	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}
	data := readinessData(t, envelope)
	if data["active_agent_count"].(float64) != 3 {
		t.Fatalf("active_agent_count = %v, want 3", data["active_agent_count"])
	}
	ids := data["ready_agent_ids"].([]interface{})
	if len(ids) != 1 || ids[0].(string) != uid.String() {
		t.Fatalf("ready_agent_ids = %v, want [%s]", ids, uid)
	}
	if !data["can_create_single"].(bool) {
		t.Fatal("one ready agent must allow single chat")
	}
	if data["can_create_group"].(bool) {
		t.Fatal("one ready agent must not allow group chat")
	}
}

func TestReadinessTwoReadyAgents(t *testing.T) {
	u1, u2 := uuid.New(), uuid.New()
	r := setupReadinessRouter(&readinessStub{readiness: &service.ChatReadiness{ActiveAgentCount: 2, ReadyAgentIDs: []uuid.UUID{u1, u2}}})

	status, envelope := performGetReadiness(t, r)

	if status != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %v", status, envelope)
	}
	data := readinessData(t, envelope)
	ids := data["ready_agent_ids"].([]interface{})
	if len(ids) != 2 || ids[0].(string) != u1.String() || ids[1].(string) != u2.String() {
		t.Fatalf("ready_agent_ids = %v, want [%s %s]", ids, u1, u2)
	}
	if !data["can_create_single"].(bool) || !data["can_create_group"].(bool) {
		t.Fatalf("two ready agents must allow both creations: %v", data)
	}
}

func TestReadinessServiceFailureReturnsSanitized500(t *testing.T) {
	r := setupReadinessRouter(&readinessStub{err: errors.ErrorServerInternalError("service.chat.readiness_list")})

	status, envelope := performGetReadiness(t, r)

	if status != http.StatusInternalServerError {
		t.Fatalf("期望 HTTP 500, 实际 %d, body: %v", status, envelope)
	}
	if envelope["error_code"].(string) != "service.chat.readiness_list" {
		t.Fatalf("error_code = %v, want service.chat.readiness_list", envelope["error_code"])
	}
	if envelope["message"].(string) != "服务器内部错误" {
		t.Fatalf("5xx message must stay generic, got %q", envelope["message"])
	}
	if strings.Contains(envelope["message"].(string), "must-not-leak") {
		t.Fatalf("response leaks internals: %v", envelope)
	}
}

func TestReadinessRouteCoexistsWithSessionRoutes(t *testing.T) {
	// 动态会话路由不得吞掉静态 readiness;能被路由到即证明注册共存
	r := setupReadinessRouter(&readinessStub{readiness: &service.ChatReadiness{ReadyAgentIDs: []uuid.UUID{}}})

	status, envelope := performGetReadiness(t, r)

	if status != http.StatusOK || envelope["data"] == nil {
		t.Fatalf("static readiness route unreachable: status=%d body=%v", status, envelope)
	}
}
