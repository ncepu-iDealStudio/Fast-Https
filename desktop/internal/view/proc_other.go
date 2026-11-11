//go:build !windows

package view

// 非 Windows 平台的进程枚举与强杀桩：StopServer 的兜底路径只在 Windows
// 上需要（Unix 下 stop 发 SIGTERM 语义完整），这里给最小实现保持编译。

import (
	"fmt"
	"os"
)

type ProcessInfo struct {
	Pid  uint32
	Path string
}

func GetProcessesByName(name string) []ProcessInfo { return nil }

func KillProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Kill(); err != nil {
		return fmt.Errorf("kill %d failed: %w", pid, err)
	}
	return nil
}
