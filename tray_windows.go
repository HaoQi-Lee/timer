//go:build windows

package main

import (
	"sync"
	"unsafe"

	"timer/internal/fishicon"

	"golang.org/x/sys/windows"
)

// 最小化进托盘：把主窗口的窗口过程子类化一层，"最小化"被改写成
// "隐藏窗口 + 右下角托盘图标"，任务栏不再占位。点图标恢复，右键出
// 菜单（显示 / 退出）。Wails v2 没有托盘支持，这里照 pet_windows.go
// 的路子直接上 Win32。

const (
	// 自定义消息。窗口状态只能在自己的窗口线程上动，别的 goroutine
	// 想藏/恢复窗口一律 PostMessage 切回来处理。
	wmTrayHide    = 0x8001 // 藏窗口进托盘
	wmTrayRestore = 0x8002 // 从托盘恢复窗口
	// 托盘图标上的鼠标事件由系统以这条消息送回窗口过程（legacy 版协议：
	// wParam = 图标 id，lParam = 鼠标消息号）。
	wmTrayCallback = 0x8004
)

const (
	wmNull        = 0x0000
	wmSize        = 0x0005
	sizeMinimized = 1
	wmSysCommand  = 0x0112
	scMinimize    = 0xF020
	wmGetIcon     = 0x007F

	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205

	swHide     = 0
	swMinimize = 6
	swRestore  = 9

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04
	nifInfo    = 0x10
	niifInfo   = 0x01

	mfString    = 0x0000
	mfSeparator = 0x0800

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	smCxSmIcon = 49
	smCySmIcon = 50
)

var (
	procShowWindow             = user32DLL.NewProc("ShowWindow")
	procPostMessageW           = user32DLL.NewProc("PostMessageW")
	procSendMessageW           = user32DLL.NewProc("SendMessageW")
	procSetForegroundWindow    = user32DLL.NewProc("SetForegroundWindow")
	procCreatePopupMenu        = user32DLL.NewProc("CreatePopupMenu")
	procAppendMenuW            = user32DLL.NewProc("AppendMenuW")
	procTrackPopupMenu         = user32DLL.NewProc("TrackPopupMenu")
	procDestroyMenu            = user32DLL.NewProc("DestroyMenu")
	procGetClassLongPtrW       = user32DLL.NewProc("GetClassLongPtrW")
	procSetWindowLongPtrW      = user32DLL.NewProc("SetWindowLongPtrW")
	procCallWindowProcW        = user32DLL.NewProc("CallWindowProcW")
	procRegisterWindowMessageW = user32DLL.NewProc("RegisterWindowMessageW")
	procLoadIconW              = user32DLL.NewProc("LoadIconW")

	shell32DLL           = windows.NewLazySystemDLL("shell32.dll")
	procShellNotifyIconW = shell32DLL.NewProc("Shell_NotifyIconW")

	// 像素大肥鱼图标用的 GDI 调用
	procGetSystemMetrics   = user32DLL.NewProc("GetSystemMetrics")
	procCreateIconIndirect = user32DLL.NewProc("CreateIconIndirect")
	procCreateDIBSection   = gdi32DLL.NewProc("CreateDIBSection")
	procCreateBitmap       = gdi32DLL.NewProc("CreateBitmap")
)

// 托盘状态。窗口过程、beforeClose 都会碰，加把小锁保险。
var (
	trayMu        sync.Mutex
	trayHwnd      uintptr // 主窗口句柄，setupTray 找到后不再变
	trayOrigProc  uintptr // 子类化前的原窗口过程
	trayQuit      func()  // 托盘菜单"退出"
	trayIconAdded bool    // 图标当前挂在托盘里（窗口隐藏中）
	trayTipShown  bool    // 气泡提醒只弹一次
	trayProcCb    = windows.NewCallback(trayWndProc)

	// 资源管理器重启后托盘图标会全体消失，收到这条广播要补挂
	msgTaskbarCreated uintptr
)

