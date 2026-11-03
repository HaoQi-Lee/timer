// Command peinfo 打印可执行文件的 PE 子系统，用来确认双击时会不会弹出控制台窗口。
//
// 用法：go run ./tools/peinfo timer.exe
package main

import (
	"debug/pe"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: peinfo <可执行文件>")
		os.Exit(1)
	}
	f, err := pe.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "打不开:", err)
		os.Exit(1)
	}
	defer f.Close()

	const (
		subsystemGUI     = 2
		subsystemConsole = 3
	)

	var subsystem uint16
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		subsystem = oh.Subsystem
	case *pe.OptionalHeader64:
		subsystem = oh.Subsystem
	}

	switch subsystem {
	case subsystemGUI:
		fmt.Println("子系统: Windows GUI —— 双击不会弹控制台窗口")
	case subsystemConsole:
		fmt.Println("子系统: Windows Console —— 双击会弹一个黑色控制台窗口")
	default:
		fmt.Printf("子系统: %d\n", subsystem)
	}
}
