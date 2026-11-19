// Package websocket 升级 HTTP 连接并把 WebSocket 帧转发到上游。
package websocket

import (
	"fast-https/core"
	"fast-https/modules/proxy"
	"fast-https/infra/logging"
	"io"
	"net"
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

	// 双向共享同一份空闲状态：任意方向有数据都算活动。
	idle := core.NewTunnelIdle()

	go func() {
		buffer := make([]byte, 1024)
		for {
			ev.Conn.SetDeadline(idle.Deadline())
			n, err := ev.Conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					break
				} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					if idle.Expired() {
						logging.ConsoleLog.Debug().Msg("websocket: idle tunnel closed")
						ev.Close()
						proxy.ProxyConn.Close()
						return
					}
					continue
				} else {
					logging.PrintWarn("[1]websocket: Proxy read error ", err.Error())
					proxy.ProxyConn.Close()
					return
				}
			}
			idle.Touch()

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

		proxy.ProxyConn.SetDeadline(idle.Deadline())
		n, err := proxy.ProxyConn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				if idle.Expired() {
					logging.ConsoleLog.Debug().Msg("websocket: idle tunnel closed")
					ev.Close()
					proxy.ProxyConn.Close()
					return
				}
				continue
			} else {
				logging.PrintWarn("websocket: Proxy read error ", err.Error())
				proxy.ProxyConn.Close()
				return
			}

		}
		idle.Touch()

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
