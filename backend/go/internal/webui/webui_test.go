package webui

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerMapping(t *testing.T) {
	h := Handler()
	for _, tc := range []struct {
		path     string
		wantCode int
	}{
		{"/", 200},                 // → 默认 locale(zh-CN.html)
		{"/index.html", 200},       // fallback 到 zh-CN.html
		{"/chat/abc-uuid", 200},    // SPA fallback(client-side router 接管)
		{"/settings/profile", 200}, // SPA fallback
		{"/no-such.css", 404},      // 有扩展名 + 找不到 → 真正 404
		{"/missing.png", 404},      // 同上
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tc.path, nil)
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("%s → %d, want %d", tc.path, rec.Code, tc.wantCode)
			}
		})
	}
}

func TestHandlerCacheHeader(t *testing.T) {
	h := Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/_next/static/chunk-abc.js", nil)
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Cache-Control") == "" {
		t.Fatal("_next 路径缺缓存头")
	}
}

// TestHandler_NoIndexHtmlFallback: next 16 output:export 不生成 index.html 时,/ 路径
// 自动 fallback 到 zh-CN.html(默认 locale)
func TestHandler_NoIndexHtmlFallback(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	Handler().ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, "/ 应 fallback 到 zh-CN.html 返 200")
	body := rec.Body.String()
	require.Contains(t, body, "<!DOCTYPE html>")
}

// TestHandler_SPAFallback_ChatRoute: Next.js 客户端路由路径应 fallback 到 zh-CN.html
// 状态 200,这样 WKWebView 才不会显示 404 错误页
func TestHandler_SPAFallback_ChatRoute(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/chat/828d3bda-1e59-4fe6-b848-50150d18e280", nil)
	Handler().ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, "SPA 路径必须 200,否则 WKWebView 显示错误页")
	require.Contains(t, rec.Body.String(), "<title>炼丹炉", "SPA fallback body 应是 webui HTML")
}