// setupTray 找到主窗口，挂上托盘钩子。窗口还没建好（domReady 之后不会）
// 或子类化失败就不启用，Minimize 退回普通最小化。
func setupTray(onQuit func()) {
	name, err := windows.UTF16PtrFromString(mainWindowTitle)
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(name)))
	if hwnd == 0 {
		return
	}
	if n, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(taskbarCreatedName))); n != 0 {
		msgTaskbarCreated = n
	}
	trayQuit = onQuit
	// GWLP_WNDPROC = -4：把窗口过程换成自己的，其余消息原样转回 Wails。
	orig, _, _ := procSetWindowLongPtrW.Call(hwnd, ^uintptr(3), trayProcCb)
	if orig == 0 {
		return
	}
	trayHwnd, trayOrigProc = hwnd, orig
}

var taskbarCreatedName, _ = windows.UTF16PtrFromString("TaskbarCreated")

// trayWndProc 是套在 Wails 窗口过程外面的一层：只截和托盘有关的消息，
// 其余全部 CallWindowProc 转回原过程。
func trayWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if hwnd == trayHwnd {
		switch msg {
		case wmSysCommand:
			if wParam&0xFFF0 == scMinimize {
				// 任务栏菜单/系统快捷键的最小化：改成进托盘，不真最小化
				postTray(wmTrayHide)
				return 0
			}
		case wmSize:
			if wParam == sizeMinimized {
				// 没拦住的路径（绕过 WM_SYSCOMMAND 直接 ShowWindow 的那种）：
				// 已经最小化了，再跟着藏进托盘
				postTray(wmTrayHide)
			}
		case wmTrayHide:
			hideWindowToTray()
			return 0
		case wmTrayRestore:
			restoreWindowFromTray()
			return 0
		case wmTrayCallback:
			switch lParam {
			case wmLButtonUp, wmLButtonDblClk:
				restoreWindowFromTray()
			case wmRButtonUp:
				showTrayMenu()
			}
			return 0
		}
		if msg == msgTaskbarCreated && trayIconAdded {
			addTrayIcon(false) // 资源管理器重启，补挂图标
		}
	}
	ret, _, _ := procCallWindowProcW.Call(trayOrigProc, hwnd, msg, wParam, lParam)
	return ret
}

// minimizeToTray 把窗口收进托盘。返回 false 表示托盘没就绪（窗口还没找到），
// 调用方退回普通最小化。可以在任意 goroutine 调：先 PostMessage 切回窗口线程。
func minimizeToTray() bool {
	if trayHwnd == 0 {
		return false
	}
	postTray(wmTrayHide)
	return true
}

// restoreFromTray 把藏在托盘里的窗口捞回来（二次启动唤醒时用）。
func restoreFromTray() {
	if trayHwnd != 0 {
		postTray(wmTrayRestore)
	}
}

// removeTrayIcon 退出前摘掉托盘图标，免得进程没了图标还赖在托盘里。
func removeTrayIcon() {
	trayMu.Lock()
	defer trayMu.Unlock()
	removeTrayIconLocked()
}

// hideWindowToTray 藏窗口 + 挂托盘图标（窗口线程上执行）。
func hideWindowToTray() {
	if trayHwnd == 0 {
		return
	}
	if !addTrayIcon(true) {
		// 图标没挂上就不能藏窗口，否则用户找不到入口：退回普通最小化
		procShowWindow.Call(trayHwnd, swMinimize)
		return
	}
	procShowWindow.Call(trayHwnd, swHide) // 隐藏即从任务栏消失
}

// restoreWindowFromTray 把窗口从托盘里恢复出来（窗口线程上执行）。
func restoreWindowFromTray() {
	if trayHwnd == 0 {
		return
	}
	// SW_RESTORE 顺带处理"隐藏前已被最小化"的情况
	procShowWindow.Call(trayHwnd, swRestore)
	procSetForegroundWindow.Call(trayHwnd)
	trayMu.Lock()
	removeTrayIconLocked()
	trayMu.Unlock()
}

