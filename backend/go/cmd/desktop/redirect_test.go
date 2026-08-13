// redirect_test.go - newRedirectHandler 单测: webview 资源返回 200 + JS 跳转
// (不用 302: WKWebView 跨 scheme 302 wails:// → http:// 经常不跟随,白屏)
package desktop

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedirectHandler(t *testing.T) {
	h := newRedirectHandler(func() string { return "http://127.0.0.1:51234/?token=abc" })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 {
		t.Fatalf("期望 200(WKWebView 跨 scheme 友好),得 %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type 错: %s", ct)
	}
	if !strings.Contains(rec.Body.String(), "window.location.replace") {
		t.Fatalf("body 缺 JS 跳转: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "http://127.0.0.1:51234/?token=abc") {
		t.Fatalf("body 缺 target URL: %s", rec.Body.String())
	}
}

func TestRedirectHandler_RefreshOnTarget(t *testing.T) {
	// 闭包: 启动后期 port 已知,handler 每次请求重读最新 target
	current := "http://127.0.0.1:11111/?token=first"
	h := newRedirectHandler(func() string { return current })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), "http://127.0.0.1:11111/?token=first") {
		t.Fatalf("first target 错: %s", rec.Body.String())
	}
	current = "http://127.0.0.1:22222/?token=second"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec2.Body.String(), "http://127.0.0.1:22222/?token=second") {
		t.Fatalf("second target 错: %s", rec2.Body.String())
	}
}
