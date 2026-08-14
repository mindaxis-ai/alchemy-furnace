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

	"github.com/gin-gonic/gin"
	"os"
	"sync"

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
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"runtime"
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

	// T3 重构: 先开窗,engineproc + DB 挪到 goroutine,AssetServer 走 splash 三态
	// 就绪口径: engineproc.Start 成功 且(非 demo 时)DB 初始化 + 自动建表成功
	var readyMu sync.Mutex
	var readyOK bool
	var readyErr error
	var stopEngine func() // 可能 nil (开窗后引擎未起完就关窗),OnShutdown 需判空
	go func() {
		_, stop, err := engineproc.Start(context.Background())
		if err == nil && !configuration.IsDemo() {
			if err = dao.InitDatabase(&configuration.Configuration.Database); err == nil {
				err = dao.MaybeAutoMigrate()
			}
		}
		readyMu.Lock()
		stopEngine = stop
		readyOK, readyErr = err == nil, err
		readyMu.Unlock()
	}()

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
	// T6: 桌面 Dock 弹跳端点(走同一 DesktopGuard)
	engine.POST("/api/v1/desktop/notify", func(c *gin.Context) {
		bounceDock()
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": nil})
	})
	srv := &http.Server{Handler: engine}
	go func() {
		log.Printf("[炼丹炉] 桌面服务已就绪: http://%s", addr)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[炼丹炉] 桌面服务退出: %v", err)
		}
	}()

	// T5c: 加载上次窗口状态(nil=默认 1280x800 居中);wails v2 options.App 无 X/Y,只能恢复 W/H
	ws := loadWindowState()
	width, height := 1280, 800
	if ws != nil {
		width, height = ws.Width, ws.Height
	}

	// ALCHEMY_SMOKE=1: CI smoke,只起 HTTP 不开窗,外部 curl 验证后杀进程
	if os.Getenv("ALCHEMY_SMOKE") == "1" {
		log.Printf("[炼丹炉] SMOKE 模式: HTTP 已就绪,等待信号")
		select {}
	}

	app := NewApp()
	err = wails.Run(&options.App{
		Title:    "炼丹炉",
		Width:    width,
		Height:   height,
		MinWidth: 960, MinHeight: 640,
		// T4: macOS 原生菜单栏(关于/退出 + 剪贴板快捷键)
		Menu:     buildAppMenu(),
		// mac: 通顶内容,红绿灯 inset 悬浮(任务 T2)
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About:    &mac.AboutInfo{Title: "炼丹炉", Message: "Alchemy Furnace"},
			// T5a 毛玻璃前提: webview 透明才能透出桌面
			WebviewIsTransparent: true,
		},
		// win: 深色标题栏(任务 T2);globals.css body 底色 #f7f3ed 浅色但 chrome 跟系统区分
		Windows: &windows.Options{
			Theme: windows.Dark,
		},
		// T5a 毛玻璃: alpha=0 让 webview 启动前也是透明(透桌面)
		// (T2 之前用 #f7f3ed 浅色防白闪,与毛玻璃互斥;T5 转入 alpha=0 接受 webview 未渲染前的瞬间透明)
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer: &assetserver.Options{
			Handler: newSplashHandler(
				// ready 时跳到真 origin (T2 已加 platform 参数)
				func() string { return fmt.Sprintf("http://%s/?token=%s&platform=%s", addr, token, runtime.GOOS) },
				// readiness 读 goroutine 写出的共享状态
				func() (bool, error) { readyMu.Lock(); defer readyMu.Unlock(); return readyOK, readyErr },
			),
		},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "com.alchemyfurnace.desktop"},
		Bind:               []interface{}{app},
		OnStartup:          app.startup,
		OnShutdown: func(ctx context.Context) {
			// T5c 关窗落盘窗口几何(必须在 srv.Shutdown 前,ctx 仍有效)
			saveWindowState(ctx)
			_ = srv.Shutdown(ctx)
			readyMu.Lock()
			stop := stopEngine
			readyMu.Unlock()
			if stop != nil {
				stop()
			}
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
