package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func guardRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/x", DesktopGuard("tok123", "127.0.0.1:9999"), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	return r
}

func TestGuardPass(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/x", nil)
	req.Host = "127.0.0.1:9999"
	req.Header.Set("X-Alchemy-Token", "tok123")
	rec := httptest.NewRecorder()
	guardRouter().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("合法请求被拦: %d", rec.Code)
	}
}

func TestGuardReject(t *testing.T) {
	for _, tc := range []struct{ host, token string }{
		{"evil.com", "tok123"},       // Host 不符(DNS rebinding)
		{"127.0.0.1:9999", "wrong"},  // token 错
		{"127.0.0.1:9999", ""},       // 无 token
	} {
		req := httptest.NewRequest("GET", "/api/v1/x", nil)
		req.Host = tc.host
		if tc.token != "" {
			req.Header.Set("X-Alchemy-Token", tc.token)
		}
		rec := httptest.NewRecorder()
		guardRouter().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("host=%q token=%q 应 401,得 %d", tc.host, tc.token, rec.Code)
		}
	}
}
