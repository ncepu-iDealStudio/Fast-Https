// Package proxy_tcp 把原始 TCP 连接转发到配置的上游。
package proxy_tcp

import (
	"fast-https/core"
	"fast-https/infra/logging"
	"io"
	"net"
	"time"
)

// This is a simple demo
func ProxyEventTcp(ev *core.Event, proxyaddr string) {

	conn2, err := net.Dial("tcp", proxyaddr)
	if err != nil {
		// 建连失败必须返回，否则下面 conn2 就是空指针解引用
		logging.PrintErr("Can't connect to "+proxyaddr, err.Error())
		ev.Close()
		return
	}

	go func() {
		buffer := make([]byte, 1024)
		for {
			ev.Conn.SetDeadline(time.Now().Add(time.Second * 10))
			n, err := ev.Conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					break
				} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					continue
				} else {
					logging.PrintWarn("[1]tcp_proxy: Proxy read error ", err.Error())
					conn2.Close()
					return
				}
			}

			_, err = conn2.Write(buffer[:n])
			if err != nil {
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					ev.Close()
					return
				} else {
					conn2.Close()
					logging.PrintWarn("tcp_proxy: Proxy Write error ", err.Error())
					break
				}
			}
		}
	}()

	buffer := make([]byte, 1024)
	for {

		conn2.SetDeadline(time.Now().Add(time.Second * 10))
		n, err := conn2.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				continue
			} else {
				logging.PrintWarn("tcp_proxy: Proxy read error ", err.Error())
				conn2.Close()
				return
			}

		}

		_, err = ev.Conn.Write(buffer[:n])
		if err != nil {

			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				conn2.Close()
				return
			} else {
				logging.PrintWarn("tcp_proxy: Proxy Write error ", err.Error())
				conn2.Close()
				break
			}
		}
	}
}
