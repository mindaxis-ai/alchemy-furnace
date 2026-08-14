// splash.go — L3 启动屏三态 handler
// pending: 火焰动画 + 1s 自刷新(等 engineproc + DB 就绪)
// ready:   JS window.location.replace 跳到真 origin(沿用 newRedirectHandler 的坑注释: WKWebView 跨 scheme 302 不跟随)
// err:     错误页含 reload 重试按钮
// 自包含内联 CSS,无外部资源(防 wails asset server 还没起就 404)
package desktop

import (
	"fmt"
	"net/http"
)

const splashCSS = `body{margin:0;height:100vh;display:flex;flex-direction:column;align-items:center;justify-content:center;background:#09090b;color:#d4d4d8;font-family:ui-sans-serif,system-ui}
.flame{font-size:56px;animation:f 1.2s ease-in-out infinite}
@keyframes f{0%,100%{transform:scale(1);opacity:.85}50%{transform:scale(1.12);opacity:1}}
.msg{margin-top:18px;font-size:14px;opacity:.75;letter-spacing:.08em}
.err{color:#f87171;font-size:13px;max-width:70%;text-align:center}
.retry{margin-top:16px;padding:8px 22px;border:1px solid #52525b;border-radius:8px;color:#d4d4d8;cursor:pointer;background:transparent}
.retry:hover{background:#27272a}`

// newSplashHandler 返回三态 http.Handler
// - readiness 由调用方提供(查询 engineproc.Start + DB 是否就绪,可能为 nil 表示直接走 ready 路径)
// - target 提供 ready 时的跳转 URL
func newSplashHandler(target func() string, readiness func() (bool, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var ready bool
		var err error
		if readiness != nil {
			ready, err = readiness()
		}
		switch {
		case err != nil:
			fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><title>炼丹炉</title><style>%s</style>
<body><div class=flame>🔥</div><p class=msg>丹炉点火失败 · Failed to ignite</p>
<p class=err>%s</p><button class=retry onclick="location.reload()">重试 · Retry</button>`, splashCSS, err.Error())
		case ready:
			// JS replace 而非 302: WKWebView 跨 scheme 302 (wails:// → http://) 经常不跟随 → 白屏
			fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><title>炼丹炉</title><body><script>window.location.replace(%q);</script></body>`, target())
		default:
			// 1 秒自刷新,持续轮询 readiness
			fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><meta http-equiv=refresh content=1><title>炼丹炉</title><style>%s</style>
<body><div class=flame>🔥</div><p class=msg>正在点燃丹炉 · Lighting the furnace…</p>`, splashCSS)
		}
	})
}
