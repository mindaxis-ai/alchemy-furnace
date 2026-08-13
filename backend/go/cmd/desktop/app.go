// cmd/desktop/app.go - Wails 绑定对象(任务 11 扩展)
// 注意: webview 重定向到 http origin 后 Wails Bind 不可达,仅作兜底
// 版本/更新等正式接口走 HTTP 端点(/api/v1/version 全模式;update 三端点仅 desktop,Guard 保护)
package desktop

import (
	"context"

	"github.com/alchemy-furnace/server/internal/buildinfo"
	"github.com/alchemy-furnace/server/internal/configuration"
)

// App 桌面绑定对象(Wails 自动扫描导出方法,经 JS 桥调用)
type App struct{ ctx context.Context }

// NewApp 构造应用对象
func NewApp() *App { return &App{} }

// startup 窗口启动回调(Wails 注入 ctx,此处仅记录,无 Web 端调用)
func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// GetVersion 版本信息(经 HTTP /api/v1/version 也可取,任务 11 落 HTTP)
func (a *App) GetVersion() map[string]string {
	return map[string]string{
		"version":   buildinfo.Version,
		"commit":    buildinfo.Commit,
		"buildDate": buildinfo.BuildDate,
		"mode":      configuration.Mode(),
	}
}
