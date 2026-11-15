//go:build windows

package servicecmd

import (
	"os"

	"fast-https/management/svcctl"
)

func SendCtrlC(_ int) error { return nil }

func SignalReloadProcess(_ *os.Process, pid int) error {
	return svcctl.SignalReload(pid)
}
