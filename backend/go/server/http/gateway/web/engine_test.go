// engine_test.go - web.NewEngine 端到端装配测试
// 覆盖: serve/desktop 模式差异 + /api/v1 路由组 guard 隔离 + webui NoRoute 分流
package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/paths"
	"github.com/alchemy-furnace/server/server/http/middleware"
)

// TestMain 使用隔离 SQLite，测试与正式运行走同一套 GORM DAO 装配。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "alchemy-web-test-")
	if err != nil {
		panic(err)
	}
	cfg := &configuration.DatabaseConfig{Driver: configuration.DriverSQLite, SQLitePath: filepath.Join(dir, "web.db")}
	if err := dao.InitDatabase(cfg); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = dao.CloseDatabase()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func newReq(t *testing.T, method, path, host, token string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if host != "" {
		req.Host = host
	}
	if token != "" {
		req.Header.Set("X-Alchemy-Token", token)
	}
	return req
}

// TestNewEngine_ServeMode: serve 模式 → 没 update 端点,没 guard,version 全模式
func TestNewEngine_ServeMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	paths.SetDataDirOverrideForTest(t.TempDir())

	r, err := NewEngine(false)
	require.NoError(t, err)

	// /api/v1/version 全模式应可达
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, newReq(t, "GET", "/api/v1/version", "", ""))
	require.Equal(t, 200, rec.Code, "version 端点全模式应可达")
	require.Contains(t, rec.Body.String(), `"version":`)

	// /api/v1/update/check 桌面专有 → 404(serve 模式不注册)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, newReq(t, "GET", "/api/v1/update/check", "", ""))
	require.Equal(t, 404, rec.Code, "serve 模式不应有 update 端点")
}

// TestNewEngine_DesktopMode_Version: desktop 模式 + guard → version 应受 guard 保护
func TestNewEngine_DesktopMode_Version(t *testing.T) {
	gin.SetMode(gin.TestMode)
	paths.SetDataDirOverrideForTest(t.TempDir())

	const token = "abc-token"
	addr := "127.0.0.1:54321"
	r, err := NewEngine(true, middleware.DesktopGuard(token, addr))
	require.NoError(t, err)

	// 无 token → 401
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, newReq(t, "GET", "/api/v1/version", addr, ""))
	require.Equal(t, 401, rec.Code, "缺 token 应被 Guard 拒")

	// 有 token + 正确 host → 200
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, newReq(t, "GET", "/api/v1/version", addr, token))
	require.Equal(t, 200, rec.Code)
	require.Contains(t, rec.Body.String(), `"version":`)
}

// TestNewEngine_DesktopMode_UpdateEndpoints: 三个 update 端点都应受 guard 保护
func TestNewEngine_DesktopMode_UpdateEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	paths.SetDataDirOverrideForTest(t.TempDir())

	const token = "desk-token"
	addr := "127.0.0.1:11111"
	r, err := NewEngine(true, middleware.DesktopGuard(token, addr))
	require.NoError(t, err)

	for _, path := range []string{"/api/v1/update/check", "/api/v1/update/progress"} {
		t.Run(path, func(t *testing.T) {
			// 无 token → 401
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, newReq(t, "GET", path, addr, ""))
			require.Equal(t, 401, rec.Code, "无 token 应被 Guard 拒")

			// 有 token → 端点可达(200,进度或 has_update 字段)
			rec = httptest.NewRecorder()
			r.ServeHTTP(rec, newReq(t, "GET", path, addr, token))
			// /check 走真实 GitHub(可能失败),/progress 走 atomic 返 0
			if strings.Contains(path, "progress") {
				require.Equal(t, 200, rec.Code)
				require.Contains(t, rec.Body.String(), `"progress":`)
			} else {
				// check 可能被网络拦截,但走通到 updater.CheckLatest 内部
				require.Contains(t, []int{200, 500}, rec.Code)
			}
		})
	}

	// POST /update/apply 也应被 guard 保护(未授权直接 401)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, newReq(t, "POST", "/api/v1/update/apply", addr, ""))
	require.Equal(t, 401, rec.Code)
}

// TestNewEngine_NoRoute: /api/* 走 JSON 404;非 /api 走 webui 嵌入
func TestNewEngine_NoRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	paths.SetDataDirOverrideForTest(t.TempDir())

	r, err := NewEngine(false)
	require.NoError(t, err)

	// /api/xxx → JSON 404
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, newReq(t, "GET", "/api/xxx", "", ""))
	require.Equal(t, 404, rec.Code)
	require.Contains(t, rec.Body.String(), `"message":`, "API 路径应回 JSON 信封")

	// / 走 webui: 应返 200 + body 含 webui HTML(占位或真实)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, newReq(t, "GET", "/", "", ""))
	require.Equal(t, http.StatusOK, rec.Code, "根路径必须 200(WKWebView 404 会显示错误页)")
	body := rec.Body.String()
	require.True(t, strings.Contains(body, "炼丹炉 webui 占位") ||
		strings.Contains(body, "<title>炼丹炉"),
		"根路径应回 webui 内容")
	// 任意子路径 SPA fallback: 也应 200,body 仍是首页(便于 client-side router 接管)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, newReq(t, "GET", "/chat/abc", "", ""))
	require.Equal(t, http.StatusOK, rec.Code, "子路径 SPA fallback 也应 200")
	body = rec.Body.String()
	require.True(t, strings.Contains(body, "炼丹炉 webui 占位") ||
		strings.Contains(body, "<title>炼丹炉"),
		"SPA fallback body 应为 webui HTML")
}
