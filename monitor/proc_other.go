//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
)

// processRunning 用信号 0 探测进程是否存在，不发送任何实际信号。
//
// EPERM 表示进程存在、只是当前用户无权探测它（例如 Monitor 以普通用户
// 观察 root 启动的服务），要按「在跑」处理，否则会被误判成已经崩溃。
func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, os.ErrPermission)
}
