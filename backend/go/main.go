// main.go - Wails build 期望的项目根 main 入口
// 桌面端入口(serve 模式从 cmd/main 起,本文件仅供 wails build 用)
// ALCHEMY_SMOKE=1 时只起 HTTP 不开窗
package main

import (
	"github.com/alchemy-furnace/server/cmd/desktop"
)

func main() { desktop.Run() }
