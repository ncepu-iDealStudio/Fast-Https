package main

import (
	_ "embed"
	"runtime"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// 图标来自项目原有的 monitor/icon/starting.ico（含 32/48/256 三帧）。
// Windows 托盘直接用 ICO 的多尺寸帧；Linux/macOS 用从同一个 ICO 里
// 提取出的 256×256 PNG 帧——Wails 的 Linux 托盘走 image.Decode，不认 ICO。
//go:embed build/windows/icon.ico
var trayIconICO []byte

//go:embed build/appicon.png
var trayIconPNG []byte

func trayIcon() []byte {
	if runtime.GOOS == "windows" {
		return trayIconICO
	}
	return trayIconPNG
}

// setupTray 把「关闭窗口」改成隐藏到后台托盘，托盘菜单提供显示与退出。
// SystemTrayManager.New() 会在 app.Run() 时自动启动托盘，不需要手动 Run。
func setupTray(app *application.App, window *application.WebviewWindow) {
	var quitting atomic.Bool

	menu := app.NewMenu()
	menu.Add("显示主界面").OnClick(func(*application.Context) {
		window.Show()
	})
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) {
		quitting.Store(true)
		app.Quit()
	})

	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon())
	tray.SetTooltip("Fast-Https 本机管理端")
	tray.SetMenu(menu)
	// AttachWindow 会让左键单击自动切换窗口显隐，右键弹出上面的菜单。
	tray.AttachWindow(window)

	// 点窗口 X：取消默认关闭并隐藏，程序继续留在托盘。
	// 从托盘「退出」时 quitting 已置位，放行真正的关闭，避免退出被 hook 拦下。
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if quitting.Load() {
			return
		}
		e.Cancel()
		window.Hide()
	})
}
