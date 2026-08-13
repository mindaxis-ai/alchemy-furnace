// redirect_test.go - newRedirectHandler 单测: webview 内任意路径必须 302 到 http origin
package main

import (
	"net/http/httptest"
	"testing"
)

func TestRedirectHandler(t *testing.T) {
	h := newRedirectHandler(func() string { return "http://127.0.0.1:51234/?token=abc" })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 302 || rec.Header().Get("Location") != "http://127.0.0.1:51234/?token=abc" {
		t.Fatalf("redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestRedirectHandler_RefreshOnTarget(t *testing.T) {
	// 闭包: 启动后期 port 已知,handler 每次请求取最新 target
	current := "http://127.0.0.1:11111/?token=first"
	h := newRedirectHandler(func() string { return current })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Header().Get("Location") != "http://127.0.0.1:11111/?token=first" {
		t.Fatalf("first target wrong: %q", rec.Header().Get("Location"))
	}
	current = "http://127.0.0.1:22222/?token=second"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest("GET", "/", nil))
	if rec2.Header().Get("Location") != "http://127.0.0.1:22222/?token=second" {
		t.Fatalf("second target wrong: %q", rec2.Header().Get("Location"))
	}
}
