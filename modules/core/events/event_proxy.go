package events

import (
	"crypto/tls"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"io"
	"net"
	"strconv"
	"strings"
)

func parse_header(ev *Event) ([]byte, []byte, int) {
	tmpByte := make([]byte, 1024*1024)
	header := make([]byte, 1024*1024)
	var header_str string
	var i int
	var k int
	var res []byte

	tmp_len, _ := ev.ProxyConn.Read(tmpByte)
	for i = 0; i < tmp_len-4; i++ {
		if tmpByte[i] == byte(13) && tmpByte[i+1] == byte(10) && tmpByte[i+2] == byte(13) && tmpByte[i+3] == byte(10) {
			break
		}
		header[i] = tmpByte[i]
	}

	header_str = string(header[:i])
	lines := strings.Split(header_str, "\r\n")[1:]
	for _, line := range lines {
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if strings.Compare(key, "Content-Length") == 0 {
				k, _ = strconv.Atoi(value)
			}
		}

	}
	if k == 0 {
		res = (tmpByte[i+4:])
	} else {
		res = (tmpByte[i+4:])[:k]
	}
	return res, header[:i], 0
}

// fast-https will send data to real server and get response from target
func get_data_from_server(ev *Event, proxyaddr string, data []byte) ([]byte, int) {
	if !strings.Contains(proxyaddr, ":") {
		proxyaddr = proxyaddr + ":80"
	}

	var err error
	// if ev.ProxyConn == nil {
	ev.ProxyConn, err = net.Dial("tcp", proxyaddr)
	if err != nil {
		message.PrintWarn("[Proxy event]: Can't connect to "+proxyaddr, err.Error())
		write_bytes_close(ev, response.Default_server_error())
		return nil, 1 // no server
	}
	// }

	_, err = ev.ProxyConn.Write(data)
	if err != nil {
		ev.ProxyConn.Close()
		message.PrintErr("Proxy Write error")
	}
	// fmt.Println(string(data))

	var resData []byte
	tmpByte := make([]byte, 512)
	for {
		len_once, err := ev.ProxyConn.Read(tmpByte)
		if err != nil {
			if err == io.EOF { // read all
				break
			} else {
				ev.ProxyConn.Close()
				message.PrintWarn("Proxy Read error ", err)
			}
		}
		if len_once == 0 {
			break
		}
		resData = append(resData, tmpByte[:len_once]...)
	}

	if !ev.Req_.Is_keepalive() { // connection close
		ev.ProxyConn.Close()
	}

	// fmt.Println(string(resData))
	message.PrintAccess(ev.Conn.RemoteAddr().String(), " PROXY HTTP Events "+ev.Log, " "+ev.Req_.Headers["User-Agent"])
	return resData, 0 // no error
}

// fast-https will send data to real server and get response from target
func get_data_from_ssl_server(ev *Event, proxyaddr string, data []byte) ([]byte, int) {
	if !strings.Contains(proxyaddr, ":") {
		proxyaddr = proxyaddr + ":443"
	}

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
	tmpByte := make([]byte, 512)
	for {
		len_once, err := tlsConn.Read(tmpByte)
		if err != nil {
			if err == io.EOF { // read all
				break
			} else {
				tlsConn.Close()
				message.PrintWarn("Proxy Read error", err)
			}
		}
		if len_once == 0 {
			break
		}
		resData = append(resData, tmpByte[:len_once]...)
	}

	if !ev.Req_.Is_keepalive() {
		tlsConn.Close()
	}

	message.PrintAccess(ev.Conn.RemoteAddr().String(), " PROXY HTTPS Events "+ev.Log, " "+ev.Req_.Headers["User-Agent"])
	return resData, 0 // no error
}

func Proxy_event(req_data []byte, proxyaddr string, Proxy uint8, ev *Event) {
	if Proxy == 1 { // http proxy
		res, _ := get_data_from_server(ev, proxyaddr, req_data)
		if ev.Req_.Is_keepalive() {
			write_bytes(ev, res)
			Handle_event(ev)
		} else {
			write_bytes_close(ev, res)
		}
	} else {
		res, _ := get_data_from_ssl_server(ev, proxyaddr, req_data)
		if ev.Req_.Is_keepalive() {
			write_bytes(ev, res)
			Handle_event(ev)
		} else {
			write_bytes_close(ev, res)
		}
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
