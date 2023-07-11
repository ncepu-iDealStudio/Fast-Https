package fh_socket

import (
	"fmt"

	"golang.org/x/sys/unix"
)

type SockFd struct {
	FdType uint8
	FdConf uint8
	Fd     int
}

func SockFdInit(fdtype uint8, fdconf uint) *SockFd {
	var sockfd SockFd
	if fdtype == 1 {
		sockfd.FdType = 1
		sockfd.Fd = ListenFd()
	} else if fdtype == 2 {
		sockfd.FdType = 2
		sockfd.Fd = ClientFd()
	} else {
		fmt.Println("error fdtype")
	}
	return &sockfd
}

func ListenFd() int {
	ListenFd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, unix.IPPROTO_TCP)
	if err != nil {
		fmt.Println("create socket err", err)
		return 0
	}
	FdConfigure(ListenFd)
	return ListenFd
}

func ClientFd() int {
	ListenFd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, unix.IPPROTO_TCP)
	if err != nil {
		fmt.Println("create socket err", err)
		return 0
	}
	return ListenFd
}

func FdConfigure(Fd int) {
	err := unix.SetsockoptInt(Fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
	if err != nil {
		fmt.Println("set socket err", err)
	}
}

func FdNonBlock(Fd int) {
	err := unix.SetNonblock(Fd, true)
	if err != nil {
		fmt.Println("block err", err)
		return
	}
}
