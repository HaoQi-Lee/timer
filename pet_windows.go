//go:build windows

package main

import (
	"os"
	"strconv"
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

	gdi32DLL          = windows.NewLazySystemDLL("gdi32.dll")
	procCreateRectRgn = gdi32DLL.NewProc("CreateRectRgn")
	procCombineRgn    = gdi32DLL.NewProc("CombineRgn")
	procSetWindowRgn  = user32DLL.NewProc("SetWindowRgn")
	procDeleteObject  = gdi32DLL.NewProc("DeleteObject")
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
// 每物理像素行找一段段连续的实体格，用小矩形一个个并起来；
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
	acc, _, _ := procCreateRectRgn.Call(0, 0, 0, 0)
	for py := 0; py < physH; py++ {
		run := -1
		for px := 0; px <= physW; px++ {
			op := px < physW && cell(px, py) == '1'
			if op && run < 0 {
				run = px
			} else if !op && run >= 0 {
				r, _, _ := procCreateRectRgn.Call(uintptr(run), uintptr(py), uintptr(px), uintptr(py+1))
				procCombineRgn.Call(acc, acc, r, 2 /* RGN_OR */)
				procDeleteObject.Call(r)
				run = -1
			}
		}
	}
	return acc
}

// applyPetMask 把当前蒙版套到窗口上；没有有效蒙版就还原整窗矩形。
//
// 调用方需持有 petMaskMu。SetWindowRgn 成功后区域归系统所有，所以每次都新建。
func applyPetMask() {
	if petHwnd == 0 {
		return
	}
	rgn := buildPetRegion()
	if rgn == 0 {
		procSetWindowRgn.Call(petHwnd, 0, 1)
		return
	}
	// bRedraw=0：区域跟着视频帧连续变，重画会闪
	if ret, _, _ := procSetWindowRgn.Call(petHwnd, rgn, 0); ret == 0 {
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
	} else {
		applyPetMask()
	}
}
