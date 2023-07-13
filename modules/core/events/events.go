package events

import (
	"bufio"
	"fast-https/modules/core/listener"
	"log"
	"net"
)

type Events interface {
	Start()
	Stop()
	Test()
}

var data = "HTTP/1.1 200 OK\r\nConnection: keep-alive\r\n\r\nHello World"

func HandleEvent(conn net.Conn, lis_info listener.ListenInfo) {

	Handle_read(conn, lis_info)
	Handle_write(conn, data)
	conn.Close()

}

func Handle_read(conn net.Conn, lis_info listener.ListenInfo) {

	reader := bufio.NewReader(conn)
	message, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading from client:", err)
		return
	}

	log.Printf("Received message from client: %s\n", message)

	switch lis_info.Proxy {
	case 0:

	}

}

func Handle_write(conn net.Conn, res string) {

	write_buf := []byte(res)
	_, err := conn.Write(write_buf)
	if err != nil {
		// log.Println("Error writing to client:", err)
		return
	}

}
