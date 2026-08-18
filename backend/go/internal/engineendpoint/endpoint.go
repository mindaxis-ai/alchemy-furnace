// Package engineendpoint 保存桌面内嵌语言引擎的当前地址。
//
// 桌面端会先装配 HTTP 路由，再异步启动 Python 引擎；因此服务不能在构造时
// 固化配置中的默认端口。Provider 让每次请求读取健康检查通过后的最新地址。
package engineendpoint

import (
	"sync/atomic"

	"github.com/alchemy-furnace/server/internal/configuration"
)

// Provider 返回当前语言引擎 BaseURL。
type Provider func() string

var current atomic.Value

// Current 返回运行时最新地址；尚未动态设置时回退到配置值（Web/测试兼容）。
func Current() string {
	if value := current.Load(); value != nil {
		return value.(string)
	}
	return configuration.Configuration.PythonEngine.BaseURL
}

// Set 在内嵌引擎健康检查通过后发布最新地址。
func Set(baseURL string) {
	current.Store(baseURL)
}

// Static 为单元测试和固定地址客户端生成 Provider。
func Static(baseURL string) Provider {
	return func() string { return baseURL }
}
