package engineproc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPickPort(t *testing.T) {
	p1, err := pickPort()
	if err != nil || p1 <= 0 {
		t.Fatalf("pickPort=%d,%v", p1, err)
	}
}

func TestWaitHealthyOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()
	if err := waitHealthy(context.Background(), srv.URL, 3*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitHealthyTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	if err := waitHealthy(context.Background(), srv.URL, 1200*time.Millisecond); err == nil {
		t.Fatal("应超时")
	}
}

func TestResolveRuntimeRootEnv(t *testing.T) {
	t.Setenv("ALCHEMY_PYTHON_RUNTIME", "/tmp/fake-rt")
	got, err := ResolveRuntimeRoot()
	if err != nil || got != "/tmp/fake-rt" {
		t.Fatalf("env 优先: %q,%v", got, err)
	}
}
