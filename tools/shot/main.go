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
	procSetCursorPos             = user32.NewProc("SetCursorPos")
	procMouseEvent               = user32.NewProc("mouse_event")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procShowWindow               = user32.NewProc("ShowWindow")
)

type rect struct {
	Left, Top, Right, Bottom int32
}

type point struct {
	X, Y int32
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
	dragDX := flag.Int("dragdx", 0, "在窗口里按住并横向拖动多少像素，用来验证拖拽区域")
	dragY := flag.Int("dragy", 0, "从窗口顶部往下多少像素处按下鼠标")
	clickX := flag.Int("clickx", -1, "在窗口里点一下，坐标相对窗口左上角")
	clickY := flag.Int("clicky", 0, "点击位置的纵坐标（相对窗口左上角）")
	cropX := flag.Int("cropx", 0, "只截窗口里的这一块（相对窗口左上角）")
	cropY := flag.Int("cropy", 0, "截取区域的纵坐标")
	cropW := flag.Int("cropw", 0, "截取区域的宽度，0 表示截整个窗口")
	cropH := flag.Int("croph", 0, "截取区域的高度，0 表示截整个窗口")
	zoom := flag.Int("zoom", 1, "放大倍数")
	list := flag.Bool("list", false, "只列出该进程的所有窗口标题")
	closePID := flag.Uint("closepid", 0, "关掉这个进程的第一个可见窗口（不看标题）")
	flag.Parse()

	if *list {
		listWindows(uint32(*pid))
		return
	}

	if *closePID != 0 {
		h, err := findAnyWindow(uint32(*closePID))
		if err != nil {
			fmt.Fprintln(os.Stderr, "找不到窗口:", err)
			os.Exit(1)
		}
		procPostMessage := user32.NewProc("PostMessageW")
		procPostMessage.Call(uintptr(h), 0x0010, 0, 0) // WM_CLOSE
		fmt.Println("已请求关闭窗口")
		return
	}

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

	if *dragDX != 0 {
		drag(hwnd, *dragDX, *dragY)
	}

	if *clickX >= 0 {
		click(hwnd, *clickX, *clickY)
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

	if *cropW > 0 && *cropH > 0 {
		if err := cropZoom(*out, *cropX, *cropY, *cropW, *cropH, *zoom); err != nil {
			fmt.Fprintln(os.Stderr, "裁剪失败:", err)
			os.Exit(1)
		}
		fmt.Printf("已裁剪放大 %dx%d -> %s\n", *cropW, *cropH, *out)
	}

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

// drag 在窗口里按住鼠标横向拖一段，用来验证这块区域能不能拖动窗口。
//
// 拖完会把鼠标指针挪回原处，不打扰使用者。
func drag(hwnd windows.Handle, dx, offsetY int) {
	var r rect
	procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))

	var origin point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&origin)))

	// 原生拖拽走的是 WM_NCLBUTTONDOWN，窗口最好在前台，
	// 否则 Windows 可能不理这条消息。
	procSetForegroundWindow.Call(uintptr(hwnd))
	time.Sleep(200 * time.Millisecond)

	sx := int(r.Left) + int(r.Right-r.Left)/2
	sy := int(r.Top) + offsetY

	const (
		leftDown = 0x0002
		leftUp   = 0x0004
		moveRel  = 0x0001
	)

	procSetCursorPos.Call(uintptr(sx), uintptr(sy))
	time.Sleep(150 * time.Millisecond)
	procMouseEvent.Call(leftDown, 0, 0, 0, 0)
	time.Sleep(150 * time.Millisecond)
	// 分几步移动，模拟真人拖动
	steps := 8
	for i := 1; i <= steps; i++ {
		procMouseEvent.Call(moveRel, uintptr(dx*i/steps), 0, 0, 0)
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)
	procMouseEvent.Call(leftUp, 0, 0, 0, 0)

	// 指针挪回原处
	procSetCursorPos.Call(uintptr(origin.X), uintptr(origin.Y))

	var after rect
	procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&after)))
	fmt.Printf("拖拽前 x=%d y=%d，拖拽后 x=%d y=%d\n",
		r.Left, r.Top, after.Left, after.Top)

	// 再连着采样一小段时间，看看窗口是不是还在跟着鼠标走。
	for i := 0; i < 8; i++ {
		time.Sleep(120 * time.Millisecond)
		var s rect
		procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&s)))
		fmt.Printf("  +%dms x=%d y=%d\n", (i+1)*120, s.Left, s.Top)
	}
}

