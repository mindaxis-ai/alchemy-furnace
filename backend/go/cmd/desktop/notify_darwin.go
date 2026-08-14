//go:build darwin

// notify_darwin.go — macOS Dock 弹跳通知
// NSInformationalRequest: 克制的一次性弹跳(非持续骚扰);窗口已聚焦时调用是 no-op
// (NSApp 拿到的是 wails 启动的 NSApp 单例,直接 requestUserAttention 即可)
package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
static void requestAttention() { [NSApp requestUserAttention:NSInformationalRequest]; }
*/
import "C"

// bounceDock 请求 Dock 一次性弹跳(回合完成时调用)
func bounceDock() { C.requestAttention() }
