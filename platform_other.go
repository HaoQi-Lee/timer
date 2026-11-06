//go:build !windows

package main

import (
	"fmt"
	"os/exec"
	goruntime "runtime"
)

// nativePlaceWindow 非 Windows 上没有原生摆位，返回 false 让调用方走 Wails 兜底。
func nativePlaceWindow(string, int, int) bool { return false }

// nativeNotify macOS 用 osascript 弹系统通知，其它平台先不弹。
func nativeNotify(title, body string) {
	if goruntime.GOOS != "darwin" {
		return
	}
	script := fmt.Sprintf(`display notification %q with title %q`, body, title)
	_ = exec.Command("osascript", "-e", script).Start()
}

// applyAutoStart 非 Windows 暂不支持（macOS 要配登录项，后续再说）。
func applyAutoStart(bool) error { return nil }
