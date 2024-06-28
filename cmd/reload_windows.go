//go:build windows

package cmd

import (
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
)

// GenerateConsoleCtrlEvent sends a specified signal to a console process group.
func sendCtrlC(processGroupId int) error {
	ret, _, err := procGenerateConsoleCtrlEvent.Call(windows.CTRL_C_EVENT, uintptr(processGroupId))
	if ret == 0 {
		return err
	}
	return nil
}
