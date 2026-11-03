package main

import (
	"embed"
	"os"
	"runtime"

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
	app := NewApp()
	openSettings := hasFlag("-s", "--settings")

	if err := wails.Run(&options.App{
		Title:            "上班计时器",
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
		OnStartup:        app.startupWith(openSettings),
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			Theme:                windows.Dark,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHidden(),
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			Appearance:           mac.NSAppearanceNameDarkAqua,
		},
	}); err != nil {
		println("启动失败:", err.Error())
	}
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
