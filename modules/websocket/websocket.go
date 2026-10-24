// Package websocket 升级 HTTP 连接并把 WebSocket 帧转发到上游。
package websocket

import (
	"fast-https/core"
	"fast-https/modules/proxy"
	"fast-https/infra/logging"
	"io"
	"net"
	"time"
)

// like a tcp proxy
// todo: improve buffer
func WebSocketHandler(ev *core.Event) {
	// fmt.Println("+++++++++++++++++++++++++")

	proxy, flag := (ev.RR.CircleData).(*proxy.Proxy)
	if !flag || proxy == nil || proxy.ProxyConn == nil {
		// 断言失败或上游连接未初始化时必须返回，否则下面就是空指针解引用
		logging.PrintErr("--proxy can not convert circle data to *Proxy")
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
					logging.PrintWarn("[1]websocket: Proxy read error ", err.Error())
					proxy.ProxyConn.Close()
					return
				}
			}

			_, err = proxy.ProxyConn.Write(buffer[:n])
			if err != nil {
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					ev.Close()
					return
				} else {
					proxy.ProxyConn.Close()
					logging.PrintWarn("websocket: Proxy Write error ", err.Error())
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
				break
			} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				continue
			} else {
				logging.PrintWarn("websocket: Proxy read error ", err.Error())
				proxy.ProxyConn.Close()
				return
			}

		}

		_, err = ev.Conn.Write(buffer[:n])
		if err != nil {

			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				proxy.ProxyConn.Close()
				return
			} else {
				logging.PrintWarn("websocket: Proxy Write error ", err.Error())
				proxy.ProxyConn.Close()
				break
			}
		}
	}

}
