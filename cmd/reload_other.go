//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

func sendCtrlC(_ int) error { return nil }

func signalReloadProcess(process *os.Process, _ int) error {
	return process.Signal(syscall.SIGINT)
}
