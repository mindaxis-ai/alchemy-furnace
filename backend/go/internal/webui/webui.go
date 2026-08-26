// Package webui 内嵌前端静态产物(output:export 的 frontend/out),单二进制跑全站
//
// 真实产物由 Task 12 打包脚本从 frontend/out 拷贝到 out/ 覆盖;
// 占位 index.html/404.html 提交入库,保证 embed 可编译,开发期能 serve。
package webui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
)

//go:embed all:out
var outFS embed.FS

// defaultLocaleHTML 返回 next 16 output:export 生成的 locale HTML;
// 同时作为 / 路径的"真实页面"优先级:zh-CN > en > index.html(占位兜底)
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

// Handler 静态文件服务 + SPA fallback + 正确 Content-Type
//
// 映射:精确文件 → Next export 的 route.html → 动态 route/_.html → 默认页 → 404
//
// 根路径 / 处理:优先默认 locale(zh-CN.html)→ index.html(占位)
//   - 生产:real build 有 zh-CN.html,直接服务真实首页
//   - 开发:无 real build,只有 index.html 占位,服务占位
//
// 缓存策略:
//   - _next/*  immutable 1 年(Next.js 静态资源,文件名带 hash)
//   - *.html   no-cache(每次构建 hash 变,需重新验证)
//
// Content-Type: 用 mime.TypeByExtension 自动识别
//
//	.css → text/css; .js → application/javascript; .svg → image/svg+xml 等
func Handler() http.Handler {
	sub, _ := fs.Sub(outFS, "out")
	defaultHTML := defaultLocaleHTML(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		// 旧版深链 /chat/<uuid> 规范化到 /chat?session=<uuid>（307，保留查询含桌面 token）
		if sessionID, ok := legacyChatSessionID(p); ok {
			query := r.URL.Query()
			query.Set("session", sessionID)
			http.Redirect(w, r, "/chat?"+query.Encode(), http.StatusTemporaryRedirect)
			return
		}
		// 根路径:优先默认 locale(zh-CN),回退 index.html 占位
		if p == "" {
			if serveFile(w, sub, defaultHTML) {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		if strings.HasPrefix(p, "_next/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		// 1) 精确文件 / Next.js output:export 的扁平 route.html / 目录 index
		if serveFile(w, sub, p) || serveFile(w, sub, p+".html") || serveFile(w, sub, p+"/index.html") {
			w.WriteHeader(http.StatusOK)
			return
		}
		// 2) 动态导出路由: /pills/<id>、/agents/<id> → <dir>/_.html。
		//    注意:/chat/<uuid> 已在上方 307 规范化,不会落入这里。
		if !strings.Contains(path.Base(p), ".") {
			if dir := path.Dir(p); dir != "." && serveFile(w, sub, path.Join(dir, "_.html")) {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		// 3) 未导出的无扩展路径回退默认页，让客户端路由接管。
		base := path.Base(p)
		if base == "index.html" || !strings.Contains(base, ".") {
			if serveFile(w, sub, defaultHTML) {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		// 4) 真没有 → 404 页
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		if b, err := fs.ReadFile(sub, "404.html"); err == nil {
			w.Write(b)
		}
	})
}

// legacyChatSessionID 识别旧版会话深链 /chat/<uuid>。
// 只有恰好两段、首段为 chat、第二段为合法 UUID 时才匹配;
// 其余 /chat/* 路径（如 /chat/settings）不视为会话，返回 ok=false。
func legacyChatSessionID(cleanPath string) (string, bool) {
	parts := strings.Split(strings.Trim(cleanPath, "/"), "/")
	if len(parts) != 2 || parts[0] != "chat" {
		return "", false
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return "", false
	}
	return parts[1], true
}

// serveFile 读取 embed 文件并写响应(正确 Content-Type + 缓存头),成功返回 true
func serveFile(w http.ResponseWriter, sub fs.FS, name string) bool {
	b, err := fs.ReadFile(sub, name)
	if err != nil {
		return false
	}
	ext := strings.ToLower(path.Ext(name))
	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else if ext == ".html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	if ext == ".html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Write(b)
	return true
}
