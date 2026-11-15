package main

import (
	_ "embed"
	"runtime"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// 图标来自项目原有应用图标（含 32/48/256 三帧），副本已内置在 build/ 下。
// Windows 托盘直接用 ICO 的多尺寸帧；Linux/macOS 用从同一个 ICO 里
// 提取出的 256×256 PNG 帧——Wails 的 Linux 托盘走 image.Decode，不认 ICO。
//
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

// setupTray 把「关闭窗口」改成隐藏到后台托盘，托盘菜单提供显示、
// 服务启停与退出。SystemTrayManager.New() 会在 app.Run() 时自动启动
// 托盘，不需要手动 Run。appQuitting 标记「用户确实要退出」：托盘的
// 「退出」和前端关闭弹窗里的「直接退出」都置位，WindowClosing hook
// 见到它才放行，否则一律隐藏到托盘。
var appQuitting atomic.Bool

// serviceAction 是托盘菜单里的服务动作（转交主程序 CLI，桌面端不自己
// 管理进程）。启停都在后台 goroutine 执行：stop 会等 1 秒复核，不能卡
// 托盘菜单线程。
func setupTray(app *application.App, window *application.WebviewWindow, svc *DesktopService) {
	menu := app.NewMenu()
	menu.Add("显示主界面").OnClick(func(*application.Context) {
		window.Show()
	})
	menu.AddSeparator()
	menu.Add("启动服务").OnClick(func(*application.Context) {
		go func() {
			if _, err := svc.StartServer(); err != nil {
				app.Dialog.Error().SetTitle("启动失败").SetMessage(err.Error()).Show()
			}
		}()
	})
	menu.Add("停止服务").OnClick(func(*application.Context) {
		go func() {
			if _, err := svc.StopServer(); err != nil {
				app.Dialog.Error().SetTitle("停止失败").SetMessage(err.Error()).Show()
			}
		}()
	})
	menu.Add("重载配置").OnClick(func(*application.Context) {
		go func() {
			if _, err := svc.ReloadServer(); err != nil {
				app.Dialog.Error().SetTitle("重载失败").SetMessage(err.Error()).Show()
			}
		}()
	})
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) {
		appQuitting.Store(true)
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
		if appQuitting.Load() {
			return
		}
		e.Cancel()
		window.Hide()
	})
}
