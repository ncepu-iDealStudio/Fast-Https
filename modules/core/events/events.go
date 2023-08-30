package events

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/core/timer"
	"fmt"
	"io"
	"net"
	"strings"
)

// each request event is saved in this struct
type Event struct {
	Conn      net.Conn
	ProxyConn net.Conn
	Lis_info  listener.ListenInfo
	Req_      *request.Req
	Res_      *response.Response
	Timer     *timer.Timer
	Log       string
}

// distribute event
// LisType(2) tcp proxy
func Handle_event(ev *Event) {

	// handle tcp proxy
	if ev.Lis_info.LisType == 2 {
		Proxy_event_tcp(ev.Conn, ev.Lis_info.Cfg[0].Proxy_addr)
		return
	}

	// read data (bytes and str) from socket
	byte_row, str_row := read_data(*ev)
	// save requte information to ev.Req_
	ev.Req_ = request.Req_init()
	if byte_row == nil || str_row == "" { // client closed
		// fmt.Println("39 client close")
		close(*ev)
		return
	} else {
		ev.Req_.Http_parse(str_row)
		// parse host
		ev.Req_.Parse_host(ev.Lis_info)
	}

	ev.Log = ev.Conn.RemoteAddr().String() + " " + ev.Req_.Method + " " + ev.Req_.Path

	for _, cfg := range ev.Lis_info.Cfg {
		switch cfg.Proxy {
		case 0: // Proxy: 0, static events
			if ev.Req_.Host == cfg.ServerName && strings.HasPrefix(ev.Req_.Path, cfg.Path) {
				row_file_path := ev.Req_.Path[len(cfg.Path):]
				if row_file_path == "" && cfg.Path != "/" {
					// fmt.Println("301")
					_event_301(*ev, cfg.Path+"/")
					return
				}

				// according to user's confgure and requets endporint handle events
				if cfg.Path != "/" {
					Static_event(cfg, cfg.StaticRoot+row_file_path, *ev)
					return
				} else {
					Static_event(cfg, cfg.StaticRoot+ev.Req_.Path, *ev)
					return
				}
			}
		case 1, 2: // proxy: 1 or 2,  proxy events
			if ev.Req_.Host == cfg.ServerName {

				ev.Req_.Set_headers("Host", cfg.Proxy_addr)
				ev.Req_.Flush()

				// according to user's confgure and requets endporint handle events
				res, err := Proxy_event(ev, ev.Req_.Byte_row(), cfg.Proxy_addr, cfg.Proxy)
				if err == 1 { // target no response
					write_bytes_close(*ev, response.Default_server_error())
					return
				}
				if ev.Req_.Connection == "close" {
					write_bytes_close(*ev, res)
					return
				} else {
					_, err := ev.Conn.Write(res)
					if err != nil {
						fmt.Println("Error writing to client89:", err)
						return
					}
					Handle_event(ev)
					return
				}
			}
		}
	}
	write_bytes_close(*ev, response.Default_not_found())
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
		// opErr := (*net.OpError)(unsafe.Pointer(reflect.ValueOf(err).Pointer()))
		// if opErr.Err.Error() == "i/o timeout" {
		// 	message.PrintWarn("write timeout")
		// 	return nil, ""
		// }
		fmt.Println("Error reading from client:", err)
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
			fmt.Println("Error writing to client133:", err)
			return
		}
		data = data[n:]
	}
	err := ev.Conn.Close()
	if err != nil {
		fmt.Println("Error Close:", err)
	}
}

// write row bytes
func write_bytes(ev Event, data []byte) {
	for len(data) > 0 {
		n, err := ev.Conn.Write(data)
		if err != nil {
			// opErr := (*net.OpError)(unsafe.Pointer(reflect.ValueOf(err).Pointer()))
			// if opErr.Err.Error() == "i/o timeout" {
			// 	message.PrintWarn("write timeout")
			// 	return
			// }
			fmt.Println("Error writing to client155:", err)
			return
		}
		data = data[n:]
	}
}

// only close the connection
func close(ev Event) {
	err := ev.Conn.Close()
	if err != nil {
		fmt.Println("Error Close:", err)
	}
}
