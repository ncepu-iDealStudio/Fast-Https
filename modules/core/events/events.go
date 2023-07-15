package events

import (
	"fast-https/modules/core/listener"
	httpparse "fast-https/utils/HttpParse"
	"log"
	"net"
	"strings"
)

type Events interface {
}

// {Listen:8080
// 	ServerName:apple.ideal.com
// 	Static:{Root:/var/html Index:[index.html index.htm]}
// 	Path:/
// 	Ssl:
// 	Ssl_Key:
// 	Gzip:0
// 	PROXY_TYPE:0
// 	PROXY_DATA:
// }

func HandleEvent(conn net.Conn, lis_info listener.ListenInfo) {
	if lis_info.LisType == 2 {
		ProxyEventTCP(conn, lis_info.Data[0].Proxy_addr)
		return
	}

	byte_row, str_row := read_data(conn)

	for _, item := range lis_info.Data {
		switch item.Proxy {
		case 0:
			req, err := httpparse.HttpParse2(str_row)
			if err == 10 {
				goto next
			}
			// fmt.Println(req.Host, req.Path, item.ServerName)
			if req.Host == item.ServerName {
				if strings.HasPrefix(req.Path, item.Path) {
					if item.Path == "/" {
						res := StaticEvent(item, item.StaticRoot+req.Path)
						write_bytes_close(conn, res)
					} else {
						res := StaticEvent(item, item.StaticRoot+req.Path[len(item.Path):])
						write_bytes_close(conn, res)
					}
					goto next
				}
			}
		case 1, 2:
			req, err := httpparse.HttpParse2(str_row)
			if err == 10 {
				goto next
			}
			if req.Host == item.ServerName {
				if strings.HasPrefix(req.Path, item.Path) {
					res, err := ProxyEvent(byte_row, item.Proxy_addr)
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
	write_bytes_close(conn, []byte("HTTP/1.1 404 \r\n\r\nNOTFOUNT1"))

next:
}

func read_data(conn net.Conn) ([]byte, string) {
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		log.Println("Error reading from client:------", err)
	}
	str_row := string(buffer[:n])
	return buffer, str_row
}

// handle row bytes
func write_bytes_close(conn net.Conn, res []byte) {
	_, err := conn.Write(res)
	if err != nil {
		log.Println("Error writing to client:", err)
	}
	conn.Close()
}

func Write_bytes(conn net.Conn, res []byte) {

	_, err := conn.Write(res)
	if err != nil {
		log.Println("Error writing to client:", err)
	}
}
