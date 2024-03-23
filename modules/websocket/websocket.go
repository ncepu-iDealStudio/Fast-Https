package websocket

import (
	"fast-https/modules/core"
	"fast-https/modules/proxy"
	"fast-https/utils/message"
	"fmt"
	"io"
)

var Res = `
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: HSmrc0sMlYUkAGmm5OPpG2HaGWk=
Sec-WebSocket-Protocol: chat 
`

// like a tcp proxy
func WebSocketHandler(ev *core.Event) {
	fmt.Println("+++++++++++++++++++++++++")

	proxy, flag := (ev.RR.CircleData).(*proxy.Proxy)
	if !flag {
		message.PrintErr("--proxy can not convert circle data to *Proxy")
	}

	buffer := make([]byte, 1024)
	var in []byte
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
		in = append(in, buffer[:n]...)
	}
	fmt.Println(in)

	for {
		n, err := proxy.ProxyConn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				message.PrintWarn("[2]websocket: Proxy read error", err.Error())
				proxy.ProxyConn.Close()
				return
			}
		}
		in = append(in, buffer[:n]...)
	}
	fmt.Println(in)

	/*for {

		if err == io.EOF {
			continue
		}
		fmt.Println(buffer[:n])
		fmt.Println("write to upper", buffer[:n])
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

		fmt.Println("write to down", buffer[:n])
		_, err = ev.Conn.Write(buffer[:n])
		if err != nil {
			message.PrintWarn("websocket: Proxy Write error", err.Error())
			proxy.ProxyConn.Close()
			break
		}
	}*/

}
