//go:build !darwin

// notify_other.go — 非 mac 平台 stub
// Windows: FlashWindowEx 需要窗口 hwnd,wails v2 不导出,留 no-op
// Linux: libnotify 等各有差异,本期不实现
package desktop

func bounceDock() {}
