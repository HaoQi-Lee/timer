//go:build windows

package main

import (
	"os"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32：把窗口背景效果从"模糊穿透"换成"纯透明"用。
var (
	user32DLL                         = windows.NewLazySystemDLL("user32.dll")
	dwmapiDLL                         = windows.NewLazySystemDLL("dwmapi.dll")
	procFindWindowW                   = user32DLL.NewProc("FindWindowW")
	procSetWindowCompositionAttribute = user32DLL.NewProc("SetWindowCompositionAttribute")
	procDwmSetWindowAttribute         = dwmapiDLL.NewProc("DwmSetWindowAttribute")
	procSetClassLongPtrW              = user32DLL.NewProc("SetClassLongPtrW")
	procDwmExtendFrameIntoClientArea  = dwmapiDLL.NewProc("DwmExtendFrameIntoClientArea")
	procInvalidateRect                = user32DLL.NewProc("InvalidateRect")
	procGetWindowRect                 = user32DLL.NewProc("GetWindowRect")

	gdi32DLL            = windows.NewLazySystemDLL("gdi32.dll")
	procExtCreateRegion = gdi32DLL.NewProc("ExtCreateRegion")
	procSetWindowRgn    = user32DLL.NewProc("SetWindowRgn")
	procDeleteObject    = gdi32DLL.NewProc("DeleteObject")
)

// accentPolicy 对应 Win32 的 ACCENT_POLICY。
type accentPolicy struct {
	State         uint32
	Flags         uint32
	GradientColor uint32
	AnimationID   uint32
}

// wndCompAttrData 对应 Win32 的 WINDOWCOMPOSITIONATTRIBDATA。
type wndCompAttrData struct {
	Attribute uint32
	Data      unsafe.Pointer
	Size      uintptr
}

// dwmMargins 对应 Win32 的 MARGINS。
type dwmMargins struct {
	Left, Right, Top, Bottom int32
}

// unblurPetWindow 去掉宠物窗口背后的毛玻璃。
//
// Wails 的 WindowIsTranslucent 在 Windows 上开的是 ACCENT_ENABLE_BLURBEHIND，
// 桌面宠物窗又大都是透明像素，结果就是角色后面的桌面整片被 DWM 糊掉。
// 这里把 accent 关掉：窗口带 WS_EX_NOREDIRECTIONBITMAP，DWM 会直接合成
// WebView2 的合成层内容，透明像素原样露出桌面，无模糊也无罩色。
//
// 不同 Windows 版本行为有差异，留个环境变量方便调试切换：
// WORK_TIMER_PET_ACCENT=0/2/3/4（0=DISABLED 默认，3=BLURBEHIND 即 Wails 原版）
func unblurPetWindow() {
	title, err := windows.UTF16PtrFromString(petWindowTitle)
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return
	}
	petHwnd = hwnd // 轮廓命中测试也要用这个句柄

	state := uint32(0) /* ACCENT_DISABLED */
	if v := os.Getenv("WORK_TIMER_PET_ACCENT"); v != "" {
		if n, perr := strconv.ParseUint(v, 10, 32); perr == nil {
			state = uint32(n)
		}
	}

	ap := accentPolicy{State: state}
	data := wndCompAttrData{
		Attribute: 0x13, /* WCA_ACCENT_POLICY */
		Data:      unsafe.Pointer(&ap),
		Size:      unsafe.Sizeof(ap),
	}
	procSetWindowCompositionAttribute.Call(hwnd, uintptr(unsafe.Pointer(&data)))

	// Windows 11 上 WindowIsTranslucent 还会设系统背景（Mica/亚克力），一并关掉。
	none := uint32(1) /* DWMSBT_NONE */
	procDwmSetWindowAttribute.Call(hwnd, 38 /* DWMWA_SYSTEMBACKDROP_TYPE */, uintptr(unsafe.Pointer(&none)), unsafe.Sizeof(none))

	// Wails 还给窗口类设了 BackgroundColour 的纯黑背景刷（alpha 被无视），
	// accent 关掉后它就露出来成一片黑。把背景刷清掉，重画一次。
	// 这是默认方案；WORK_TIMER_PET_FIX=glass 可换"玻璃延伸"方案对比。
	fix := os.Getenv("WORK_TIMER_PET_FIX")
	if fix == "glass" || fix == "glassbrush" {
		// 备选方案：整窗"玻璃延伸"（sheet of glass），黑色区域会被 DWM 当成玻璃
		m := dwmMargins{Left: -1, Right: -1, Top: -1, Bottom: -1}
		procDwmExtendFrameIntoClientArea.Call(hwnd, uintptr(unsafe.Pointer(&m)))
	}
	if fix != "glass" && fix != "none" { // 默认与 glassbrush：清背景刷
		procSetClassLongPtrW.Call(hwnd, ^uintptr(9), 0) // GCLP_HBRBACKGROUND = -10
		procInvalidateRect.Call(hwnd, 0, 1)
	}
}

