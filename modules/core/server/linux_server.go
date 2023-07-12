package server

import (
	"fmt"

	"fast-https/modules/core/events"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/socket"

	"golang.org/x/sys/unix"
)

type eventList struct {
	size   int
	events []unix.EpollEvent
}

func newEventList(size int) *eventList {
	return &eventList{size, make([]unix.EpollEvent, size)}
}

func Unixrun() {

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
	ev.Events = unix.EPOLLIN
	// 把链接注册到epoll中
	err = unix.EpollCtl(epollFd, unix.EPOLL_CTL_ADD, ListenFd, &ev)
	if err != nil {
		fmt.Println("epoll ctl err", err)
		return
	}
	var fd int32
	var i int

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

		for i = 0; i < nready; i++ {
			fd = el.events[i].Fd

			if fd == int32(ListenFd) {
				// go func() {
				conn, _, err := unix.Accept(ListenFd)
				if err != nil {
					fmt.Println("accept err", err)
					return
				}
				socket.UnixFdNonBlock(conn)

				ev.Fd = int32(conn)
				ev.Events = unix.EPOLLIN
				// 把链接注册到epoll中
				err = unix.EpollCtl(epollFd, unix.EPOLL_CTL_ADD, conn, &ev)
				if err != nil {
					fmt.Println("epoll ctl err", err)
					return
				}
				// }()
			} else if el.events[i].Events&unix.EPOLLIN == 1 {

				// go func(fd int32, epollFd int) {

				events.UnixHandle_read(fd, epollFd)

				// go events.Handle_write(fd, epollFd)

				// }(fd, epollFd)

			} else if el.events[i].Events&unix.EPOLLOUT == 1 {
				events.UnixHandle_write(fd, epollFd)
			}
		}
	}
}
