//go:build unix

package txn

import (
	"os"
	"os/signal"
	"syscall"
)

func ignoreHangup() func() {
	signal.Ignore(syscall.SIGHUP, syscall.SIGPIPE)
	return func() {
		signal.Reset(syscall.SIGHUP, syscall.SIGPIPE)
		_ = os.Stdout
	}
}
