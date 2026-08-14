// menu.go — L4 macOS 原生菜单栏
// AppMenu: 关于/服务/隐藏 ⌘H/退出 ⌘Q (mac 自动)
// EditMenu: Undo/Redo/Cut/Copy/Paste/SelectAll (⌘Z/⇧⌘Z/⌘X/⌘C/⌘V/⌘A,让 webview 输入框自带快捷键)
// 快捷键(⌘,/⌘N)由前端 desktop-guards.tsx 监听,菜单里不重复(避免两套入口分歧)
package desktop

import (
	"runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
)

// buildAppMenu 仅在 mac 装配菜单;win/linux 走 webview 默认(系统菜单栏不存在)
func buildAppMenu() *menu.Menu {
	m := menu.NewMenu()
	if runtime.GOOS == "darwin" {
		m.Append(menu.AppMenu())
		m.Append(menu.EditMenu())
	}
	return m
}
