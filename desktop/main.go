package main

import (
	"embed"
	"log"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// 前端产物由 vite 构建到 frontend/dist，编译期嵌进单文件可执行程序。
//
//go:embed all:frontend/dist
var assets embed.FS

// appRef 供 QuitApp 使用：托盘常驻时关闭窗口只隐藏，真正退出必须显式 Quit。
var appRef *application.App

func main() {
	// --tray：开机自启的托盘常驻模式——启动后立即隐藏主窗口，
	// 服务交给托盘菜单管理，不占用户桌面。
	startInTray := false
	for _, a := range os.Args[1:] {
		if strings.EqualFold(a, "--tray") {
			startInTray = true
		}
	}

	app := application.New(application.Options{
		Name:        "Fast-Https Desktop",
		Description: "Fast-Https 本机管理端：状态、站点、证书与配置重载",
		Services: []application.Service{
			application.NewService(NewDesktopService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// 有托盘常驻，关窗不等于退出程序。
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	// Frameless：去掉系统标题栏（图标、标题文字与系统窗口按钮），
	// 由前端 .topbar 自绘标题栏——可拖拽，右侧三个按钮走窗口 API。
	// DisableFramelessWindowDecorations 保持默认 false，保留 Win11 圆角与阴影。
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Fast-Https · 本机管理端",
		Width:            1280,
		Height:           800,
		Frameless:        true,
		BackgroundColour: application.NewRGB(255, 255, 255),
		URL:              "/",
		// 桌面端不是浏览器：右键菜单里的「返回 / 刷新 / 另存为 / 打印 / 检查」
		// 对使用者没有意义，直接交给 WebView 关闭。devtools 在 production
		// 构建里本来就默认关闭，这里显式关掉，避免开发版误带着入口。
		DefaultContextMenuDisabled: true,
		DevToolsEnabled:            false,
		Hidden:                     startInTray,
	})
	setMainWindow(window)
	appRef = app

	setupTray(app, window, NewDesktopService())

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