// showTrayMenu 弹托盘右键菜单（窗口线程上执行）。
func showTrayMenu() {
	if trayHwnd == 0 {
		return
	}
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	itemShow, _ := windows.UTF16PtrFromString("显示主窗口")
	itemQuit, _ := windows.UTF16PtrFromString("退出")
	procAppendMenuW.Call(menu, mfString, 1, uintptr(unsafe.Pointer(itemShow)))
	procAppendMenuW.Call(menu, mfSeparator, 0, 0)
	procAppendMenuW.Call(menu, mfString, 2, uintptr(unsafe.Pointer(itemQuit)))

	var pt winPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// 不先抢前台的话，点桌面别处菜单不会自己收掉（Win32 老坑）
	procSetForegroundWindow.Call(trayHwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd,
		uintptr(pt.X), uintptr(pt.Y), 0, trayHwnd, 0)
	// 配套的收尾消息，让窗口正确回到后台状态
	procPostMessageW.Call(trayHwnd, wmNull, 0, 0)

	switch cmd {
	case 1:
		restoreWindowFromTray()
	case 2:
		// 不在窗口过程里同步退出，免得清理过程和消息循环打架
		if trayQuit != nil {
			go trayQuit()
		}
	}
}

// addTrayIcon 把图标挂进托盘。withTip 控制要不要弹气泡提醒
// （告诉用户窗口去了哪，只提醒一次）。返回是否挂成功。
func addTrayIcon(withTip bool) bool {
	trayMu.Lock()
	defer trayMu.Unlock()

	nid := trayIconData(nifMessage | nifIcon | nifTip)
	nid.UCallbackMessage = wmTrayCallback
	nid.HIcon = trayIconHandle()
	copyUTF16(nid.SzTip[:], mainWindowTitle)

	if ret, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(nid))); ret == 0 {
		return false
	}
	trayIconAdded = true

	if withTip && !trayTipShown {
		trayTipShown = true
		*nid = *trayIconData(nifInfo)
		copyUTF16(nid.SzInfo[:], "窗口收到这里了，点图标恢复")
		copyUTF16(nid.SzInfoTitle[:], mainWindowTitle)
		nid.DwInfoFlags = niifInfo
		procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(nid)))
	}
	return true
}

// removeTrayIconLocked 从托盘摘掉图标。调用方需持有 trayMu。
func removeTrayIconLocked() {
	if !trayIconAdded {
		return
	}
	trayIconAdded = false
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(trayIconData(0))))
}

// trayIconData 造一份填好公共字段的 NOTIFYICONDATA。
func trayIconData(flags uint32) *notifyIconData {
	nid := &notifyIconData{
		HWnd:   trayHwnd,
		UID:    1,
		UFlags: flags,
	}
	nid.CbSize = uint32(unsafe.Sizeof(*nid))
	return nid
}

// copyUTF16 把字符串（含结尾 NUL）拷进定长 UTF16 数组，超长截断。
func copyUTF16(dst []uint16, s string) {
	u, _ := windows.UTF16FromString(s)
	copy(dst, u)
}

// trayIconHandle 给托盘找个像样的图标：优先像素大肥鱼（桌宠像素版），
// 造不出来再退回窗口图标，最后系统默认图标。
func trayIconHandle() uintptr {
	if h := fishIconHandle(); h != 0 {
		return h
	}
	for _, what := range [...]uintptr{2 /*ICON_SMALL2*/, 0 /*ICON_SMALL*/, 1 /*ICON_BIG*/} {
		if h, _, _ := procSendMessageW.Call(trayHwnd, wmGetIcon, what, 0); h != 0 {
			return h
		}
	}
	if h, _, _ := procGetClassLongPtrW.Call(trayHwnd, ^uintptr(33) /*GCLP_HICONSM=-34*/); h != 0 {
		return h
	}
	if h, _, _ := procGetClassLongPtrW.Call(trayHwnd, ^uintptr(13) /*GCLP_HICON=-14*/); h != 0 {
		return h
	}
	h, _, _ := procLoadIconW.Call(0, 32512 /*IDI_APPLICATION*/)
	return h
}

