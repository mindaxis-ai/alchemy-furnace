// cmd_shim.go - 保留 go run ./cmd/desktop 入口(packge main 兼容)
package main

import (
	"github.com/alchemy-furnace/server/cmd/desktop"
)

func main() { desktop.Run() }
