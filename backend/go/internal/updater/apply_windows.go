//go:build windows

// apply_windows.go - Windows NSIS 静默安装更新
// 流程: Setup.exe /S 启动 → 主进程 os.Exit(0) 让出文件句柄
// NSIS 安装器会沿用首装位置(InstallDirRegKey)并自动拉起新版
package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// ApplyAndRestart windows 实现:
// Setup.exe /S 静默安装(NSIS 语法),装完 NSIS 自动拉起新版
func ApplyAndRestart(ctx context.Context, setupExe string) error {
	if _, err := os.Stat(setupExe); err != nil {
		return fmt.Errorf("updater: 安装器不存在: %w", err)
	}
	cmd := exec.CommandContext(ctx, setupExe, "/S")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("updater: 启动安装器失败: %w", err)
	}
	// 主进程退出,让出文件句柄
	os.Exit(0)
	return nil
}
