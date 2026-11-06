package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// 前端产物由 vite 构建到 frontend/dist，编译期嵌进单文件可执行程序。
//go:embed all:frontend/dist
var assets embed.FS

func main() {
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
	})
	setMainWindow(window)

	setupTray(app, window)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
