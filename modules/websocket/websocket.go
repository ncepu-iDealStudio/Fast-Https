// Package websocket 升级 HTTP 连接并把 WebSocket 帧转发到上游。
package websocket

import (
	"fast-https/config"
	"fast-https/core"
	"fast-https/infra/logging"
	"fast-https/modules/proxy"
	"io"
	"net"
)

// like a tcp proxy
// todo: improve buffer
func WebSocketHandler(ev *core.Event) {
	p, flag := (ev.RR.CircleData).(*proxy.Proxy)
	if !flag || p == nil || (p.ProxyConn == nil && p.ProxyTlsConn == nil) {
		// 断言失败或上游连接未初始化时必须返回，否则下面就是空指针解引用
		logging.PrintErr("--proxy can not convert circle data to *Proxy")
		ev.Close()
		return
	}

	upstreamConn := p.ProxyConn
	if p.ProxyType == config.PROXY_HTTPS && p.ProxyTlsConn != nil {
		upstreamConn = p.ProxyTlsConn
	}
	defer func() {
		if p.Close != nil {
			_ = p.Close()
		} else if upstreamConn != nil {
			_ = upstreamConn.Close()
		}
		ev.Close()
	}()

	// 双向共享同一份空闲状态：任意方向有数据都算活动。
	idle := core.NewTunnelIdle()

	go func() {
		buffer := make([]byte, 1024)
		for {
			ev.Conn.SetDeadline(idle.Deadline())
			n, err := ev.Conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					if cw, ok := upstreamConn.(interface{ CloseWrite() error }); ok {
						_ = cw.CloseWrite()
					}
					break
				} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					if idle.Expired() {
						logging.ConsoleLog.Debug().Msg("websocket: idle tunnel closed")
						ev.Close()
						_ = upstreamConn.Close()
						return
					}
					continue
				} else {
					logging.PrintWarn("[1]websocket: Proxy read error ", err.Error())
					_ = upstreamConn.Close()
					return
				}
			}
			idle.Touch()

			_, err = upstreamConn.Write(buffer[:n])
			if err != nil {
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					ev.Close()
					_ = upstreamConn.Close()
					return
				} else {
					_ = upstreamConn.Close()
					logging.PrintWarn("websocket: Proxy Write error ", err.Error())
					break
				}
			}
		}
	}()

	buffer := make([]byte, 1024)
	for {

		upstreamConn.SetDeadline(idle.Deadline())
		n, err := upstreamConn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			} else if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				if idle.Expired() {
					logging.ConsoleLog.Debug().Msg("websocket: idle tunnel closed")
					ev.Close()
					_ = upstreamConn.Close()
					return
				}
				continue
			} else {
				logging.PrintWarn("websocket: Proxy read error ", err.Error())
				_ = upstreamConn.Close()
				return
			}

		}
		idle.Touch()

		_, err = ev.Conn.Write(buffer[:n])
		if err != nil {

			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				_ = upstreamConn.Close()
				return
			} else {
				logging.PrintWarn("websocket: Proxy Write error ", err.Error())
				_ = upstreamConn.Close()
				break
			}
		}
	}

}
