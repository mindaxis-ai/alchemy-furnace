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

// Handler 静态文件服务,映射: 精确文件 → 目录/index.html → 404.html(404)
//
// 缓存策略:
//   - _next/*  immutable 1 年(Next.js 静态资源,文件名带 hash)
//   - *.html   no-cache(每次构建 hash 变,需重新验证)
func Handler() http.Handler {
	sub, _ := fs.Sub(outFS, "out")
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
		if serve(p) || serve(p+"/index.html") {
			return
		}
		// 回退 404
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		if b, err := fs.ReadFile(sub, "404.html"); err == nil {
			w.Write(b)
		}
	})
}
