package listener

import (
	"fmt"
	"net"

	"fast-https/modules/core/socket"

	"golang.org/x/sys/unix"
)

func UnixListen(listen_type string, listen_ip_port string) int {
	// 获取是tcp的listenFd
	sfd := socket.UnixSockFdInit(1, 1)
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
