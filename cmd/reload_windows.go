//go:build windows

package cmd

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
)

// GenerateConsoleCtrlEvent sends a specified signal to a console process group.
// sendCtrlC 向指定进程组发送 Ctrl+C 信号
//
// 参数:
//
//	processGroupId: int - 进程组ID
//
// 返回值:
//
//	error: 如果发送信号失败，则返回错误信息；否则返回 nil
func sendCtrlEvent(event uintptr, processGroupId int) error {
	ret, _, err := procGenerateConsoleCtrlEvent.Call(event, uintptr(processGroupId))
	if ret == 0 {
		return err
	}
	return nil
}

// signalReloadProcess delivers SIGINT. The server treats SIGINT as reload on every platform.
// A process started as its own console group ignores CTRL_C, so try CTRL_BREAK first.
func signalReloadProcess(process *os.Process, pid int) error {
	if err := sendCtrlEvent(windows.CTRL_BREAK_EVENT, pid); err == nil {
		return nil
	}
	if err := sendCtrlEvent(windows.CTRL_C_EVENT, pid); err == nil {
		return nil
	}
	return process.Signal(os.Interrupt)
}
