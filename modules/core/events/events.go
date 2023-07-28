package events

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/core/timer"
	"fast-https/utils/message"
	"io"
	"net"
	"strings"
)

// each request event is saved in this struct
type Event struct {
	Conn     net.Conn
	Lis_info listener.ListenInfo
	Req_     *request.Req
	Timer    *timer.Timer
}

// for static requests which not end with "/"
// attention: if backends use API interface, they
// must end with "/"
func _event_301(ev Event, path string) {
	res := []byte("HTTP/1.1 301 Moved Permanently\r\n" +
		"Location: " + path + "\r\n" +
		"Connection: close\r\n" +
		"\r\n")
	write_bytes_close(ev, res)
}

// distribute event
// LisType(2) tcp proxy
func Handle_event(ev Event) {

	// handle tcp proxy
	if ev.Lis_info.LisType == 2 {
		Proxy_event_tcp(ev.Conn, ev.Lis_info.Data[0].Proxy_addr)
		return
	}

	// read data (bytes and str) from socket
	byte_row, str_row := read_data(ev)
	// save requte infomation to ev.Req_
	ev.Req_ = request.Req_init()
	if byte_row == nil || str_row == "" { // client closed
		return
	} else {
		ev.Req_.Http_parse(str_row)
		// parse host
		ev.Req_.Parse_host(ev.Lis_info)
	}

	message.PrintInfo("Events ", ev.Conn.RemoteAddr(), " "+ev.Req_.Method, " "+ev.Req_.Path)

	for _, d := range ev.Lis_info.Data {
		switch d.Proxy {
		case 0: // Proxy: 0, static events
			if ev.Req_.Host == d.ServerName && strings.HasPrefix(ev.Req_.Path, d.Path) {
				row_file_path := ev.Req_.Path[len(d.Path):]

				// according to user's confgure and requets endporint handle events
				if d.Path != "/" {
					if row_file_path == "" {
						_event_301(ev, d.Path+"/")
						return
					}
					Static_event(d, d.StaticRoot+row_file_path, ev)
					return
				} else {
					Static_event(d, d.StaticRoot+ev.Req_.Path, ev)
					return
				}
			}
		case 1, 2: // proxy: 1 or 2,  proxy events
			if ev.Req_.Host == d.ServerName {

				// according to user's confgure and requets endporint handle events
				res, err := Proxy_event(ev, byte_row, d.Proxy_addr, d.Proxy)
				if err == 1 { // target no response
					write_bytes_close(ev, response.Default_server_error)
					return
				}
				if ev.Req_.Connection == "close" {
					write_bytes_close(ev, res)
					return
				} else {
					write_bytes_close(ev, res)
					// write_bytes(ev, res)
					return
					// Handle_event(ev)
				}
			}
		}
	}
	write_bytes_close(ev, response.Default_not_found)
}

// read data from EventFd
// attention: row str only can be used when parse FirstLine or Headers
// because request body maybe contaions '\0'
func read_data(ev Event) ([]byte, string) {
	buffer := make([]byte, 1024*4)
	n, err := ev.Conn.Read(buffer)
	if err != nil {
		if err == io.EOF { // read None, remoteAddr is closed
			return nil, ""
		}
		if n == 0 {
			// message.PrintInfo(ev.Conn.RemoteAddr(), " closed")
			return nil, ""
		}
		message.PrintErr("Error reading from client:", err)
	}
	str_row := string(buffer[:n])
	// buffer = buffer[:n]
	return buffer, str_row // return row str or bytes
}

// write row bytes and close
func write_bytes_close(ev Event, data []byte) {
	for len(data) > 0 {
		n, err := ev.Conn.Write(data)
		if err != nil {
			message.PrintErr("Error writing to client:", err)
			return
		}
		data = data[n:]
	}
	err := ev.Conn.Close()
	if err != nil {
		message.PrintErr("Error Close:", err)
	}
}

// write row bytes
func write_bytes(ev Event, data []byte) {
	for len(data) > 0 {
		n, err := ev.Conn.Write(data)
		if err != nil {
			message.PrintErr("Error writing to client:", err)
			return
		}
		data = data[n:]
	}
}

// only close the connection
func close(ev Event) {
	err := ev.Conn.Close()
	if err != nil {
		message.PrintErr("Error Close:", err)
	}
}
