package websocket

import (
	"fast-https/modules/core"
	"fast-https/modules/proxy"
	"fast-https/utils/message"
	"fmt"
	"io"
)

// like a tcp proxy
func WebSocketHandler(ev *core.Event) {
	fmt.Println("+++++++++++++++++++++++++")

	proxy, flag := (ev.RR.CircleData).(*proxy.Proxy)
	if !flag {
		message.PrintErr("--proxy can not convert circle data to *Proxy")
	}

	buffer := make([]byte, 1024)
	for {
		n, err := ev.Conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				message.PrintWarn("[1]websocket: Proxy read error", err.Error())
				proxy.ProxyConn.Close()
				return
			}
		}

		if err == io.EOF {
			continue
		}
		fmt.Println(buffer[:n])

		_, err = proxy.ProxyConn.Write(buffer[:n])
		if err != nil {
			proxy.ProxyConn.Close()
			message.PrintWarn("websocket: Proxy Write error", err.Error())
			break
		}

		// ___________________________________

		n, err = proxy.ProxyConn.Read(buffer)
		if err != nil && err != io.EOF {
			message.PrintWarn("websocket: Proxy read error", err.Error())
			proxy.ProxyConn.Close()
			break
		}
		if err == io.EOF {
			continue
		}
		fmt.Println(buffer[:n])

		_, err = ev.Conn.Write(buffer[:n])
		if err != nil {
			message.PrintWarn("websocket: Proxy Write error", err.Error())
			proxy.ProxyConn.Close()
			break
		}
	}

}
