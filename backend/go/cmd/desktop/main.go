// cmd/desktop/main.go - 桌面入口: 数据目录→secret→Python 编排→随机端口→Wails 开窗
// ALCHEMY_SMOKE=1 时只起 HTTP 不开窗(CI 无显示环境 smoke 用)
//
// 安全: 127.0.0.1 随机端口 + token + Host 头校验(middleware.DesktopGuard)
package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/alchemy-furnace/server/internal/configuration"
	"github.com/alchemy-furnace/server/internal/configuration/loader"
	"github.com/alchemy-furnace/server/internal/dao"
	"github.com/alchemy-furnace/server/internal/engineproc"
	"github.com/alchemy-furnace/server/internal/logger"
	"github.com/alchemy-furnace/server/internal/paths"
	"github.com/alchemy-furnace/server/server/http/gateway/web"
	"github.com/alchemy-furnace/server/server/http/middleware"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// newRedirectHandler webview 资源处理器: 任何请求都返回 200 + JS 跳转到 http origin
// 为什么不用 302: WKWebView 跨 scheme 302(wails:// → http://) 经常不跟随,显示白屏
// 用 JS window.location.replace 强制同 frame 跳转(JS 跳转不经过 scheme 协商)
func newRedirectHandler(target func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><title>炼丹炉</title><body><script>window.location.replace(%q);</script></body>`, target())
	})
}

func main() {
	paths.SetDesktopMode(true)
	if _, err := paths.EnsureDataDir(); err != nil {
		log.Fatalf("[炼丹炉] 数据目录不可用: %v", err)
	}
	if err := loader.LoadConfig(""); err != nil {
		log.Fatalf("[炼丹炉] 加载配置失败: %v", err)
	}
	if err := logger.Init(configuration.Configuration.Server.Mode); err != nil {
		log.Fatalf("[炼丹炉] 初始化日志失败: %v", err)
	}

	_, stopEngine, err := engineproc.Start(context.Background())
	if err != nil {
		log.Fatalf("[炼丹炉] Python 引擎启动失败: %v", err)
	}

	if !configuration.IsDemo() {
		if err := dao.InitDatabase(&configuration.Configuration.Database); err != nil {
			log.Fatalf("[炼丹炉] 初始化数据库失败: %v", err)
		}
		if err := dao.MaybeAutoMigrate(); err != nil {
			log.Fatalf("[炼丹炉] 自动建表失败: %v", err)
		}
	}

	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		log.Fatalf("[炼丹炉] token 生成失败: %v", err)
	}
	token := hex.EncodeToString(tok)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("[炼丹炉] 端口监听失败: %v", err)
	}
	addr := ln.Addr().String()
	engine, err := web.NewEngine(true, middleware.DesktopGuard(token, addr))
	if err != nil {
		log.Fatalf("[炼丹炉] 装配路由失败: %v", err)
	}
	srv := &http.Server{Handler: engine}
	go func() {
		log.Printf("[炼丹炉] 桌面服务已就绪: http://%s", addr)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[炼丹炉] 桌面服务退出: %v", err)
		}
	}()

	// ALCHEMY_SMOKE=1: CI smoke,只起 HTTP 不开窗,外部 curl 验证后杀进程
	if os.Getenv("ALCHEMY_SMOKE") == "1" {
		log.Printf("[炼丹炉] SMOKE 模式: HTTP 已就绪,等待信号")
		select {}
	}

	app := NewApp()
	err = wails.Run(&options.App{
		Title:    "炼丹炉",
		Width:    1280,
		Height:   800,
		MinWidth: 960, MinHeight: 640,
		AssetServer: &assetserver.Options{
			Handler: newRedirectHandler(func() string {
				return fmt.Sprintf("http://%s/?token=%s", addr, token)
			}),
		},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "com.alchemyfurnace.desktop"},
		Bind:               []interface{}{app},
		OnStartup:          app.startup,
		OnShutdown: func(ctx context.Context) {
			_ = srv.Shutdown(ctx)
			stopEngine()
			dao.CloseDatabase()
		},
	})
	if err != nil {
		log.Fatalf("[炼丹炉] 窗口启动失败: %v", err)
	}
}

// Run 桌面入口(供 backend/go/main.go shim 与 cmd/desktop-main/main.go 调用)
// 与原 main() 行为一致(数据目录→secret→Python→随机端口→Wails 开窗;ALCHEMY_SMOKE=1 不开窗)
func Run() {
	main()
}
