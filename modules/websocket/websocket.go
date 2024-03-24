package websocket

import (
	"fast-https/modules/core"
	"fast-https/modules/proxy"
	"fast-https/utils/message"
	"fmt"
	"io"
	"net"
	"time"
)

// like a tcp proxy
// todo: improve buffer
func WebSocketHandler(ev *core.Event) {
	// fmt.Println("+++++++++++++++++++++++++")

	proxy, flag := (ev.RR.CircleData).(*proxy.Proxy)
	if !flag {
		message.PrintErr("--proxy can not convert circle data to *Proxy")
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
					message.PrintWarn("[1]websocket: Proxy read error ", err.Error())
					proxy.ProxyConn.Close()
					return
				}
			}

			// fmt.Println("read from postman:", buffer[:n])

			_, err = proxy.ProxyConn.Write(buffer[:n])
			if err != nil {
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					fmt.Println("websocket server closed ...")
					ev.Close()
					return
				} else {
					proxy.ProxyConn.Close()
					message.PrintWarn("websocket: Proxy Write error ", err.Error())
					break
				}
			}
		}
	}()

	buffer := make([]byte, 1024)
	for {

		proxy.ProxyConn.SetDeadline(time.Now().Add(time.Second * 10))
		n, err := proxy.ProxyConn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				fmt.Println("eof 2")
				break
			} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				// fmt.Println("time out once 2")
				continue
			} else {
				message.PrintWarn("websocket: Proxy read error ", err.Error())
				proxy.ProxyConn.Close()
				return
			}

		}

		// fmt.Println("read from websocket server", buffer[:n])

		_, err = ev.Conn.Write(buffer[:n])
		if err != nil {

			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				fmt.Println("postman client closed ...")
				proxy.ProxyConn.Close()
				return
			} else {
				message.PrintWarn("websocket: Proxy Write error ", err.Error())
				proxy.ProxyConn.Close()
				break
			}
		}
	}

}
