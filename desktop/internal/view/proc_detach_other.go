//go:build !windows

package view

import "os/exec"

// 非 Windows 平台不需要特殊分离：主程序自己会 daemonize
// （见 cmd/commands.go 的 StartHandler）。
func setDetached(cmd *exec.Cmd) {}
