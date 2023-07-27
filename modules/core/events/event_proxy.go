package events

import (
	"crypto/tls"
	"fast-https/utils/message"
	"io"
	"net"
)

// fast-https will send data to real server and get response from target
func get_data_from_server(ev Event, proxyaddr string, data []byte) ([]byte, int) {

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

	var resData []byte
	tmpByte := make([]byte, 1)
	for {
		len, err := conn.Read(tmpByte)
		if err != nil {
			if err == io.EOF { // read all
				break
			} else {
				conn.Close()
				message.PrintWarn("Proxy Read error", err)
			}
		}
		if len == 0 {
			break
		}
		resData = append(resData, tmpByte...)
	}
	// if ev.Req_.Connection == "close" {
	// 	conn.Close()
	// }
	conn.Close()
	return resData, 0 // no error
}

// fast-https will send data to real server and get response from target
func get_data_from_ssl_server(ev Event, proxyaddr string, data []byte) ([]byte, int) {

	conn, err := net.Dial("tcp", proxyaddr)
	if err != nil {

		message.PrintWarn("Can't connect to "+proxyaddr, err.Error())
		return nil, 1 // no server
	}

	config := tls.Config{InsecureSkipVerify: true}
	tlsConn := tls.Client(conn, &config)

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
	conn.Close()
	return resData, 0 // no error
}

func Proxy_event(ev Event, req_data []byte, proxyaddr string, Proxy uint8) ([]byte, int) {

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
