// splash_test.go - L3 启动屏三态单测: pending/ready/err
// 覆盖三态返回 HTML 结构(自刷新/JS 跳转/错误页含重试)
package desktop

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSplashStates(t *testing.T) {
	// pending → 启动页(自刷新,火焰动画)
	h := newSplashHandler(
		func() string { return "http://x/?token=t&platform=darwin" },
		func() (bool, error) { return false, nil },
	)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "refresh") {
		t.Fatalf("pending 态应返回自刷新启动页(缺 refresh meta):\n%s", body)
	}
	if !strings.Contains(body, "点燃") {
		t.Fatalf("pending 态应含'点燃'文案:\n%s", body)
	}

	// ready → 跳转页(JS location.replace,避免 WKWebView 跨 scheme 302)
	h = newSplashHandler(
		func() string { return "http://x/?token=t&platform=darwin" },
		func() (bool, error) { return true, nil },
	)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), "window.location.replace") {
		t.Fatalf("ready 态应返回跳转页:\n%s", rec.Body.String())
	}

	// err → 错误页(包含错误原因 + reload 重试按钮)
	h = newSplashHandler(
		nil,
		func() (bool, error) { return false, errors.New("引擎挂了") },
	)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	body = rec.Body.String()
	if !strings.Contains(body, "引擎挂了") {
		t.Fatalf("err 态应含错误原因:\n%s", body)
	}
	if !strings.Contains(body, "location.reload") {
		t.Fatalf("err 态应含重试按钮:\n%s", body)
	}
}
