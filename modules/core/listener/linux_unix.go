//go:build linux && amd64

package listener

import (
	"syscall"

	"golang.org/x/sys/unix"
)

var ReuseCallBack func(network, address string, c syscall.RawConn) error

func init() {
	ReuseCallBack = func(_, _ string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			syscall.SetsockoptInt((int(fd)), syscall.SOL_SOCKET, unix.SO_REUSEADDR, 1)
			syscall.SetsockoptInt((int(fd)), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		})
	}

}
