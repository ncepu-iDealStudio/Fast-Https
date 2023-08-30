package events

import (
	"crypto/tls"
	"fast-https/utils/message"
	"fmt"
	"io"
	"net"
)

// fast-https will send data to real server and get response from target
func get_data_from_server(ev *Event, proxyaddr string, data []byte) ([]byte, int) {
	var err error
	// if ev.ProxyConn == nil {
	ev.ProxyConn, err = net.Dial("tcp", proxyaddr)
	if err != nil {

		message.PrintWarn("[Proxy event]: Can't connect to "+proxyaddr, err.Error())
		return nil, 1 // no server
	}
	// }

	_, err = ev.ProxyConn.Write(data)
	if err != nil {
		ev.ProxyConn.Close()
		message.PrintErr("Proxy Write error")
	}

	var resData []byte
	tmpByte := make([]byte, 1)
	for {
		len, err := ev.ProxyConn.Read(tmpByte)
		if err != nil {
			if err == io.EOF { // read all
				break
			} else {
				ev.ProxyConn.Close()
				message.PrintWarn("Proxy Read error", err)
			}
		}
		if len == 0 {
			break
		}
		resData = append(resData, tmpByte...)
	}
	// if ev.Req_.Connection == "close" {
	// ev.ProxyConn.Close()
	// }

	ev.ProxyConn.Close()
	fmt.Println(ev.Conn.RemoteAddr().String(), " PROXY Events "+ev.Log, " "+ev.Req_.Headers["User-Agent"])

	// fmt.Print(string(resData))
	return resData, 0 // no error
}

// fast-https will send data to real server and get response from target
func get_data_from_ssl_server(ev *Event, proxyaddr string, data []byte) ([]byte, int) {
	var err error
	ev.ProxyConn, err = net.Dial("tcp", proxyaddr)
	if err != nil {
		message.PrintWarn("Can't connect to "+proxyaddr, err.Error())
		return nil, 1 // no server
	}

	config := tls.Config{InsecureSkipVerify: true}
	tlsConn := tls.Client(ev.ProxyConn, &config)

	_, err = tlsConn.Write(data)
	if err != nil {
		tlsConn.Close()
		message.PrintErr("Proxy Write error")
		return nil, 1
	}

	var resData []byte
	tmpByte := make([]byte, 1)
	for {
		len, err := tlsConn.Read(tmpByte)
		if err != nil {
			if err == io.EOF { // read all
				break
			} else {
				tlsConn.Close()
				message.PrintWarn("Proxy Read error", err)
			}
		}
		if len == 0 {
			break
		}
		resData = append(resData, tmpByte...)
	}

	// if ev.Req_.Connection == "close" {
	// 	tlsConn.Close()
	// }

	tlsConn.Close()

	fmt.Println(ev.Conn.RemoteAddr().String(), " PROXY Events "+ev.Log, " "+ev.Req_.Headers["User-Agent"])

	return resData, 0 // no error
}

func Proxy_event(ev *Event, req_data []byte, proxyaddr string, Proxy uint8) ([]byte, int) {

	if Proxy == 1 {
		return get_data_from_server(ev, proxyaddr, req_data)
	} else {
		return get_data_from_ssl_server(ev, proxyaddr, req_data)
	}

}

func Proxy_event_tcp(conn net.Conn, proxyaddr string) {

	conn2, err := net.Dial("tcp", proxyaddr)
	if err != nil {
		message.PrintErr("Can't connect to "+proxyaddr, err.Error())
	}
	buffer := make([]byte, 1024)

	for {

		n, err := conn.Read(buffer)
		if err != nil {
			if n == 0 {
				conn2.Close()
				conn.Close()
				break
			}
		}

		_, err = conn2.Write(buffer[:n])
		if err != nil {
			conn2.Close()
			message.PrintErr("Proxy Write error")
		}

	}
}
