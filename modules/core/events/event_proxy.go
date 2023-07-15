package events

import (
	"fmt"
	"log"
	"net"
)

func get_data_from_server(proxyaddr string, data []byte) ([]byte, int) {

	conn, err := net.Dial("tcp", proxyaddr)
	if err != nil {

		log.Println("Can't connect to "+proxyaddr, err.Error())
		return nil, 1 // no server
	}
	_, err = conn.Write(data)
	if err != nil {
		conn.Close()
		log.Fatal("Proxy Write error")
	}

	buffer := make([]byte, 1024*512)
	n, err := conn.Read(buffer)
	if err != nil {
		conn.Close()
		fmt.Println("Proxy Read error", err)
	}
	conn.Close()

	return buffer[:n], 0 // no error
}

func ProxyEvent(req_data []byte, proxyaddr string) ([]byte, int) {

	return get_data_from_server(proxyaddr, req_data)
}

func ProxyEventTCP(conn net.Conn, proxyaddr string) {

	conn2, err := net.Dial("tcp", proxyaddr)
	if err != nil {
		log.Fatal("Can't connect to "+proxyaddr, err.Error())
	}
	buffer := make([]byte, 1024)

	for {

		n, err := conn.Read(buffer)
		if err != nil {
			if n == 0 {
				conn2.Close()
				conn.Close()
				break
			}
		}

		_, err = conn2.Write(buffer[:n])
		if err != nil {
			conn2.Close()
			log.Fatal("Proxy Write error")
		}

	}

}
