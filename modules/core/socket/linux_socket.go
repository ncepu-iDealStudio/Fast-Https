package socket

import (
	"fmt"

	"golang.org/x/sys/unix"
)

type SockFd struct {
	FdType uint8
	FdConf uint8
	Fd     int
}

func UnixSockFdInit(fdtype uint8, fdconf uint) *SockFd {
	var sockfd SockFd
	if fdtype == 1 {
		sockfd.FdType = 1
		sockfd.Fd = UnixListenFd()
	} else if fdtype == 2 {
		sockfd.FdType = 2
		sockfd.Fd = UnixClientFd()
	} else {
		fmt.Println("error fdtype")
	}
	return &sockfd
}

func UnixListenFd() int {
	ListenFd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, unix.IPPROTO_TCP)
	if err != nil {
		fmt.Println("create socket err", err)
		return 0
	}
	UnixFdConfigure(ListenFd)
	UnixFdNonBlock(ListenFd)
	return ListenFd
}

func UnixClientFd() int {
	ListenFd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, unix.IPPROTO_TCP)
	if err != nil {
		fmt.Println("create socket err", err)
		return 0
	}
	return ListenFd
}

func UnixFdConfigure(Fd int) {
	err := unix.SetsockoptInt(Fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
	if err != nil {
		fmt.Println("set socket err", err)
	}
}

func UnixFdNonBlock(Fd int) {
	err := unix.SetNonblock(Fd, true)
	if err != nil {
		fmt.Println("block err", err)
		return
	}
}
