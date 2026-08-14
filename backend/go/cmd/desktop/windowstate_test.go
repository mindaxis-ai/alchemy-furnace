// windowstate_test.go - L5c 窗口尺寸/位置记忆 单测
// 覆盖: save/load 往返;offscreen 校验
// (注意: wails v2 options.App 不支持 X/Y 字段,本测试只测数据结构,不在 options 应用 X/Y)
package desktop

import (
	"testing"

	"github.com/alchemy-furnace/server/internal/paths"
)

func TestWindowStateRoundTrip(t *testing.T) {
	paths.SetDesktopMode(true)
	paths.SetDataDirOverrideForTest(t.TempDir())
	defer func() { paths.SetDesktopMode(false); paths.SetDataDirOverrideForTest("") }()
	s := &windowState{Width: 1440, Height: 900, X: 100, Y: 80}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	got := loadWindowState()
	if got == nil || got.Width != 1440 || got.Height != 900 || got.X != 100 || got.Y != 80 {
		t.Fatalf("回读失败: %+v", got)
	}
}

func TestWindowStateOffscreen(t *testing.T) {
	s := &windowState{Width: 1280, Height: 800, X: -99999, Y: -99999}
	if !s.offscreen() {
		t.Fatal("屏外坐标应判 true")
	}
	s2 := &windowState{Width: 800, Height: 600, X: 0, Y: 0} // 宽高小于 min
	if !s2.offscreen() {
		t.Fatal("宽高过小应判屏外")
	}
}
