package events

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func UnixHandle_read(fd int32, epollFd int) {
	buf := make([]byte, 1024)

	n, err := unix.Read(int(fd), buf)
	if err != nil {
		if err := unix.EpollCtl(epollFd, unix.EPOLL_CTL_DEL, int(fd), nil); err != nil {
			fmt.Println("close epoll err888")
		}
		unix.Close(int(fd))
		return

	}
	if n > 0 {
		fmt.Println("[suc]", string(buf[:n]))
	}
	if n <= 0 {
		if err := unix.EpollCtl(epollFd, unix.EPOLL_CTL_DEL, int(fd), nil); err != nil {
			fmt.Println("close epoll err888")
		}
		unix.Close(int(fd))
		return
	}

	// ev := unix.EpollEvent{}
	// ev.Fd = int32(fd)
	// ev.Events = unix.EPOLLOUT
	// // 把链接注册到epoll中
	// err = unix.EpollCtl(epollFd, unix.EPOLL_CTL_MOD, int(fd), &ev)
	// if err != nil {
	// 	fmt.Println("epoll ctl err", err)
	// 	return
	// }

	UnixHandle_write(fd, epollFd)
}

func UnixHandle_write(fd int32, epollFd int) {
	data := "HTTP/1.1 200 OK\r\n\r\nHello World"
	write_buf := []byte(data)

	_, err1 := unix.Write(int(fd), write_buf)
	if err1 != nil {
		fmt.Println("Write data   err")
	}

	if err := unix.EpollCtl(epollFd, unix.EPOLL_CTL_DEL, int(fd), nil); err != nil {
		fmt.Println("close epoll err999")
	}
	unix.Close(int(fd))
}
