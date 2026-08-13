// Package web: engine.go - serve 与 desktop 共享的 gin 引擎装配
package web

import (
	"github.com/gin-gonic/gin"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/webui"
	"github.com/alchemy-furnace/server/server/http/middleware"
)

// NewEngine 装配 gin 引擎(中间件 + 路由 + NoRoute 分流)
//
// extraAPIGuards: 仅挂到 /api/v1 组(serve 模式传空,desktop 模式传 DesktopGuard)
// serve 与 desktop 行为差异通过 guards 控制,装配主体共用 → 零回归
func NewEngine(extraAPIGuards ...gin.HandlerFunc) (*gin.Engine, error) {
	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.ModeHeader(),
		middleware.ErrorRecovery(),
		middleware.GinLogger(),
		middleware.CORS(configuration.Configuration.Server.AllowOrigins),
	)
	if err := Register(r, extraAPIGuards...); err != nil {
		return nil, err
	}
	// NoRoute 分流: /api/* JSON 404,其他走 webui(serve/desktop 都有)
	r.NoRoute(func(c *gin.Context) {
		if len(c.Request.URL.Path) >= 5 && c.Request.URL.Path[:5] == "/api/" {
			middleware.NoRouteHandler()(c)
			return
		}
		webui.Handler().ServeHTTP(c.Writer, c.Request)
	})
	r.NoMethod(middleware.NoMethodHandler())
	return r, nil
}
