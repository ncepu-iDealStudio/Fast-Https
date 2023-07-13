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

func HandleEvent(conn net.Conn, lis_info listener.ListenInfo) {

	switch lis_info.Proxy {
	case 0:
		str_row := handle_read(conn)

		res := StaticEvent()
		handle_write_close(conn, res)
	case 1, 2:
		res := ProxyEvent()
		handle_write_close(conn, res)
	case 3:

	}

}

func handle_read_parse() {

}

func handle_read(conn net.Conn) string {
	reader := bufio.NewReader(conn)
	message, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading from client:", err)
	}

	log.Printf("Received message from client: %s\n", message)
	return message
}

func handle_write(conn net.Conn, res string) {
	write_buf := []byte(res)
	_, err := conn.Write(write_buf)
	if err != nil {
		// log.Println("Error writing to client:", err)
		return
	}
}

func handle_write_close(conn net.Conn, res string) {

	write_buf := []byte(res)
	_, err := conn.Write(write_buf)
	if err != nil {
		// log.Println("Error writing to client:", err)
		return
	}

	conn.Close()
}
