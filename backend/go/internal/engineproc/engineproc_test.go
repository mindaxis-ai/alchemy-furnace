package engineproc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeRootFromRelativeExecutableIsAbsolute(t *testing.T) {
	root, err := runtimeRootFromExecutable(filepath.Join("build", "bin", "AlchemyFurnace"))
	if err != nil {
		t.Fatalf("runtime root: %v", err)
	}
	if !filepath.IsAbs(root) {
		t.Fatalf("runtime root = %q, want absolute path", root)
	}
}

func TestPythonProcessEnvDisablesBytecodeWrites(t *testing.T) {
	env := pythonProcessEnv([]string{"PATH=/usr/bin", "PYTHONDONTWRITEBYTECODE=0"})
	count := 0
	for _, value := range env {
		if strings.HasPrefix(value, "PYTHONDONTWRITEBYTECODE=") {
			count++
			if value != "PYTHONDONTWRITEBYTECODE=1" {
				t.Fatalf("bytecode env=%q", value)
			}
		}
	}
	if count != 1 {
		t.Fatalf("PYTHONDONTWRITEBYTECODE count=%d, env=%v", count, env)
	}
}

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
