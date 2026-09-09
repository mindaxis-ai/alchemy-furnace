package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/internal/interface/service"
	"github.com/alchemy-furnace/server/server/http/router"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type deleteSessionStub struct {
	service.Chat
	gotUID uuid.UUID
	calls  int
	err    errors.Error
}

func (s *deleteSessionStub) DeleteSession(_ context.Context, uid uuid.UUID) errors.Error {
	s.calls++
	s.gotUID = uid
	return s.err
}

func performDeleteSession(t *testing.T, h *Chat, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.DELETE("/api/v1/chat/sessions/:uuid", router.Wrapper(h.DeleteSession))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, path, nil))
	return w
}

func TestDeleteSessionForwardsUUIDAndReturnsOK(t *testing.T) {
	uid := uuid.New()
	stub := &deleteSessionStub{}

	w := performDeleteSession(t, New(stub), "/api/v1/chat/sessions/"+uid.String())

	if w.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200, 实际 %d, body: %s", w.Code, w.Body.String())
	}
	if stub.calls != 1 || stub.gotUID != uid {
		t.Fatalf("DeleteSession 调用 = %d, uuid = %v; want 1, %v", stub.calls, stub.gotUID, uid)
	}
}

func TestDeleteSessionRejectsMalformedUUIDBeforeService(t *testing.T) {
	stub := &deleteSessionStub{}

	w := performDeleteSession(t, New(stub), "/api/v1/chat/sessions/not-a-uuid")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("期望 HTTP 400, 实际 %d, body: %s", w.Code, w.Body.String())
	}
	if stub.calls != 0 {
		t.Fatalf("非法 UUID 不应触达 service, calls = %d", stub.calls)
	}
}

func TestDeleteSessionPropagatesServiceError(t *testing.T) {
	stub := &deleteSessionStub{err: errors.New(errors.ErrorTypeRecordNotFound, "chat.session_not_found", "会话不存在")}

	w := performDeleteSession(t, New(stub), "/api/v1/chat/sessions/"+uuid.NewString())

	if w.Code != http.StatusNotFound {
		t.Fatalf("期望 HTTP 404, 实际 %d, body: %s", w.Code, w.Body.String())
	}
	if stub.calls != 1 {
		t.Fatalf("service 调用 = %d, want 1", stub.calls)
	}
}
