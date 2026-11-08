//go:build windows

package svcctl

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
)

func sendCtrlEvent(event uintptr, processGroupId int) error {
	ret, _, err := procGenerateConsoleCtrlEvent.Call(event, uintptr(processGroupId))
	if ret == 0 {
		return err
	}
	return nil
}

// SignalReload 先试 CTRL_BREAK（独立控制台组会忽略 CTRL_C），再试 CTRL_C，最后 SIGINT。
func SignalReload(pid int) error {
	if pid <= 0 {
		return os.ErrInvalid
	}
	if err := sendCtrlEvent(windows.CTRL_BREAK_EVENT, pid); err == nil {
		return nil
	}
	if err := sendCtrlEvent(windows.CTRL_C_EVENT, pid); err == nil {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(os.Interrupt)
}
