package events

import (
	"fast-https/modules/core/listener"
	httpparse "fast-https/utils/HttpParse"
	"fast-https/utils/message"
	"io"
	"net"
	"strings"
)

type Events interface {
}

func Handle_event(conn net.Conn, lis_info listener.ListenInfo) {

	if lis_info.LisType == 2 {
		Proxy_event_tcp(conn, lis_info.Data[0].Proxy_addr)
		return
	}
	var req httpparse.Req

	byte_row, str_row := read_data(conn)
	if byte_row == nil { // client closed
		goto next
	} else {
		req, _ = httpparse.HttpParse2(str_row)
	}

	message.PrintInfo("Events ", conn.RemoteAddr(), " "+req.Method, " "+req.Path)

	for _, item := range lis_info.Data {
		switch item.Proxy {
		case 0:
			if req.Host == item.ServerName {
				if strings.HasPrefix(req.Path, item.Path) { // path
					if item.Path != "/" {
						row_file_path := req.Path[len(item.Path):]
						if row_file_path == "" {
							row_file_path = "/"
						}
						res := StaticEvent(item, item.StaticRoot+row_file_path)
						write_bytes_close(conn, res)
						goto next
					} else {
						res := StaticEvent(item, item.StaticRoot+req.Path)
						write_bytes_close(conn, res)
						goto next
					}
				}
			}
		case 1, 2:
			if req.Host == item.ServerName {
				if strings.HasPrefix(req.Path, item.Path) {
					res, err := Proxy_event(byte_row, item.Proxy_addr)
					if err == 1 {
						write_bytes_close(conn, []byte("HTTP/1.1 500 \r\n\r\nSERVER ERROR"))
						goto next
					}
					write_bytes_close(conn, res)
					goto next
				}
			}
		}
	}
	write_bytes_close(conn, []byte("HTTP/1.1 404 \r\n\r\nNOTFOUND[event:63]"))

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
