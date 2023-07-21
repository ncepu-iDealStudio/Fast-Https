package events

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/timer"
	"fast-https/utils/message"
	"io"
	"net"
	"strings"
)

type Event struct {
	Conn     net.Conn
	Lis_info listener.ListenInfo
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
	var req request.Req

	byte_row, str_row := read_data(ev.Conn)
	if byte_row == nil { // client closed
		goto next
	} else {
		req, _ = request.Http_parse(str_row)
	}

	message.PrintInfo("Events ", ev.Conn.RemoteAddr(), " "+req.Method, " "+req.Path)

	for _, item := range ev.Lis_info.Data {
		switch item.Proxy {
		case 0:
			if req.Host == item.ServerName && strings.HasPrefix(req.Path, item.Path) {

				if item.Path != "/" {
					row_file_path := req.Path[len(item.Path):]
					if row_file_path == "" {
						_event_301(ev.Conn, item.Path+"/")
						goto next
					}
					res := StaticEvent(item, item.StaticRoot+row_file_path)
					write_bytes_close(ev.Conn, res)
					goto next
				} else {
					res := StaticEvent(item, item.StaticRoot+req.Path)
					write_bytes_close(ev.Conn, res)
					goto next
				}
			}
		case 1, 2:
			if req.Host == item.ServerName && strings.HasPrefix(req.Path, item.Path) {

				res, err := Proxy_event(byte_row, item.Proxy_addr)
				if err == 1 {
					write_bytes_close(ev.Conn, []byte("HTTP/1.1 500 \r\n\r\nSERVER ERROR"))
					goto next
				}
				write_bytes_close(ev.Conn, res)
				goto next
			}
		}
	}
	write_bytes_close(ev.Conn, []byte("HTTP/1.1 404 \r\n\r\nNOTFOUND[event:63]"))

next:
}

func read_data(conn net.Conn) ([]byte, string) {
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		if err == io.EOF {
			message.PrintInfo(conn.RemoteAddr(), " closed")
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
	if conn.Close() != nil {
		message.PrintErr("Error Close:", err)
	}
}

func Write_bytes(conn net.Conn, res []byte) {

	_, err := conn.Write(res)
	if err != nil {
		message.PrintErr("Error writing to client:", err)
	}
}
