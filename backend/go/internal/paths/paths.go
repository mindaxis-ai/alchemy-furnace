// Package paths 统一解析应用数据目录(桌面端 os.UserConfigDir,自部署 ./data)
//
// 桌面模式 (Wails 壳): ~/Library/Application Support/AlchemyFurnace 等
// 自部署 serve 模式: ./data(保持现状,由 InitDatabase 按需创建)
//
// serve 模式 (cmd/main) 调用 SetDesktopMode(false) 或不调用,DataDir() 返回 ./data
// 桌面模式 (cmd/desktop) 入口第一行 SetDesktopMode(true)
package paths

import (
	"os"
	"path/filepath"
)

const appDirName = "AlchemyFurnace"

var desktop bool
var dataDirOverride string // 仅测试注入,生产代码不应设置

// SetDesktopMode 切换桌面/自部署模式。必须在 DataDir() 首次调用前设置。
func SetDesktopMode(v bool) { desktop = v }

// IsDesktop 当前是否桌面模式
func IsDesktop() bool { return desktop }

// SetDataDirOverrideForTest 注入数据目录,仅测试使用
func SetDataDirOverrideForTest(d string) { dataDirOverride = d }

// DataDir 数据根目录
//   - 测试注入: 返回 dataDirOverride
//   - serve 模式: 返回 "./data"(现状兼容)
//   - desktop 模式: 返回 os.UserConfigDir() + "/AlchemyFurnace"
func DataDir() (string, error) {
	if dataDirOverride != "" {
		return dataDirOverride, nil
	}
	if !desktop {
		return "./data", nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDirName), nil
}

// EnsureDataDir 创建(0700 权限)并返回数据目录;幂等。
//   - serve 模式: 仅返回路径,不创建(由 InitDatabase 决定何时创建)
//   - desktop 模式: 立即 MkdirAll 0700
func EnsureDataDir() (string, error) {
	d, err := DataDir()
	if err != nil {
		return "", err
	}
	if !desktop {
		return d, nil
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}