// petHwnd 是宠物主窗口句柄，unblurPetWindow 找到后存下来复用。
var petHwnd uintptr

// buildPetRegion 把蒙版网格拼成窗口区域（HRGN）。
//
// 每物理像素行找一段段连续的实体格，攒成 RECT 数组后一次
// ExtCreateRegion 建出整个区域——逐个 CreateRectRgn+CombineRgn 再并，
// 一次蒙版更新要几百上千次 GDI 调用，动画时每秒 8 次根本吃不消。
// 区域之外系统层面没有这扇窗，点击直接落到下面的桌面，
// 角色手臂和身体之间的镂空天然就是洞。
// 调用方需持有 petMaskMu。
func buildPetRegion() uintptr {
	physW, physH := petWindowSize()
	if petMask.cols <= 0 || petMask.rows <= 0 || len(petMask.bits) != petMask.cols*petMask.rows ||
		physW <= 0 || physH <= 0 {
		return 0
	}
	cell := func(px, py int) byte {
		cx := px * petMask.cols / physW
		cy := py * petMask.rows / physH
		return petMask.bits[cy*petMask.cols+cx]
	}

	// 先按行收集连续段：[x0, x1) × 单像素高
	type span struct{ x0, x1, y int }
	spans := make([]span, 0, physH*2)
	minX, minY, maxX, maxY := physW, physH, 0, 0
	for py := 0; py < physH; py++ {
		run := -1
		for px := 0; px <= physW; px++ {
			op := px < physW && cell(px, py) == '1'
			if op && run < 0 {
				run = px
			} else if !op && run >= 0 {
				spans = append(spans, span{run, px, py})
				if run < minX {
					minX = run
				}
				if px > maxX {
					maxX = px
				}
				if py < minY {
					minY = py
				}
				run = -1
			}
		}
	}
	if len(spans) == 0 {
		return 0
	}
	maxY = spans[len(spans)-1].y + 1

	// RGNDATA = RGNDATAHEADER(32 字节) + RECT 数组，小端手工编码
	const headerSize = 32
	buf := make([]byte, headerSize+16*len(spans))
	le := func(off int, v uint32) {
		buf[off] = byte(v)
		buf[off+1] = byte(v >> 8)
		buf[off+2] = byte(v >> 16)
		buf[off+3] = byte(v >> 24)
	}
	le(0, headerSize)             // dwSize
	le(4, 1)                      // iType = RDH_RECTANGLE
	le(8, uint32(len(spans)))     // nCount
	le(12, uint32(16*len(spans))) // nRgnSize
	le16 := func(off int, v int32) {
		buf[off] = byte(v)
		buf[off+1] = byte(v >> 8)
		buf[off+2] = byte(v >> 16)
		buf[off+3] = byte(v >> 24)
	}
	le16(16, int32(minX)) // rcBound
	le16(20, int32(minY))
	le16(24, int32(maxX))
	le16(28, int32(maxY))
	for i, s := range spans {
		off := headerSize + 16*i
		le16(off+0, int32(s.x0))
		le16(off+4, int32(s.y))
		le16(off+8, int32(s.x1))
		le16(off+12, int32(s.y+1))
	}

	rgn, _, _ := procExtCreateRegion.Call(0, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return rgn
}

// 上次真正套到窗口上的区域，用来跳过重复的 SetWindowRgn。
var (
	petLastRgnBits string
	petLastRgnW    int
	petLastRgnH    int
)

// applyPetMask 把当前蒙版套到窗口上；没有有效蒙版就还原整窗矩形。
//
// 蒙版没变就不动窗口：前端每 2 秒会重发一份一模一样的心跳蒙版，
// 白跑一次 SetWindowRgn 会闪。调用方需持有 petMaskMu。
// SetWindowRgn 成功后区域归系统所有，所以每次都新建。
func applyPetMask() {
	if petHwnd == 0 {
		return
	}
	physW, physH := petWindowSize()
	if physW == petLastRgnW && physH == petLastRgnH && petMask.bits == petLastRgnBits {
		return
	}
	rgn := buildPetRegion()
	if rgn == 0 {
		procSetWindowRgn.Call(petHwnd, 0, 1)
		petLastRgnBits, petLastRgnW, petLastRgnH = "", physW, physH
		return
	}
	// bRedraw=0：区域跟着视频帧连续变，重画会闪
	if ret, _, _ := procSetWindowRgn.Call(petHwnd, rgn, 0); ret != 0 {
		petLastRgnBits, petLastRgnW, petLastRgnH = petMask.bits, physW, physH
	} else {
		procDeleteObject.Call(rgn) // 设置失败时系统没接管，得自己回收
	}
}

// petWindowSize 返回宠物窗口当前的物理尺寸。
func petWindowSize() (int, int) {
	if petHwnd == 0 {
		return petWidth, petHeight
	}
	var rc windows.Rect
	if ret, _, _ := procGetWindowRect.Call(petHwnd, uintptr(unsafe.Pointer(&rc))); ret == 0 {
		return petWidth, petHeight
	}
	return int(rc.Right - rc.Left), int(rc.Bottom - rc.Top)
}

// setPetRegionFull 展开或还原窗口区域（右键菜单展开期间用）。
//
// 调用方需持有 petMaskMu。
func setPetRegionFull(full bool) {
	if petHwnd == 0 {
		return
	}
	if full {
		procSetWindowRgn.Call(petHwnd, 0, 0)
		petLastRgnBits = "" // 展开期间区域不是蒙版，缓存作废
	} else {
		applyPetMask()
	}
}

// 窗口外点击看护用的调用。
var (
	procGetCursorPos     = user32DLL.NewProc("GetCursorPos")
	procGetAsyncKeyState = user32DLL.NewProc("GetAsyncKeyState")
)

// winPoint 对应 Win32 的 POINT。
type winPoint struct{ X, Y int32 }

// waitOutsideClick 轮询等待"鼠标键在窗口外被按下"。
//
// 右键菜单展开期间，用户点到窗口外（真正的桌面或别的程序）时 WebView
// 收不到任何事件，只能在这层发现，再通知前端收起菜单。
// 点在窗口内则不管，前端自己的 click 处理器会收。
// 返回 true 表示等到了（该收菜单），false 表示被取消。
func waitOutsideClick(cancel <-chan struct{}) bool {
	const (
		vkLButton = 0x01
		vkRButton = 0x02
	)
	var pt winPoint
	for {
		select {
		case <-cancel:
			return false
		case <-time.After(60 * time.Millisecond):
		}
		if petHwnd == 0 {
			continue
		}
		var rc windows.Rect
		if ret, _, _ := procGetWindowRect.Call(petHwnd, uintptr(unsafe.Pointer(&rc))); ret == 0 {
			continue
		}
		if ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); ret == 0 {
			continue
		}
		if pt.X >= rc.Left && pt.X < rc.Right && pt.Y >= rc.Top && pt.Y < rc.Bottom {
			continue // 点在窗口里，交给前端
		}
		l, _, _ := procGetAsyncKeyState.Call(vkLButton)
		r, _, _ := procGetAsyncKeyState.Call(vkRButton)
		if l&0x8000 != 0 || r&0x8000 != 0 {
			return true
		}
	}
}
