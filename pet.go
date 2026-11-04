package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"timer/internal/settings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	winoptions "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

// petWindowTitle 是宠物窗口的标题，找窗口句柄时也用它。
const petWindowTitle = "桌宠大肥鱼"

// ---------- 素材与窗口尺寸 ----------
//
// 动画素材来自 dsh-pet：640×360 的 VP9 透明视频，角色固定活动在
// 命中框 (200,50)-(440,335) 里。窗口就开成命中框大小（240×285），
// 视频由 CSS 裁着只露这一块（见 pet.css）。轮廓区域的蒙版不再来自
// 静态图，而是前端定时从视频当前帧采样（pet.js 的 sampleMask）报上来，
// 窗口边缘始终跟着角色像素走。

// petWidth / petHeight 是宠物窗口的初始物理大小（100% 缩放下），
// 实际尺寸在 domReady 里按屏幕 DPI 修正。
// 右侧 240×285 是角色（命中框），左侧 180px 留给台词气泡——
// 气泡别压在角色身上，显示期间那块会并进轮廓蒙版（见 pet.js）。
const (
	petWidth  = 420
	petHeight = 285
)

// petHitMask 是前端采样来的轮廓蒙版：cols×rows 的网格，每格一个字符。
type petHitMask struct {
	cols, rows int
	bits       string
}

var (
	petMaskMu   sync.Mutex
	petMask     petHitMask // 有效性看 cols>0
	petMaskHold bool       // 右键菜单展开期间暂停贴轮廓（区域临时还原整窗）
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

// runPet 用 -pet 参数启动时走这里：开一个无边框、透明、置顶的宠物窗口。
//
// Wails v2 没有多窗口 API，桌面宠物靠同一个 exe 再拉一个进程实现，
// 两个进程之间只通过 settings.json 和进程生命周期打交道。
func runPet() error {
	// 部分 GPU 驱动的 VP9 硬件解码会丢掉透明通道：alpha 视频渲染成灰底，
	// 桌宠素材全是 VP9-alpha，这里强制视频走软解保住透明。
	// Wails v2 没有 AdditionalBrowserArgs 直通项（那是 v3 的），
	// 用 WebView2 加载器的官方环境变量注入，只影响本进程。
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--disable-accelerated-video-decode")

	app := NewPetApp()
	return wails.Run(&options.App{
		Title:            "桌宠大肥鱼",
		Width:            petWidth,
		Height:           petHeight,
		DisableResize:    true,
		Frameless:        true,
		AlwaysOnTop:      true,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer:      &assetserver.Options{Assets: petAssets{base: assets}},
		Bind:             []interface{}{app},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		Windows: &winoptions.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			// 透明窗口要是再带系统阴影和圆角，四角会露出一圈不透明的黑边。
			DisableFramelessWindowDecorations: true,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHidden(),
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
		},
	})
}

// petAssets 把主页面换成宠物页面。
//
// Wails 固定从 index.html 开始加载，这里把对 index.html（含根路径）的请求
// 改道到 pet.html，其余静态资源照常从 dist 目录取。
type petAssets struct {
	base embed.FS
}

// Open 实现 fs.FS，把首页请求映射到 pet.html。
//
// 注意：对 "."（根目录）必须原样放行让 embed 返回目录，
// Wails 的 assetserver 对 "/" 请求会先打开目录、再自己拼上 index.html
// 打开一次，两步都得成立页面才出得来。
func (a petAssets) Open(name string) (fs.File, error) {
	name = strings.TrimPrefix(strings.TrimPrefix(name, "/"), "./")
	if name == "index.html" {
		name = "pet.html"
	}
	return a.base.Open(path.Join("frontend/dist", name))
}

// PetApp 是宠物窗口绑定给前端的门面。
//
// 计时引擎直接复用 App（同一个进程里 NewApp 一份），
// 所以宠物自己也能算薪资快照，气泡里可以播报"已赚多少、还有多久下班"。
type PetApp struct {
	ctx  context.Context
	core *App
}

// NewPetApp 建好宠物后端。
func NewPetApp() *PetApp {
	return &PetApp{core: NewApp(false)}
}

// startup 启动计时引擎，宠物前端和主窗口一样收 tick 事件。
func (p *PetApp) startup(ctx context.Context) {
	p.ctx = ctx
	p.core.startup(ctx)
}

// domReady 摆好宠物窗口的位置：记住过就用记住的，否则居中显示。
func (p *PetApp) domReady(ctx context.Context) {
	// 先把窗口收成贴着角色的尺寸，再摆位置，免得位置按旧尺寸算。
	p.hugPetWindow(ctx)

	// 重新读盘而不是用启动时的内存副本：主进程可能刚写过位置。
	cfg, err := settings.Load()
	if err != nil {
		cfg = settings.Default()
	}
	if !p.restorePosition(ctx, cfg.PetX, cfg.PetY) {
		runtime.WindowCenter(ctx)
	}

	// 窗口建好后把 Wails 默认的毛玻璃背景换成纯透明，别糊到桌面。
	// 轮廓区域不用在这边套：前端视频一起来就开始采样上报（SetHitMask）。
	unblurPetWindow()
}

