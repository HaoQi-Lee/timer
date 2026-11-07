//go:build !windows

package main

// 非 Windows 平台的桌宠窗口桩实现。
//
// 透明、置顶、隐藏标题栏这些在 macOS 上由 runPet 里的 mac.Options 搞定，
// 不需要 Win32 那套 DWM 处理；逐像素的点击穿透（GDI 窗口区域）是
// Win32 独有能力，macOS 要走 NSWindow 层面，先整窗接收点击。

// unblurPetWindow 在非 Windows 上是空操作：macOS 的窗口透明由
// mac.Options 的 WebviewIsTransparent/WindowIsTranslucent 负责。
func unblurPetWindow() {}

// applyPetMask 在非 Windows 上是空操作：没有窗口区域可套，
// 轮廓蒙版照常记录（SetHitMask），只是不裁剪窗口。
func applyPetMask() {}

// setPetRegionFull 在非 Windows 上是空操作：窗口始终是整窗矩形。
func setPetRegionFull(bool) {}

// hidePetFromTaskbar 在非 Windows 上是空操作：macOS 桌宠窗口本来
// 就不进 Dock 任务栏（程序坞里只有主进程）。
func hidePetFromTaskbar() {}

// waitOutsideClick 非 Windows 上没有实现，等取消就返回（菜单只靠窗口内点击收起）。
func waitOutsideClick(cancel <-chan struct{}) bool {
	<-cancel
	return false
}
