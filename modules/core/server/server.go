package server

import (
	"fast-https/modules/core/events"
	"log"
	"net"
)

func Run() {
	port := "8080"

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatal("Error starting the server:", err)
	}

	log.Printf("Server listening on port %s\n", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}

		// log.Printf("New client connected: %s\n", conn.RemoteAddr())

		go events.HandleEvent(conn)
	}
}
