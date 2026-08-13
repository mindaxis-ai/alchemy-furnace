// Package webui 内嵌前端静态产物(output:export 的 frontend/out),单二进制跑全站
//
// 真实产物由 Task 12 打包脚本从 frontend/out 拷贝到 out/ 覆盖;
// 占位 index.html/404.html 提交入库,保证 embed 可编译,开发期能 serve。
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed out
var outFS embed.FS

// defaultLocaleHTML 返回 next 16 output:export 生成的 locale HTML;
// desktop 模式没有 index.html,根路径与 SPA fallback 都用它
func defaultLocaleHTML(sub fs.FS) string {
	if _, err := fs.Stat(sub, "zh-CN.html"); err == nil {
		return "zh-CN.html"
	}
	if _, err := fs.Stat(sub, "en.html"); err == nil {
		return "en.html"
	}
	if _, err := fs.Stat(sub, "index.html"); err == nil {
		return "index.html"
	}
	return "404.html"
}

// Handler 静态文件服务 + SPA fallback
//
// 映射: 精确文件 → 目录/index.html → 默认 locale(SPA fallback)→ 404.html
//
// 缓存策略:
//   - _next/*  immutable 1 年(Next.js 静态资源,文件名带 hash)
//   - *.html   no-cache(每次构建 hash 变,需重新验证)
func Handler() http.Handler {
	sub, _ := fs.Sub(outFS, "out")
	defaultHTML := defaultLocaleHTML(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if strings.HasPrefix(p, "_next/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		serve := func(name string) bool {
			b, err := fs.ReadFile(sub, name)
			if err != nil {
				return false
			}
			if strings.HasSuffix(name, ".html") {
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
			}
			w.Write(b)
			return true
		}
		// 1) 精确文件 / 目录/index.html
		if serve(p) || serve(p+"/index.html") {
			w.WriteHeader(http.StatusOK)
			return
		}
		// 2) SPA fallback:找不到资源 + 路径里没点 → client-side router 接管
		// 覆盖两种情况:
		//   a) p == "index.html" (来自 / 路径,index.html 不存在 → 默认 locale)
		//   b) p 是 /chat/abc 这类(无扩展名)→ 默认 locale
		base := path.Base(p)
		if base == "index.html" || !strings.Contains(base, ".") {
			if serve(defaultHTML) {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		// 3) 真没有 → 404 页
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		if b, err := fs.ReadFile(sub, "404.html"); err == nil {
			w.Write(b)
		}
	})
}
