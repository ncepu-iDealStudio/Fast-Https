package events

import (
	"fast-https/utils/message"
	"net"
)

func get_data_from_server(proxyaddr string, data []byte) ([]byte, int) {

	conn, err := net.Dial("tcp", proxyaddr)
	if err != nil {

		message.PrintWarn("Can't connect to "+proxyaddr, err.Error())
		return nil, 1 // no server
	}
	_, err = conn.Write(data)
	if err != nil {
		conn.Close()
		message.PrintErr("Proxy Write error")
	}

	buffer := make([]byte, 1024*512)
	n, err := conn.Read(buffer)
	if err != nil {
		conn.Close()
		message.PrintWarn("Proxy Read error", err)
	}
	conn.Close()

	return buffer[:n], 0 // no error
}

func Proxy_event(req_data []byte, proxyaddr string) ([]byte, int) {

	return get_data_from_server(proxyaddr, req_data)
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
