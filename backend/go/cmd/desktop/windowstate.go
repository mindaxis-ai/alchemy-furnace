// windowstate.go — L5c 窗口尺寸/位置记忆
// 落盘: paths.DataDir()/window-state.json
// 应用: wails v2 options.App 只有 Width/Height 字段(无 X/Y),所以 Width/Height 可恢复,X/Y 仅落盘
// (wails v2 不支持设置起始位置;若 v3 升级可补回 X/Y 恢复)
package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/alchemy-furnace/server/internal/paths"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type windowState struct{ Width, Height, X, Y int }

// offscreen 宽高过小或坐标超出合理范围(多屏拔除/分辨率变更后常见)即视为无效
func (s *windowState) offscreen() bool {
	return s.X < -8192 || s.Y < -8192 || s.X > 16384 || s.Y > 16384 || s.Width < 960 || s.Height < 640
}

// save 写入数据目录(若 EnsureDataDir 失败仅返回 err,由调用方决定;关窗路径上 OnShutdown 调,_ 吃掉)
func (s *windowState) save() error {
	dir, err := paths.EnsureDataDir()
	if err != nil {
		return err
	}
	b, _ := json.Marshal(s)
	return os.WriteFile(filepath.Join(dir, "window-state.json"), b, 0o600)
}

// loadWindowState 读窗口状态;文件不存在/解析失败/offscreen 都返回 nil(走默认 1280x800)
func loadWindowState() *windowState {
	dir, err := paths.DataDir()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "window-state.json"))
	if err != nil {
		return nil
	}
	var s windowState
	if json.Unmarshal(b, &s) != nil || s.offscreen() {
		return nil
	}
	return &s
}

// saveWindowState 关窗时落盘当前几何(X/Y wails v2 应用不到下次启动,仅落盘以便未来用)
func saveWindowState(ctx context.Context) {
	w, h := wailsruntime.WindowGetSize(ctx)
	x, y := wailsruntime.WindowGetPosition(ctx)
	_ = (&windowState{w, h, x, y}).save()
}
