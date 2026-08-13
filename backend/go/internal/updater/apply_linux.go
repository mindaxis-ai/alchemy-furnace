//go:build linux

// apply_linux.go - 桌面分发不支持 Linux
// 保留 ApplyAndRestart 符号使 cross-compile 不会失败;
// 运行时直接返错,告诉用户桌面分发仅支持 macOS / Windows
package updater

import (
	"context"
	"fmt"
	"runtime"
)

// ApplyAndRestart linux 不支持(桌面分发仅 macOS / Windows)
func ApplyAndRestart(_ context.Context, _ string) error {
	return fmt.Errorf("updater: 桌面分发不支持 %s,仅 macOS / Windows", runtime.GOOS)
}
