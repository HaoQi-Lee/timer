package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"timer/internal/settings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	winoptions "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
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

// petWidth / petHeight 是宠物窗口的逻辑大小。Wails 创建窗口时自己会按
// 屏幕 DPI 缩放成物理尺寸（CSS 视口恒为 420×285），千万别再手动乘缩放
// 去调 WindowSetSize——那是逻辑像素，会被二次放大，窗口区域就套不上
// 前端固定像素布局的角色了（角色会被裁掉小半个）。
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

	// 置顶与否跟着配置走（右键菜单里可以切）
	cfg, err := settings.Load()
	if err != nil {
		cfg = settings.Default()
	}

	app := NewPetApp()
	return wails.Run(&options.App{
		Title:         petWindowTitle,
		Width:         petWidth,
		Height:        petHeight,
		DisableResize: true,
		Frameless:     true,
		AlwaysOnTop:   cfg.PetOnTop(),
		// 藏着建窗口：先把"不进任务栏"的窗口样式设好再显示（见 domReady），
		// 免得任务栏按钮先冒出来再消失闪一下。
		StartHidden:      true,
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
	// 菜单看护：右键菜单展开期间盯"窗口外的鼠标按下"，盯到就收起菜单
	watchMu     sync.Mutex
	watchCancel chan struct{}
}

// NewPetApp 建好宠物后端。
func NewPetApp() *PetApp {
	core := NewApp(false)
	core.quiet = true // 下班提醒只由主进程发，宠物不弹，不然会弹两份
	return &PetApp{core: core}
}

// startup 启动计时引擎，宠物前端和主窗口一样收 tick 事件；
// 另外起一个看护循环：位置动了就存、主进程改了配置就热加载。
func (p *PetApp) startup(ctx context.Context) {
	p.ctx = ctx
	p.core.startup(ctx)
	go p.watchConfig(ctx)
}

// domReady 摆好宠物窗口的位置：记住过就用记住的，否则居中显示。
func (p *PetApp) domReady(ctx context.Context) {
	// 重新读盘而不是用启动时的内存副本：主进程可能刚写过位置。
	cfg, err := settings.Load()
	if err != nil {
		cfg = settings.Default()
	}
	if !placeWindow(ctx, petWindowTitle, cfg.PetX, cfg.PetY) {
		runtime.WindowCenter(ctx)
	}
	runtime.WindowSetAlwaysOnTop(ctx, cfg.PetOnTop())

	// 窗口建好后把 Wails 默认的毛玻璃背景换成纯透明，别糊到桌面。
	// 轮廓区域不用在这边套：前端视频一起来就开始采样上报（SetHitMask）。
	unblurPetWindow()

	// 趁窗口还藏着摘掉任务栏按钮，样式落定后再显示出来。
	hidePetFromTaskbar()
	runtime.WindowShow(ctx)
}

// beforeClose 在宠物窗口关闭前记住位置。
func (p *PetApp) beforeClose(ctx context.Context) bool {
	p.savePosition(ctx)
	return false
}

// savePosition 把宠物当前位置写进配置（合并写，只动位置字段）。
func (p *PetApp) savePosition(ctx context.Context) {
	if ctx == nil {
		return
	}
	x, y := runtime.WindowGetPosition(ctx)
	_ = updateSettings(func(cfg *settings.Settings) {
		cfg.PetX, cfg.PetY = x, y
	})
}

// watchConfig 是宠物进程的看护循环，每 3 秒干两件事：
//
//   - 位置没变就不写盘（原来是无条件心跳写，白写一整天）；
//   - 配置文件的计时字段变了（主窗口改了年薪/排班/物品）就热加载，
//     宠物气泡里的数据不用重启就能跟上主窗口。
func (p *PetApp) watchConfig(ctx context.Context) {
	path, err := settings.Path()
	if err != nil {
		return
	}

	p.core.mu.RLock()
	lastSig := timerSignature(p.core.cfg)
	p.core.mu.RUnlock()
	var lastMod time.Time
	savedX, savedY := 0, 0
	saved := false

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		// 1) 位置：动了才存。主进程退出时是直接杀掉宠物的，靠这个兜底，
		//    位置最多丢最后 3 秒。
		if x, y := runtime.WindowGetPosition(ctx); !saved || x != savedX || y != savedY {
			if updateSettings(func(cfg *settings.Settings) {
				cfg.PetX, cfg.PetY = x, y
			}) == nil {
				savedX, savedY, saved = x, y, true
			}
		}

		// 2) 配置：文件动过且计时字段真的变了才重算（位置字段变了不用管）。
		fi, err := os.Stat(path)
		if err != nil || fi.ModTime().Equal(lastMod) {
			continue
		}
		lastMod = fi.ModTime()
		cfg, err := settings.Load()
		if err != nil {
			continue
		}
		if sig := timerSignature(cfg); sig != lastSig {
			lastSig = sig
			p.core.applyConfig(cfg)
		}
	}
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
	return updateSettings(func(cfg *settings.Settings) {
		cfg.PetPack = pack
	})
}

// PetAlwaysOnTop 返回宠物窗口是否置顶。
func (p *PetApp) PetAlwaysOnTop() bool {
	cfg, err := settings.Load()
	if err != nil {
		return true
	}
	return cfg.PetOnTop()
}

// SetPetAlwaysOnTop 切换宠物置顶并记住（合并写，不动别的字段）。
func (p *PetApp) SetPetAlwaysOnTop(on bool) error {
	if err := updateSettings(func(cfg *settings.Settings) {
		v := on
		cfg.PetAlwaysOnTop = &v
	}); err != nil {
		return err
	}
	if p.ctx != nil {
		runtime.WindowSetAlwaysOnTop(p.ctx, on)
	}
	return nil
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
// 展开的同时开始盯窗口外的点击——菜单展开着的时候那块透明窗口
// 会吃掉点击，而点到窗口外时 WebView 收不到事件，只能在这层收。
func (p *PetApp) SetRegionFull(full bool) {
	petMaskMu.Lock()
	petMaskHold = full
	setPetRegionFull(full)
	petMaskMu.Unlock()

	if full {
		p.startOutsideWatch()
	} else {
		p.stopOutsideWatch()
	}
}

// startOutsideWatch 盯住"窗口外的鼠标按下"，盯到就让前端收起菜单。
func (p *PetApp) startOutsideWatch() {
	p.watchMu.Lock()
	defer p.watchMu.Unlock()
	if p.watchCancel != nil {
		return // 已经在盯了
	}
	cancel := make(chan struct{})
	p.watchCancel = cancel
	go func() {
		fired := waitOutsideClick(cancel)

		p.watchMu.Lock()
		if p.watchCancel == cancel {
			p.watchCancel = nil
		}
		p.watchMu.Unlock()

		if fired && p.ctx != nil {
			runtime.EventsEmit(p.ctx, "pet-hide-menu")
		}
	}()
}

// stopOutsideWatch 停掉窗口外点击看护（菜单收起时调用）。
func (p *PetApp) stopOutsideWatch() {
	p.watchMu.Lock()
	defer p.watchMu.Unlock()
	if p.watchCancel != nil {
		close(p.watchCancel)
		p.watchCancel = nil
	}
}

// ClosePet 收起宠物：宠物自己退出，主进程会探测到并同步开关状态。
func (p *PetApp) ClosePet() {
	if p.ctx == nil {
		return
	}
	// Quit 之前先把位置落盘：runtime.Quit 不一定回头走 beforeClose。
	p.savePosition(p.ctx)
	runtime.Quit(p.ctx)
}
