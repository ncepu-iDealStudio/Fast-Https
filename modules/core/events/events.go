package events

import (
	"fast-https/modules/core/listener"
	httpparse "fast-https/utils/HttpParse"
	"log"
	"net"
)

type Events interface {
}

func HandleEvent(conn net.Conn, lis_info listener.ListenInfo) {

	switch lis_info.Proxy {
	case 0:
		_, str_row := read_data(conn)
		req := httpparse.HttpParse2(str_row)
		if req.Path == "/" {
			res := StaticEvent(lis_info.Proxy_addr + req.Path + "index.html")
			write_bytes_close(conn, res)
		} else {
			res := StaticEvent(lis_info.Proxy_addr + req.Path) // Proxy equal to 0, Proxy is static file path
			write_bytes_close(conn, res)
		}
	case 1, 2:
		byte_row, _ := read_data(conn)
		res := ProxyEvent(byte_row, lis_info.Proxy_addr)
		write_bytes_close(conn, res)
	case 3:
		ProxyEventTCP(conn, lis_info.Proxy_addr)
	}

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
