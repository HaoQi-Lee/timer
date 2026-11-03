// Command shot 抓取指定窗口的屏幕区域并存成 PNG，用于开发期自查界面效果。
//
// 用法：go run ./tools/shot -title 上班计时器 -out shot.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowText            = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextLen         = user32.NewProc("GetWindowTextLengthW")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procPrintWindow              = user32.NewProc("PrintWindow")
	procBitBlt                   = gdi32.NewProc("BitBlt")
	procCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection         = gdi32.NewProc("CreateDIBSection")
	procSelectObject             = gdi32.NewProc("SelectObject")
	procGetDIBits                = gdi32.NewProc("GetDIBits")
	procDeleteObject             = gdi32.NewProc("DeleteObject")
	procDeleteDC                 = gdi32.NewProc("DeleteDC")
	procGetWindowLong            = user32.NewProc("GetWindowLongW")
)

type rect struct {
	Left, Top, Right, Bottom int32
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

func main() {
	title := flag.String("title", "上班计时器", "要抓取的窗口标题")
	out := flag.String("out", "shot.png", "输出 PNG 路径")
	pid := flag.Uint("pid", 0, "只抓这个进程 ID 的窗口，0 表示不限")
	closeOnly := flag.Bool("close", false, "只关闭窗口，不截图")
	flag.Parse()

	hwnd, err := findWindow(*title, uint32(*pid))
	if err != nil {
		fmt.Fprintln(os.Stderr, "找不到窗口:", err)
		os.Exit(1)
	}

	if *closeOnly {
		// 发 WM_CLOSE 让窗口自己正常退出
		procPostMessage := user32.NewProc("PostMessageW")
		procPostMessage.Call(uintptr(hwnd), 0x0010, 0, 0) // WM_CLOSE
		fmt.Println("已请求关闭窗口")
		return
	}

	// 抓到前台来，避免被别的窗口挡住
	procSetForegroundWindow := user32.NewProc("SetForegroundWindow")
	procSetForegroundWindow.Call(uintptr(hwnd))
	time.Sleep(700 * time.Millisecond)

	if err := capture(hwnd, *out); err != nil {
		fmt.Fprintln(os.Stderr, "抓图失败:", err)
		os.Exit(1)
	}
	fmt.Println("已保存", *out)

	// 顺带报一下窗口状态，方便确认置顶是否真的生效
	gwlExStyle := int32(-20) // GWL_EXSTYLE
	exStyle, _, _ := procGetWindowLong.Call(uintptr(hwnd), uintptr(gwlExStyle))
	fmt.Printf("EXSTYLE=0x%08x TOPMOST=%v LAYERED=%v\n", exStyle, exStyle&0x8 != 0, exStyle&0x80000 != 0)

	var r rect
	procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
	fmt.Printf("RECT x=%d y=%d w=%d h=%d\n", r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top)

	// 虚拟屏幕范围，用来判断多显示器布局
	procGetSystemMetrics := user32.NewProc("GetSystemMetrics")
	vx, _, _ := procGetSystemMetrics.Call(76) // SM_XVIRTUALSCREEN
	vy, _, _ := procGetSystemMetrics.Call(77) // SM_YVIRTUALSCREEN
	vw, _, _ := procGetSystemMetrics.Call(78) // SM_CXVIRTUALSCREEN
	vh, _, _ := procGetSystemMetrics.Call(79) // SM_CYVIRTUALSCREEN
	fmt.Printf("VIRTUALSCREEN x=%d y=%d w=%d h=%d\n", vx, vy, vw, vh)
}

// findWindow 按标题找第一个可见的顶层窗口；pid 非 0 时只在该进程里找。
func findWindow(title string, pid uint32) (windows.Handle, error) {
	var found windows.Handle

	callback := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}
		if pid != 0 {
			var owner uint32
			procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
			if owner != pid {
				return 1
			}
		}
		length, _, _ := procGetWindowTextLen.Call(hwnd)
		if length == 0 {
			return 1
		}
		buf := make([]uint16, length+1)
		procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if windows.UTF16ToString(buf) == title {
			found = windows.Handle(hwnd)
			return 0
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	if found == 0 {
		return 0, fmt.Errorf("标题为 %q 的窗口", title)
	}
	return found, nil
}

// capture 把窗口内容画进内存位图再存盘。
func capture(hwnd windows.Handle, out string) error {
	var r rect
	if ret, _, _ := procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r))); ret == 0 {
		return fmt.Errorf("GetWindowRect 失败")
	}
	w := int(r.Right - r.Left)
	h := int(r.Bottom - r.Top)
	if w <= 0 || h <= 0 {
		return fmt.Errorf("窗口尺寸异常 %dx%d", w, h)
	}

	procGetDC := user32.NewProc("GetDC")
	procReleaseDC := user32.NewProc("ReleaseDC")
	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return fmt.Errorf("GetDC 失败")
	}
	defer procReleaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		return fmt.Errorf("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(hdcMem)

	header := bitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:    int32(w),
		Height:   -int32(h), // 负数表示自上而下
		Planes:   1,
		BitCount: 32,
	}
	var bits uintptr
	hbm, _, _ := procCreateDIBSection.Call(
		hdcMem,
		uintptr(unsafe.Pointer(&header)),
		0,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if hbm == 0 {
		return fmt.Errorf("CreateDIBSection 失败")
	}
	defer procDeleteObject.Call(hbm)

	old, _, _ := procSelectObject.Call(hdcMem, hbm)
	defer procSelectObject.Call(hdcMem, old)

	// 优先用 PrintWindow 直接问窗口要内容（能拿到 WebView 合成结果）；
	// 失败就退回到从屏幕拷贝，这样连窗口的半透明效果也能看到。
	method := "PrintWindow"
	if ret, _, _ := procPrintWindow.Call(uintptr(hwnd), hdcMem, 2); ret == 0 {
		method = "BitBlt"
		if ret, _, _ := procBitBlt.Call(hdcMem, 0, 0, uintptr(w), uintptr(h), hdcScreen, uintptr(r.Left), uintptr(r.Top), 0x00CC0020); ret == 0 {
			return fmt.Errorf("PrintWindow 与 BitBlt 都失败了")
		}
	}
	_ = method

	buf := make([]byte, w*h*4)
	procGetDIBits.Call(hdcMem, hbm, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&header)), 0)

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			img.Set(x, y, color.RGBA{buf[i+2], buf[i+1], buf[i], 255})
		}
	}

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
