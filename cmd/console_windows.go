//go:build windows

package cmd

import (
	"os"
	"syscall"
	"unsafe"
)

// 隐藏双击启动时的控制台窗口，保留「双击 fast-https.exe 不弹窗」的体验。
//
// Windows 主程序是控制台子系统，cmd/PowerShell 才会等待进程退出、保证输出
// 和提示符顺序正常。双击启动时系统会新建一个控制台，此时控制台进程列表里
// 只有自己；从终端启动时列表里还有父 shell，窗口必须保留。
func init() {
	if len(os.Args) > 1 {
		// 子命令和参数都可能有输出，交给终端正常显示。
		return
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	getConsoleProcessList := kernel32.NewProc("GetConsoleProcessList")
	user32 := syscall.NewLazyDLL("user32.dll")
	showWindow := user32.NewProc("ShowWindow")
	if getConsoleWindow.Find() != nil || getConsoleProcessList.Find() != nil || showWindow.Find() != nil {
		return
	}

	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd == 0 {
		// 服务管理器或桌面端以 CREATE_NO_WINDOW 启动时没有控制台。
		return
	}

	var pids [2]uint32
	count, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if count != 1 {
		return
	}
	const swHide = 0
	showWindow.Call(hwnd, swHide)
}
