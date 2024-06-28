//go:build !linux || !amd64

package listener

import (
	"syscall"
)

var ReuseCallBack func(network, address string, c syscall.RawConn) error

func init() {
	ReuseCallBack = func(_, _ string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {

		})
	}

}
