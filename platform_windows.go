//go:build windows

package main

import (
	"os"
	"sync"
	"unsafe"

	toast "git.sr.ht/~jackmordaunt/go-toast/v2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Win32：多屏摆位、系统通知、开机自启要用的几个调用。
var (
	procMonitorFromPoint = user32DLL.NewProc("MonitorFromPoint")
	procSetWindowPos     = user32DLL.NewProc("SetWindowPos")
)

// pointVal 把物理屏幕坐标打包成 MonitorFromPoint 要的按值 POINT 参数
// （x86-64 上 8 字节结构体直接进一个寄存器：低 32 位 X、高 32 位 Y，
// 传指针的话会被当成一个垃圾坐标，永远落在屏幕外）。
func pointVal(x, y int) uintptr {
	return uintptr(uint32(x)) | uintptr(uint32(y))<<32
}

// SetWindowPos 的标志位。
const (
	swpNoSize     = 0x0001
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
)

// nativePlaceWindow 按物理绝对坐标把窗口摆到 (x, y)。
//
// 不走 Wails 的 WindowSetPosition：它在 Windows 上是"当前显示器工作区
// 相对坐标"，而记住的位置来自 GetWindowRect 的物理绝对坐标，多屏时会错位。
// 坐标不落在任何一块屏幕上（比如副屏被拔了）时返回 false。
func nativePlaceWindow(title string, x, y int) bool {
	name, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return false
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(name)))
	if hwnd == 0 {
		return false
	}
	// MONITOR_DEFAULTTONULL：坐标不在任何屏幕上时返回 NULL
	mon, _, _ := procMonitorFromPoint.Call(pointVal(x, y), 0)
	if mon == 0 {
		return false
	}
	ret, _, _ := procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0,
		swpNoSize|swpNoZOrder|swpNoActivate)
	return ret != 0
}

// toastOnce 保证应用元数据只注册一次。
var toastOnce sync.Once

// nativeNotify 弹一条 Windows 系统通知。
func nativeNotify(title, body string) {
	toastOnce.Do(func() {
		// 注册应用的 AppUserModelID，通知才能正常显示与分组
		_ = toast.SetAppData(toast.AppData{AppID: mainWindowTitle})
	})
	n := toast.Notification{AppID: mainWindowTitle, Title: title, Body: body}
	_ = n.Push()
}

// 注册表 Run 键：当前用户登录时自动启动。
const (
	runKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "WorkTimer"
)

// applyAutoStart 把"登录自启"写进（或从中删掉）注册表。
func applyAutoStart(on bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if !on {
		if err := key.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// 路径里可能有空格，加引号免得被当成命令行参数截断
	return key.SetStringValue(runValueName, `"`+exe+`"`)
}
