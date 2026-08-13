// Package middleware: desktop_guard.go - 仅 desktop 入口挂载
//
// 防两类威胁:
//   1. DNS rebinding: 浏览器 webview 加载 127.0.0.1,恶意网页诱导 fetch 127.0.0.1:PORT
//   2. 本机其他进程: 任何能连端口的进程都能调本地 API
//
// 防护: Host 必等监听 addr + X-Alchemy-Token 恒定时间比较
package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// DesktopGuard 校验 Host(防 DNS rebinding) + X-Alchemy-Token(每启动随机)
//
// token: 启动时随机生成的共享密钥(由 cmd/desktop 注入到 webview)
// addr: 实际监听地址(127.0.0.1:PORT),与 token 配套
func DesktopGuard(token, addr string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Host != addr ||
			subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Alchemy-Token")), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 40101, "message": "未授权的本地访问", "data": nil,
			})
			return
		}
		c.Next()
	}
}
