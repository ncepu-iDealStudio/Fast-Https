package listener

import (
	"fmt"
	"net"

	"fast-https/modules/core/fh_socket"

	"golang.org/x/sys/unix"
)

const (
	HTTP        = 1
	HTTPS       = 2
	HTTP_PROXY  = 3
	HTTPS_PROXY = 4
	TCP_PROXY   = 5
)

// one listen port arg
type ListenInfo struct {
	Ltype int
	laddr string
	lport int
	Lfd   int
}

func Listen() []ListenInfo {
	lisi := make([]ListenInfo, 4)
	lisi[0].Lfd = listen_fd("tcp", "127.0.0.1:8080")
	lisi[0].laddr = "127.0.0.1"
	lisi[0].lport = 8080
	lisi[0].Ltype = HTTP

	return lisi
}

func listen_fd(listen_type string, listen_ip_port string) int {
	// 获取是tcp的listenFd
	sfd := fh_socket.SockFdInit(1, 1)
	ListenFd := sfd.Fd

	tcpAddr, err := net.ResolveTCPAddr(listen_type, listen_ip_port)
	if err != nil {
		fmt.Println("resove err", err)
		return 0
	}
	sa := &unix.SockaddrInet4{
		Port: tcpAddr.Port,
	}
	// 绑定的端口
	err = unix.Bind(ListenFd, sa)
	if err != nil {
		fmt.Println("socket bind err", err)
		return 0
	}
	var n int
	if n > 1<<16-1 {
		n = 1<<16 - 1
	}
	// 监听服务
	err = unix.Listen(ListenFd, n)
	if err != nil {
		fmt.Println("listen err", err)
		return 0
	}
	return ListenFd
}
