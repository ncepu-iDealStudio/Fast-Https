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

type Event struct {
	Conn     net.Conn
	Lis_info listener.ListenInfo
	Req_     *request.Req
	Timer    *timer.Timer
}

func _event_301(conn net.Conn, path string) {
	res := []byte("HTTP/1.1 301 Moved Permanently\r\n" +
		"Location: " + path + "\r\n" +
		"Connection: close\r\n" +
		"\r\n")
	write_bytes_close(conn, res)
}

func Handle_event(ev Event) {

	if ev.Lis_info.LisType == 2 {
		Proxy_event_tcp(ev.Conn, ev.Lis_info.Data[0].Proxy_addr)
		return
	}

	ev.Req_ = request.Req_init()
	byte_row, str_row := read_data(ev)
	if byte_row == nil { // client closed
		goto next
	} else {
		ev.Req_.Http_parse(str_row)
	}

	message.PrintInfo("Events ", ev.Conn.RemoteAddr(), " "+ev.Req_.Method, " "+ev.Req_.Path)

	for _, d := range ev.Lis_info.Data {
		switch d.Proxy {
		case 0:
			if ev.Req_.Host == d.ServerName && strings.HasPrefix(ev.Req_.Path, d.Path) {
				row_file_path := ev.Req_.Path[len(d.Path):]

				if d.Path != "/" {
					if row_file_path == "" {
						_event_301(ev.Conn, d.Path+"/")
						goto next
					}
					Static_event(d, d.StaticRoot+row_file_path, ev, ev.Req_)
					goto next
				} else {
					Static_event(d, d.StaticRoot+ev.Req_.Path, ev, ev.Req_)
					goto next
				}
			}
		case 1, 2:
			if ev.Req_.Host == d.ServerName && strings.HasPrefix(ev.Req_.Path, d.Path) {

				res, err := Proxy_event(byte_row, d.Proxy_addr)
				if err == 1 {
					write_bytes_close(ev.Conn, response.Default_server_error)
					goto next
				}
				write_bytes_close(ev.Conn, res)
				goto next
			}
		}
	}
	write_bytes_close(ev.Conn, response.Default_not_found)

next:
}

func read_data(ev Event) ([]byte, string) {
	buffer := make([]byte, 1024)
	n, err := ev.Conn.Read(buffer)
	if err != nil {
		if err == io.EOF {
			message.PrintInfo(ev.Conn.RemoteAddr(), " closed")
			return nil, ""
		}
		message.PrintErr("Error reading from client:", err)
	}
	str_row := string(buffer[:n])
	return buffer, str_row
}

// handle row bytes
func write_bytes_close(conn net.Conn, res []byte) {
	_, err := conn.Write(res)
	if err != nil {
		message.PrintErr("Error writing to client:", err)
	}
	err = conn.Close()
	if err != nil {
		message.PrintErr("Error Close:", err)
	}
}

func Write_bytes(conn net.Conn, res []byte) {

	_, err := conn.Write(res)
	if err != nil {
		message.PrintErr("Error writing to client:", err)
	}
}
