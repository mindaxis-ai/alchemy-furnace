// main.go - 「炼丹炉」API 网关入口(新框架,Luna-CY 模板结构)
// 子命令: serve(默认)/ migrate up|down|reset / seed
// 启动命令: go run cmd/main/main.go [serve]
package main

import (
	"fmt"
	"os"

	"github.com/alchemy-furnace/server/cmd/main/command"
)

func main() {
	if err := command.NewMainCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "[炼丹炉] 启动失败: %v\n", err)
		os.Exit(1)
	}
}
