//go:build darwin

// apply_darwin.go - macOS 原位替换更新
// 流程: 等待旧进程退出 → 备份旧版(.old) → 移入新版 → 失败回滚 → open 重启
package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// appBundlePathFromExe 由可执行文件路径上溯 .app 根
// 例: /Applications/AlchemyFurnace.app/Contents/MacOS/AlchemyFurnace → /Applications/AlchemyFurnace.app
// 非 .app 内运行(开发模式)返回错误,不应用更新
func appBundlePathFromExe(exe string) (string, error) {
	i := strings.Index(exe, ".app/")
	if i < 0 {
		return "", fmt.Errorf("updater: 非 .app 内运行,不应用更新: %s", exe)
	}
	return exe[:i+len(".app")], nil
}

// swapScript 等待旧进程退出 → 备份旧版 → 移入新版(失败回滚) → open 重启
//
// pid: 旧进程 ID(等它退出再 swap)
// newApp: 新版 .app 临时目录路径
// appPath: 目标 .app 路径(被替换的旧版)
func swapScript(pid int, newApp, appPath string) string {
	// 顺序: 等死 → 备份旧 → 移新(失败回滚) → 删 .old → open
	// 用 %d/%s/%s 格式化(后两参数 newApp/appPath)
	return fmt.Sprintf(`#!/bin/bash
set -e
APP_PATH=%q
NEW_APP=%q
OLD_PID=%d

# 等待旧进程退出(主进程已 os.Exit(0),但 wails webview 子进程可能未死)
while kill -0 $OLD_PID 2>/dev/null; do sleep 0.2; done
sleep 0.3

# 备份旧版
rm -rf "$APP_PATH.old"
mv "$APP_PATH" "$APP_PATH.old"

# 移入新版(失败回滚)
if ! mv "$NEW_APP" "$APP_PATH"; then
  mv "$APP_PATH.old" "$APP_PATH"
  exit 1
fi

# 清理旧版 + 重启
rm -rf "$APP_PATH.old"
open "$APP_PATH"
`, appPath, newApp, pid)
}

// ApplyAndRestart macOS 实现:
// 1. 定位当前 .app 路径
// 2. 解压新 zip 到临时目录
// 3. 生成 swap 脚本 → 写文件 → exec 启动
// 4. 主进程 os.Exit(0) 让出文件句柄
func ApplyAndRestart(ctx context.Context, zipPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("updater: 获取可执行路径失败: %w", err)
	}
	appPath, err := appBundlePathFromExe(exe)
	if err != nil {
		return err
	}

	// 解压 zip → 临时目录
	tmpDir, err := os.MkdirTemp("", "alchemy-update-*")
	if err != nil {
		return fmt.Errorf("updater: 创建临时目录失败: %w", err)
	}
	newApp := filepath.Join(tmpDir, "AlchemyFurnace.app")
	if err := unzipToDir(ctx, zipPath, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("updater: 解压失败: %w", err)
	}
	// 解压产物必须是 <tmp>/AlchemyFurnace.app
	if _, err := os.Stat(newApp); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("updater: zip 中未找到 AlchemyFurnace.app")
	}

	// 生成 + 启动 swap 脚本
	pid := os.Getpid()
	script := swapScript(pid, newApp, appPath)
	scriptPath := filepath.Join(tmpDir, "swap.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("updater: 写 swap 脚本失败: %w", err)
	}

	cmd := exec.Command("/bin/bash", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("updater: 启动 swap 脚本失败: %w", err)
	}

	// 父进程退出,让 swap 脚本接管
	os.Exit(0)
	return nil
}
