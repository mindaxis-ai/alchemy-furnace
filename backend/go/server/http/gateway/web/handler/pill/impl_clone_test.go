package pill

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/model"
	"github.com/alchemy-furnace/server/server/http/router"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// cloneStub 只覆盖 ClonePill 的 service.Pill 桩;其余方法经嵌入接口 nil-panic
type cloneStub struct {
	service.Pill
	clone *model.ElixirPill
	err   errors.Error
}

func (s *cloneStub) ClonePill(ctx context.Context, uid uuid.UUID) (*model.ElixirPill, errors.Error) {
	return s.clone, s.err
}

// setupCloneRouter 同时注册 clone 与动态 GET /:uuid,验证两者共存不冲突
func setupCloneRouter(stub service.Pill) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := New(stub)
	r.POST("/api/v1/pills/:uuid/clone", router.Wrapper(h.Clone))
	r.GET("/api/v1/pills/:uuid", router.Wrapper(h.Get))
	return r
}

func performClone(t *testing.T, r *gin.Engine, path string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var envelope map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析响应包络失败: %v, body: %s", err, w.Body.String())
	}
	return w.Code, envelope
}

func TestCloneCreated(t *testing.T) {
	cloneUID := uuid.New()
	r := setupCloneRouter(&cloneStub{clone: &model.ElixirPill{
		UUID:        cloneUID,
		Name:        "丹心妙语 副本",
		Description: "温润如茶的表达风格",
		SkillSchema: model.JSONMap{"expression_dna": map[string]interface{}{"tone": "温润"}},
		Tags:        model.JSONList{"古风"},
		Author:      "太上老君",
		Version:     "2.1.0",
		IsBuiltin:   false,
	}})

	status, envelope := performClone(t, r, "/api/v1/pills/"+uuid.NewString()+"/clone")

	if status != http.StatusCreated {
		t.Fatalf("期望 HTTP 201, 实际 %d, body: %v", status, envelope)
	}
	if envelope["code"].(float64) != 0 {
		t.Fatalf("期望 code=0, 实际 %v", envelope)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data 字段缺失或类型错误: %v", envelope)
	}
	if data["id"].(string) != cloneUID.String() {
		t.Fatalf("data.id = %v, want %s", data["id"], cloneUID)
	}
	if data["name"].(string) != "丹心妙语 副本" {
		t.Fatalf("data.name = %v", data["name"])
	}
	if data["is_builtin"].(bool) {
		t.Fatal("clone 响应 is_builtin 必须为 false")
	}
	if data["skill_schema"] == nil {
		t.Fatal("clone 响应必须携带完整 skill_schema")
	}
}

func TestCloneInvalidUUID(t *testing.T) {
	r := setupCloneRouter(&cloneStub{})

	status, envelope := performClone(t, r, "/api/v1/pills/not-a-uuid/clone")

	if status != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400, 实际 %d, body: %v", status, envelope)
	}
	if envelope["error_code"].(string) != "handler.pill.uuid_parse" {
		t.Fatalf("error_code = %v, want handler.pill.uuid_parse", envelope["error_code"])
	}
}

func TestCloneNotFoundPreservesStableCode(t *testing.T) {
	r := setupCloneRouter(&cloneStub{err: errors.ErrorRecordNotFound("service.pill.clone_take")})

	status, envelope := performClone(t, r, "/api/v1/pills/"+uuid.NewString()+"/clone")

	if status != http.StatusNotFound {
		t.Fatalf("期望 HTTP 404, 实际 %d, body: %v", status, envelope)
	}
	if envelope["error_code"].(string) != "service.pill.clone_take" {
		t.Fatalf("error_code = %v, want service.pill.clone_take", envelope["error_code"])
	}
}

func TestCloneInternalErrorSanitized(t *testing.T) {
	r := setupCloneRouter(&cloneStub{err: errors.ErrorServerInternalError("service.pill.clone")})

	status, envelope := performClone(t, r, "/api/v1/pills/"+uuid.NewString()+"/clone")

	if status != http.StatusInternalServerError {
		t.Fatalf("期望 HTTP 500, 实际 %d, body: %v", status, envelope)
	}
	if envelope["error_code"].(string) != "service.pill.clone" {
		t.Fatalf("error_code = %v, want service.pill.clone", envelope["error_code"])
	}
	if envelope["message"].(string) != "服务器内部错误" {
		t.Fatalf("5xx message must stay generic, got %q", envelope["message"])
	}
}

func TestCloneRouteCoexistsWithDynamicUUIDRoute(t *testing.T) {
	// 动态 GET /:uuid 不得吞掉 POST /:uuid/clone;能被路由到即证明注册共存
	r := setupCloneRouter(&cloneStub{clone: &model.ElixirPill{UUID: uuid.New(), Name: "x 副本"}})

	status, envelope := performClone(t, r, "/api/v1/pills/"+uuid.NewString()+"/clone")

	if status != http.StatusCreated || envelope["data"] == nil {
		t.Fatalf("clone route unreachable: status=%d body=%v", status, envelope)
	}
}
