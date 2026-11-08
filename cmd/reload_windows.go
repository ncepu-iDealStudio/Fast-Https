//go:build windows

package cmd

import (
	"os"

	"fast-https/management/svcctl"
)

func sendCtrlC(_ int) error { return nil }

func signalReloadProcess(_ *os.Process, pid int) error {
	return svcctl.SignalReload(pid)
}
