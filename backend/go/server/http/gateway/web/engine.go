// Package web: engine.go - serve 与 desktop 共享的 gin 引擎装配
package web

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/webui"
	"github.com/alchemy-furnace/server/server/http/middleware"
)

// NewEngine 装配 gin 引擎(中间件 + 路由 + NoRoute 分流)
//
// isDesktop: true → 启用 /api/v1/update/* 端点(desktop 模式专用)
// extraAPIGuards: 仅挂到 /api/v1 组(serve 模式传空,desktop 模式传 DesktopGuard)
// serve 与 desktop 行为差异通过 guards + isDesktop 控制,装配主体共用 → 零回归
func NewEngine(isDesktop bool, extraAPIGuards ...gin.HandlerFunc) (*gin.Engine, error) {
	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.ErrorRecovery(),
		middleware.GinLogger(),
		middleware.CORS(configuration.Configuration.Server.AllowOrigins),
	)
	if err := Register(r, isDesktop, extraAPIGuards...); err != nil {
		return nil, err
	}
	// NoRoute 分流: /api/* JSON 404,其他走 webui(serve/desktop 都有)
	r.NoRoute(func(c *gin.Context) {
		if len(c.Request.URL.Path) >= 5 && c.Request.URL.Path[:5] == "/api/" {
			middleware.NoRouteHandler()(c)
			return
		}
		// gin 在命中 NoRoute 前已把 status 标记为 404,这里先重置为 200,
		// 否则 webui.Handler 即使成功 serve 也会保留 404(WKWebView 显示错误页)
		c.Status(http.StatusOK)
		webui.Handler().ServeHTTP(c.Writer, c.Request)
	})
	r.NoMethod(middleware.NoMethodHandler())
	return r, nil
}
