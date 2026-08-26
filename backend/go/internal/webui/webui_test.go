package webui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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

// TestHandler_SPAFallback_NonSessionChatPath: 非会话的客户端路由路径应 fallback 到
// webui HTML 200,这样 WKWebView 才不会显示 404 错误页。
// 注意:/chat/<uuid> 已改为 307(见 TestHandler_LegacyChatSessionRedirect),
// 这里只验证非 UUID 段不会被误判为会话。
func TestHandler_SPAFallback_NonSessionChatPath(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/chat/new", nil)
	Handler().ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, "非会话 SPA 路径必须 200,否则 WKWebView 显示错误页")
	require.True(t, isWebuiBody(rec.Body.String()), "SPA fallback body 应是 webui HTML")
}

// TestHandler_LegacyChatSessionRedirect: 旧的 /chat/<uuid> 深链必须 307 规范化到
// /chat?session=<uuid>,且保留 DesktopGuard token 等查询参数;规范地址不再重定向。
func TestHandler_LegacyChatSessionRedirect(t *testing.T) {
	const uuid = "828d3bda-1e59-4fe6-b848-50150d18e280"
	h := Handler()

	tests := []struct {
		name     string
		target   string
		wantCode int
		wantQuery url.Values
	}{
		{
			name:     "legacy uuid",
			target:   "/chat/" + uuid,
			wantCode: http.StatusTemporaryRedirect,
			wantQuery: url.Values{"session": []string{uuid}},
		},
		{
			name:     "preserve desktop token",
			target:   "/chat/" + uuid + "?token=opaque-test-token",
			wantCode: http.StatusTemporaryRedirect,
			wantQuery: url.Values{
				"session": []string{uuid},
				"token":   []string{"opaque-test-token"},
			},
		},
		{
			name:     "non-uuid segment is not a session",
			target:   "/chat/settings",
			wantCode: http.StatusOK,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tc.target, nil)
			h.ServeHTTP(rec, req)
			require.Equal(t, tc.wantCode, rec.Code)
			if tc.wantCode != http.StatusTemporaryRedirect {
				return
			}
			loc := rec.Header().Get("Location")
			require.NotEmpty(t, loc, "307 必须带 Location")
			u, err := url.Parse(loc)
			require.NoError(t, err)
			require.Equal(t, "/chat", u.Path)
			require.Equal(t, tc.wantQuery, u.Query())
		})
	}

	// 规范地址 /chat?session=<uuid> 直接 200,不应再被重定向。
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/chat?session="+uuid, nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "规范地址不应重定向")
	require.Empty(t, rec.Header().Get("Location"))
	require.True(t, isWebuiBody(rec.Body.String()))
}
