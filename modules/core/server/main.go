// package listener
package main

import (
	"fmt"

	"fast-https/modules/core/fh_socket"
	"fast-https/modules/core/listener"

	"golang.org/x/sys/unix"
)

type eventList struct {
	size   int
	events []unix.EpollEvent
}

func newEventList(size int) *eventList {
	return &eventList{size, make([]unix.EpollEvent, size)}
}

func run() {

	// linfo := make([]listener.ListenInfo, 4)
	linfo := listener.Listen()
	ListenFd := linfo[0].Lfd

	el := newEventList(1024)

	epollFd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		unix.Close(epollFd)
		fmt.Println("create fd err", err)
		return
	}

	ev := unix.EpollEvent{}
	ev.Fd = int32(ListenFd)
	ev.Events = unix.EPOLLPRI | unix.EPOLLIN
	// 把链接注册到epoll中
	err = unix.EpollCtl(epollFd, unix.EPOLL_CTL_ADD, ListenFd, &ev)
	if err != nil {
		fmt.Println("epoll ctl err", err)
		return
	}

	for {

		// 有读写事件到来就会得到通知
		nready, err := unix.EpollWait(epollFd, el.events, 0)
		if nready <= 0 {
			continue
		}
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			fmt.Println("listenner wait err", err)
			return
		}

		for i := 0; i < nready; i++ {
			fd := el.events[i].Fd

			if fd == int32(ListenFd) {
				conn, _, err := unix.Accept(ListenFd)
				if err != nil {
					fmt.Println("accept err", err)
					return
				}
				fh_socket.FdNonBlock(conn)

				ev := unix.EpollEvent{}
				ev.Fd = int32(conn)
				ev.Events = unix.EPOLLIN
				// 把链接注册到epoll中
				err = unix.EpollCtl(epollFd, unix.EPOLL_CTL_ADD, conn, &ev)
				if err != nil {
					fmt.Println("epoll ctl err", err)
					return
				}

			} else if el.events[i].Events&unix.EPOLLIN == 1 {

				buf := make([]byte, 1024)

				n, err := unix.Read(int(fd), buf)
				if err != nil {
					unix.Close(int(el.events[i].Fd))
					break
				}
				if n > 0 {
					ev := unix.EpollEvent{
						Events: unix.EPOLLOUT,
						Fd:     fd,
					}
					if err := unix.EpollCtl(epollFd, unix.EPOLL_CTL_MOD, int(fd), &ev); err != nil {
						fmt.Println("get data   err")
						continue
					}
					fmt.Println("[suc]", string(buf[:n]))
				}
				if n <= 0 {
					ev := unix.EpollEvent{
						Events: unix.EPOLLOUT | unix.EPOLLIN | unix.EPOLLERR | unix.EPOLLHUP,
						Fd:     fd,
					}
					unix.Close(int(fd))
					if err := unix.EpollCtl(epollFd, unix.EPOLL_CTL_DEL, int(fd), &ev); err != nil {
						fmt.Println("close epoll err")
						return
					}
				}

			} else if el.events[i].Events&unix.EPOLLOUT == 1 {
				// write_buf := make([]byte, 1024)
				data := "HTTP/1.1 200 OK\r\n\r\nHello World"
				write_buf := []byte(data)

				_, err := unix.Write(int(fd), write_buf)
				if err != nil {
					fmt.Println("Write data   err")
					continue
				}

				ev := unix.EpollEvent{
					Events: unix.EPOLLOUT | unix.EPOLLIN | unix.EPOLLERR | unix.EPOLLHUP,
					Fd:     fd,
				}
				unix.Close(int(fd))
				if err := unix.EpollCtl(epollFd, unix.EPOLL_CTL_DEL, int(fd), &ev); err != nil {
					fmt.Println("close epoll err")
					return
				}

			}
		}
	}
}

func main() {
	run()
}
