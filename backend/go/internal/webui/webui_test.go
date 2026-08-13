package webui

import (
	"net/http/httptest"
	"testing"
)

func TestHandlerMapping(t *testing.T) {
	h := Handler()
	for _, tc := range []struct {
		path     string
		wantCode int
	}{
		{"/", 200},              // out/index.html
		{"/index.html", 200},    // 精确文件
		{"/no-such-page", 404},  // 回退 404.html,状态 404
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", tc.path, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != tc.wantCode {
			t.Fatalf("%s → %d, want %d", tc.path, rec.Code, tc.wantCode)
		}
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
