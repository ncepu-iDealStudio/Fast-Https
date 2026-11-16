//go:build !windows

package cli

import "os/exec"

// hideConsole 只在 Windows 需要：Unix 下 GUI 进程拉起子进程本来就没有控制台窗口。
func hideConsole(*exec.Cmd) {}