// click 在窗口里点一下，用来验证按钮还能不能正常响应。
func click(hwnd windows.Handle, x, y int) {
	var r rect
	procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))

	var origin point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&origin)))

	procSetForegroundWindow.Call(uintptr(hwnd))
	time.Sleep(150 * time.Millisecond)

	sx := int(r.Left) + x
	sy := int(r.Top) + y

	const (
		leftDown = 0x0002
		leftUp   = 0x0004
	)

	procSetCursorPos.Call(uintptr(sx), uintptr(sy))
	time.Sleep(120 * time.Millisecond)
	procMouseEvent.Call(leftDown, 0, 0, 0, 0)
	time.Sleep(80 * time.Millisecond)
	procMouseEvent.Call(leftUp, 0, 0, 0, 0)
	time.Sleep(300 * time.Millisecond)

	procSetCursorPos.Call(uintptr(origin.X), uintptr(origin.Y))

	var after rect
	procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&after)))
	fmt.Printf("点击 (%d,%d)：窗口 x %d -> %d\n", x, y, r.Left, after.Left)

	// 点了之后连着看一会儿窗口状态：最小化是异步的，
	// 前端调完接口才轮到 Go 那边 ShowWindow。
	iconic := uintptr(0)
	for i := 0; i < 10; i++ {
		time.Sleep(120 * time.Millisecond)
		iconic, _, _ = procIsIconic.Call(uintptr(hwnd))
		if iconic != 0 {
			fmt.Printf("  +%dms 已最小化\n", (i+1)*120)
			break
		}
	}
	if iconic == 0 {
		fmt.Println("  窗口没有最小化")
	}

	// 点了最小化就把窗口放回来，别把用户的计时器藏起来。
	if iconic != 0 {
		procShowWindow.Call(uintptr(hwnd), 9) // SW_RESTORE
		time.Sleep(300 * time.Millisecond)
	}
}

// findAnyWindow 找出该进程的第一个可见顶层窗口，不管标题是什么。
func findAnyWindow(pid uint32) (windows.Handle, error) {
	var found windows.Handle

	callback := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}
		var owner uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			return 1
		}
		found = windows.Handle(hwnd)
		return 0
	})
	procEnumWindows.Call(callback, 0)
	if found == 0 {
		return 0, fmt.Errorf("进程 %d 的可见窗口", pid)
	}
	return found, nil
}

// listWindows 列出进程里所有可见窗口的标题。
func listWindows(pid uint32) {
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
		var owner uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		fmt.Printf("pid=%d hwnd=%d 标题=%q\n", owner, hwnd, windows.UTF16ToString(buf))
		return 1
	})
	procEnumWindows.Call(callback, 0)
}

// cropZoom 把刚存下来的图裁一块再放大，方便看清按钮位置。
func cropZoom(path string, x, y, w, h, zoom int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	src, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		return err
	}

	if zoom < 1 {
		zoom = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, w*zoom, h*zoom))
	for dy := 0; dy < h*zoom; dy++ {
		for dx := 0; dx < w*zoom; dx++ {
			dst.Set(dx, dy, src.At(x+dx/zoom, y+dy/zoom))
		}
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, dst)
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

	// 优先从屏幕拷贝：PrintWindow 会把窗口边框画成白边，
	// 截出来的图和真实位置对不上，按像素点击就点不准了。
	// 窗口已经置前并等了一会儿，屏幕上看到的就是它的样子。
	method := "BitBlt"
	if ret, _, _ := procBitBlt.Call(hdcMem, 0, 0, uintptr(w), uintptr(h), hdcScreen, uintptr(r.Left), uintptr(r.Top), 0x00CC0020); ret == 0 {
		method = "PrintWindow"
		if ret, _, _ := procPrintWindow.Call(uintptr(hwnd), hdcMem, 2); ret == 0 {
			return fmt.Errorf("BitBlt 与 PrintWindow 都失败了")
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
