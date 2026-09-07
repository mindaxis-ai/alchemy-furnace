package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alchemy-furnace/server/internal/errors"
	"github.com/alchemy-furnace/server/server/http/router"
	"github.com/gin-gonic/gin"
)

// TestConflictWithDataEmission 验证 ErrorConflictWithData 经 Wrapper 写出 HTTP 409 + data 字段
func TestConflictWithDataEmission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	count := int64(3)
	conflictErr := errors.ErrorConflictWithData(
		"service.model.delete_referenced",
		map[string]any{"referenced_by": count},
		"该模型仍被 %d 个道人引用，无法删除",
		count,
	)

	h := router.Wrapper(func(c *gin.Context) (int, any, error) {
		return 0, nil, conflictErr
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/models/abc", nil)
	c.Set("X-Request-ID", "req-1")
	h(c)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected HTTP 409, got %d (body=%s)", w.Code, w.Body.String())
	}

	var body struct {
		Code int              `json:"code"`
		Msg  string           `json:"message"`
		Data map[string]int64 `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, w.Body.String())
	}
	if body.Code != http.StatusConflict {
		t.Fatalf("expected body code 409, got %d", body.Code)
	}
	if body.Data["referenced_by"] != count {
		t.Fatalf("expected data.referenced_by=%d, got %v", count, body.Data["referenced_by"])
	}
	if body.Msg == "" {
		t.Fatalf("expected non-empty message")
	}
}
