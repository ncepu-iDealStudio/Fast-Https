package proxy_tcp

import (
	"fast-https/modules/core"
	"fast-https/utils/message"
	"fmt"
	"io"
	"net"
	"time"
)

// This is a simple demo
func ProxyEventTcp(ev *core.Event, proxyaddr string) {

	conn2, err := net.Dial("tcp", proxyaddr)
	if err != nil {
		message.PrintErr("Can't connect to "+proxyaddr, err.Error())
	}

	go func() {
		buffer := make([]byte, 1024)
		for {
			ev.Conn.SetDeadline(time.Now().Add(time.Second * 10))
			n, err := ev.Conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					fmt.Println("eof 1")
					break
				} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					// fmt.Println("time out once 1")
					continue
				} else {
					message.PrintWarn("[1]tcp_proxy: Proxy read error ", err.Error())
					conn2.Close()
					return
				}
			}

			// fmt.Println("read from postman:", buffer[:n])

			_, err = conn2.Write(buffer[:n])
			if err != nil {
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					fmt.Println("tcp_proxy server closed ...")
					ev.Close()
					return
				} else {
					conn2.Close()
					message.PrintWarn("tcp_proxy: Proxy Write error ", err.Error())
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
				fmt.Println("eof 2")
				break
			} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				// fmt.Println("time out once 2")
				continue
			} else {
				message.PrintWarn("tcp_proxy: Proxy read error ", err.Error())
				conn2.Close()
				return
			}

		}

		// fmt.Println("read from tcp_proxy server", buffer[:n])

		_, err = ev.Conn.Write(buffer[:n])
		if err != nil {

			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				fmt.Println("chrome client closed ...")
				conn2.Close()
				return
			} else {
				message.PrintWarn("tcp_proxy: Proxy Write error ", err.Error())
				conn2.Close()
				break
			}
		}
	}
}