// trayFishIcon 缓存造好的像素大肥鱼 HICON（只造一次）。
var trayFishIcon uintptr

// fishIconHandle 把 internal/fishicon 的像素网格渲染成 HICON。
//
// 按系统小图标尺寸（跟着 DPI 走：100%=16、125%=20、150%=24）最近邻
// 放大，块状感不糊。GDI 造法：32bpp 顶朝下 DIB 当颜色位图（BGRA 字节
// 序），全 0 掩码位图（全不透明——带 alpha 通道的图标系统只看 alpha），
// CreateIconIndirect 合成。位图被系统拷走，当场的两张要自己回收。
// 只在窗口线程上被 addTrayIcon 调用，无锁。
func fishIconHandle() uintptr {
	if trayFishIcon != 0 {
		return trayFishIcon
	}
	w, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	h, _, _ := procGetSystemMetrics.Call(smCySmIcon)
	if w == 0 || h == 0 {
		return 0
	}
	if w <= 0 || h <= 0 {
		return 0
	}

	// 1) 颜色位图
	var bmi bitmapInfo
	bmi.BmiHeader = bitmapInfoHeader{
		BiSize:     uint32(unsafe.Sizeof(bmi.BmiHeader)),
		BiWidth:    int32(w),
		BiHeight:   -int32(h), // 负数 = 顶朝下，行序和渲染输出一致
		BiPlanes:   1,
		BiBitCount: 32,
	}
	// bits 由 CreateDIBSection 直接写进这个指针变量（DIB 位图的地址），
	// 之后用 unsafe.Slice 拿视图写像素——比 uintptr 存地址再转回去干净
	// （那种写法 go vet 会报 unsafe.Pointer 误用）。
	var bits *byte
	hbmColor, _, _ := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bmi)),
		0 /*DIB_RGB_COLORS*/, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmColor == 0 || bits == nil {
		return 0
	}
	iw, ih := int(w), int(h)
	rgba := fishicon.Render(iw, ih)
	dst := unsafe.Slice(bits, iw*ih*4)
	for i := 0; i < iw*ih; i++ {
		dst[i*4+0] = rgba[i*4+2] // B
		dst[i*4+1] = rgba[i*4+1] // G
		dst[i*4+2] = rgba[i*4+0] // R
		dst[i*4+3] = rgba[i*4+3] // A
	}

	// 2) 掩码位图：行按 32 位对齐
	mask := make([]byte, ((iw+31)/32*4)*ih)
	hbmMask, _, _ := procCreateBitmap.Call(uintptr(w), uintptr(h), 1, 1,
		uintptr(unsafe.Pointer(&mask[0])))
	if hbmMask == 0 {
		procDeleteObject.Call(hbmColor)
		return 0
	}

	// 3) 合成图标，回收两张现场位图
	ii := iconInfo{FIcon: 1, HbmMask: hbmMask, HbmColor: hbmColor}
	hicon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	procDeleteObject.Call(hbmColor)
	procDeleteObject.Call(hbmMask)
	if hicon == 0 {
		return 0
	}
	trayFishIcon = hicon
	return hicon
}

// postTray 给主窗口发自定义托盘消息。
func postTray(msg uintptr) {
	procPostMessageW.Call(trayHwnd, msg, 0, 0)
}

// notifyIconData 对应 Win32 的 NOTIFYICONDATAW（托盘图标的一揽子参数）。
type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

// bitmapInfoHeader 对应 Win32 的 BITMAPINFOHEADER。
type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

// bitmapInfo 对应 Win32 的 BITMAPINFO（头部 + 1 个颜色槽，BI_RGB 不用）。
type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
	BmiColors [4]byte
}

// iconInfo 对应 Win32 的 ICONINFO。
type iconInfo struct {
	FIcon    uint32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}
