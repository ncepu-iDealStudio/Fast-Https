package events

import (
	"bufio"
	"log"
	"net"
)

type Events interface {
	Start()
	Stop()
	Test()
}

func HandleEvent(conn net.Conn) {

	Handle_read(conn)
	Handle_write(conn)

	defer conn.Close()
}

func Handle_read(conn net.Conn) {

	// 读取客户端发送的数据
	reader := bufio.NewReader(conn)
	// message, err := reader.ReadString('\n')
	message, err := reader.ReadString('\n')

	if err != nil {
		// log.Println("Error reading from client:", err)
		return
	}

	log.Printf("Received message from client: %s\n", message)

}

func Handle_write(conn net.Conn) {
	data := "HTTP/1.1 200 OK\r\nConnection: keep-alive\r\n\r\nHello World"
	write_buf := []byte(data)

	_, err := conn.Write(write_buf)
	if err != nil {
		// log.Println("Error writing to client:", err)
		return
	}

}
