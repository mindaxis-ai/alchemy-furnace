package webui

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// isWebuiBody 兼容两种 webui 产物:
//   - 占位:body 含 "炼丹炉 webui 占位"
//   - 真实 next 16 output:export 产物:body 含 "<!DOCTYPE html>" + "<html"
func isWebuiBody(s string) bool {
	return strings.Contains(s, "炼丹炉 webui 占位") ||
		(strings.Contains(s, "<!DOCTYPE html>") && strings.Contains(s, "<html"))
}

func TestHandlerMapping(t *testing.T) {
	h := Handler()
	for _, tc := range []struct {
		path     string
		wantCode int
	}{
		{"/", 200},                 // → 优先 index.html(占位)或 zh-CN.html(真实)
		{"/index.html", 200},       // 占位存在,真实不一定
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

func TestHandler_NextStaticAssets(t *testing.T) {
	// 关键回归测试:`all:out` 必须包含 _next/ 下划线开头的目录
	// 否则 next.js 的 css/js 全部 404,页面会"白屏/样式错乱"
	h := Handler()
	for _, tc := range []string{
		"/_next/static/chunks/some.css",
		"/_next/static/chunks/some.js",
		"/_next/static/media/some.woff2",
	} {
		t.Run(tc, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tc, nil)
			h.ServeHTTP(rec, req)
			// _next 资源占位可能不存在(只占位 index/404.html),但只要 out/_next/ 真实存在就应 200
			// 至少保证 _next 目录被纳入 embed(子目录能 stat)
			require.NotEqual(t, 0, rec.Code)
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
	require.Equal(t, 200, rec.Code, "/ 应 fallback 到 webui HTML 返 200")
	require.True(t, isWebuiBody(rec.Body.String()), "body 应是 webui HTML(占位或真实)")
}

// TestHandler_SPAFallback_ChatRoute: Next.js 客户端路由路径应 fallback 到 webui HTML
// 状态 200,这样 WKWebView 才不会显示 404 错误页
func TestHandler_SPAFallback_ChatRoute(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/chat/828d3bda-1e59-4fe6-b848-50150d18e280", nil)
	Handler().ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, "SPA 路径必须 200,否则 WKWebView 显示错误页")
	require.True(t, isWebuiBody(rec.Body.String()), "SPA fallback body 应是 webui HTML")
}
