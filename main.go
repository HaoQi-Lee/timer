package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"timer/internal/settings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 桌面宠物是同一个 exe 的第二个进程，跑在一扇独立的透明置顶窗口里。
	if hasFlag("-pet", "--pet") {
		if err := runPet(); err != nil {
			logFatal("启动桌宠失败", err)
		}
		return
	}

	openSettings := hasFlag("-s", "--settings")
	app := NewApp(openSettings)

	if err := wails.Run(&options.App{
		Title:            mainWindowTitle,
		Width:            340,
		Height:           500,
		MinWidth:         340,
		MinHeight:        500,
		DisableResize:    true,
		Frameless:        true,
		AlwaysOnTop:      app.AlwaysOnTop(),
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer:      &assetserver.Options{Assets: assets},
		Menu:             buildMenu(app),
		Bind:             []interface{}{app},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		// 单实例：再开一个只会把已经跑着的窗口唤起来，不会出现双宠物、配置互踩。
		// 宠物子进程（-pet）不走这个锁，不受影响。
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "work-timer-" + mainWindowTitle,
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			Theme:                windowsTheme(app.Theme()),
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHidden(),
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			Appearance:           macAppearance(app.Theme()),
		},
	}); err != nil {
		logFatal("启动失败", err)
	}
}

// logFatal 记录致命错误。
//
// 程序编译成图形子系统后没有控制台可输出，错误只能落到日志文件里。
// 追加写：截断覆盖的话日志里永远只剩最后一条错误。
func logFatal(msg string, err error) {
	line := fmt.Sprintf("%s %s: %v\n", time.Now().Format("2006-01-02 15:04:05"), msg, err)

	path, perr := settings.Path()
	if perr == nil {
		dir := filepath.Dir(path)
		if merr := os.MkdirAll(dir, 0o755); merr == nil {
			if f, oerr := os.OpenFile(filepath.Join(dir, "error.log"),
				os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); oerr == nil {
				_, _ = f.WriteString(line)
				_ = f.Close()
				return
			}
		}
	}
	fmt.Print(line)
}

// hasFlag 判断命令行里有没有某个参数。
func hasFlag(names ...string) bool {
	for _, arg := range os.Args[1:] {
		for _, name := range names {
			if arg == name {
				return true
			}
		}
	}
	return false
}

// windowsTheme 把界面主题换算成 Windows 的窗口主题。
func windowsTheme(theme string) windows.Theme {
	if theme == settings.ThemeLight {
		return windows.Light
	}
	return windows.Dark
}

// macAppearance 把界面主题换算成 macOS 的系统外观。
func macAppearance(theme string) mac.AppearanceType {
	if theme == settings.ThemeLight {
		return mac.NSAppearanceNameAqua
	}
	return mac.NSAppearanceNameDarkAqua
}

// buildMenu 生成应用菜单。macOS 上第一个子菜单会成为应用菜单，所以那里放应用名。
func buildMenu(app *App) *menu.Menu {
	m := menu.NewMenu()

	if runtime.GOOS == "darwin" {
		appMenu := m.AddSubmenu("上班计时器")
		appMenu.AddText("设置…", nil, func(_ *menu.CallbackData) { app.OpenSettings() })
		appMenu.AddSeparator()
		appMenu.AddText("退出上班计时器", nil, func(_ *menu.CallbackData) { app.Quit() })
		return m
	}

	settingMenu := m.AddSubmenu("设置")
	settingMenu.AddText("设置…", nil, func(_ *menu.CallbackData) { app.OpenSettings() })
	settingMenu.AddSeparator()
	settingMenu.AddText("退出", nil, func(_ *menu.CallbackData) { app.Quit() })
	return m
}
