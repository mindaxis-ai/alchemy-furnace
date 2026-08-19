package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	internalerrors "github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/server/http/middleware"
	"github.com/gin-gonic/gin"
)

func TestWrapperPreservesErrorCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/chat", Wrapper(func(c *gin.Context) (int, any, error) {
		return 0, nil, internalerrors.New(
			internalerrors.ErrorTypeServerInternalError,
			"service.chat.agent_inactive",
			"agent service connection password=secret",
		)
	}))

	request := httptest.NewRequest(http.MethodGet, "/chat", nil)
	request.Header.Set("X-Request-ID", "request-123")
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("HTTP status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := body["code"]; got != float64(http.StatusInternalServerError) {
		t.Errorf("code = %v, want %d", got, http.StatusInternalServerError)
	}
	if got := body["request_id"]; got != "request-123" {
		t.Errorf("request_id = %v, want request-123", got)
	}
	if got := body["error_code"]; got != "service.chat.agent_inactive" {
		t.Errorf("error_code = %v, want service.chat.agent_inactive", got)
	}
	if got := body["message"]; got != "服务器内部错误" {
		t.Errorf("message = %v, want 服务器内部错误", got)
	}
}

func TestWrapperOmitsErrorCodeForOrdinaryError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/chat", Wrapper(func(c *gin.Context) (int, any, error) {
		return 0, nil, assertiveError("ordinary failure")
	}))

	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/chat", nil))

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := body["error_code"]; ok {
		t.Errorf("ordinary errors must omit error_code, got %v", body["error_code"])
	}
}

func TestWrapperPagePreservesErrorCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/chat", WrapperPage(func(c *gin.Context) (int64, int, int, any, error) {
		return 0, 0, 0, nil, internalerrors.New(
			internalerrors.ErrorTypeServerInternalError,
			"service.chat.agent_inactive",
			"agent service connection password=secret",
		)
	}))

	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/chat", nil))

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := body["error_code"]; got != "service.chat.agent_inactive" {
		t.Errorf("error_code = %v, want service.chat.agent_inactive", got)
	}
	if got := body["message"]; got != "服务器内部错误" {
		t.Errorf("message = %v, want 服务器内部错误", got)
	}
}

type assertiveError string

func (e assertiveError) Error() string { return string(e) }