// hugPetWindow 把窗口收成素材本体的大小（命中框矩形、零留白）。
//
// Wails 的窗口用物理像素、素材是逻辑像素，得乘上屏幕缩放，
// 否则高 DPI 下窗口放不下原尺寸素材、角色会被压小。
func (p *PetApp) hugPetWindow(ctx context.Context) {
	scale := 1.0
	if screens, err := runtime.ScreenGetAll(ctx); err == nil {
		for _, s := range screens {
			if s.IsPrimary && s.Size.Width > 0 {
				scale = float64(s.PhysicalSize.Width) / float64(s.Size.Width)
				break
			}
		}
	}
	runtime.WindowSetSize(ctx,
		int(float64(petWidth)*scale+0.5),
		int(float64(petHeight)*scale+0.5))
}

// beforeClose 在宠物窗口关闭前记住位置。
func (p *PetApp) beforeClose(ctx context.Context) bool {
	p.savePosition(ctx)
	return false
}

// restorePosition 判断记住的位置还能不能用，跑出屏幕就返回 false。
func (p *PetApp) restorePosition(ctx context.Context, x, y int) bool {
	if x == 0 && y == 0 {
		return false
	}
	width, height := primaryScreenSize(ctx)
	if x < -width || x >= width || y < 0 || y >= height {
		return false
	}
	runtime.WindowSetPosition(ctx, x, y)
	return true
}

// primaryScreenSize 返回主屏大小，查不到时按 1080p 兜底。
func primaryScreenSize(ctx context.Context) (int, int) {
	if screens, err := runtime.ScreenGetAll(ctx); err == nil {
		for _, s := range screens {
			if s.IsPrimary {
				return s.Size.Width, s.Size.Height
			}
		}
	}
	return 1920, 1080
}

// savePosition 把宠物当前位置写进配置。
func (p *PetApp) savePosition(ctx context.Context) {
	if ctx == nil {
		return
	}
	x, y := runtime.WindowGetPosition(ctx)
	_ = savePetConfig(func(cfg *settings.Settings) {
		cfg.PetX, cfg.PetY = x, y
	})
}

// savePetConfig 重新读盘、只改桌宠字段后写回。
//
// 宠物和主程序是两个进程、共用一个配置文件，
// 拿内存里的整份配置直接覆盖会把主进程刚写的字段冲掉。
func savePetConfig(mutate func(cfg *settings.Settings)) error {
	cfg, err := settings.Load()
	if err != nil {
		cfg = settings.Default()
	}
	mutate(&cfg)
	return settings.Save(cfg)
}

// Snapshot 返回当前完整状态，宠物前端启动时主动拉一次。
func (p *PetApp) Snapshot() Snapshot {
	return p.core.Snapshot()
}

// GetPack 返回当前表情包。
func (p *PetApp) GetPack() string {
	cfg, err := settings.Load()
	if err != nil {
		return settings.PetPackClassic
	}
	return cfg.PetPack
}

// SetPack 切换表情包并记住。
func (p *PetApp) SetPack(pack string) error {
	if !settings.ValidPetPack(pack) {
		return fmt.Errorf("未知表情包: %q", pack)
	}
	return savePetConfig(func(cfg *settings.Settings) {
		cfg.PetPack = pack
	})
}

// SetHitMask 收前端采样来的轮廓蒙版（cols×rows 网格，每格 '0'/'1'），
// 套成窗口区域——桌宠的边缘始终跟着视频里的角色像素走。
func (p *PetApp) SetHitMask(cols, rows int, bits string) {
	petMaskMu.Lock()
	defer petMaskMu.Unlock()
	petMask = petHitMask{cols: cols, rows: rows, bits: bits}
	if !petMaskHold {
		applyPetMask()
	}
}

// SetRegionFull 展开或还原窗口区域。
//
// 右键菜单可能伸到角色轮廓外面，轮廓区域会把菜单裁掉、点不到，
// 菜单打开期间临时展开成整窗并暂停贴轮廓（SetHitMask 只记不套），
// 关掉再用最新一份蒙版贴回去。
func (p *PetApp) SetRegionFull(full bool) {
	petMaskMu.Lock()
	defer petMaskMu.Unlock()
	if petHwnd == 0 {
		return
	}
	petMaskHold = full
	if full {
		procSetWindowRgn.Call(petHwnd, 0, 0)
	} else {
		applyPetMask()
	}
}

// SavePosition 记住宠物当前位置。
//
// 前端每隔几秒调一次当心跳：主程序退出时是直接杀掉宠物进程的，
// 靠心跳兜底，位置最多丢最后几秒。
func (p *PetApp) SavePosition() {
	p.savePosition(p.ctx)
}

// ClosePet 收起宠物：宠物自己退出，主进程会探测到并同步开关状态。
func (p *PetApp) ClosePet() {
	if p.ctx != nil {
		runtime.Quit(p.ctx)
	}
}
