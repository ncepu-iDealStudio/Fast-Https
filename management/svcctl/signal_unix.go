//go:build !windows

package svcctl

import (
	"os"
	"syscall"
)

// SignalReload 向进程发 SIGINT。Server 在各平台把 SIGINT 当成 reload。
func SignalReload(pid int) error {
	if pid <= 0 {
		return os.ErrInvalid
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGINT)
}
