//go:build !windows

package main

import (
	"os"
	"syscall"
)

// processRunning 用信号 0 探测进程是否存在，不发送任何实际信号。
func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
