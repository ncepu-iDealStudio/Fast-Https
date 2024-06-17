package engine

import (
	"encoding/gob"
	"fmt"
	"net"
)

func MasterInit() {
	listener, err := net.Listen("tcp", RegisterAddr)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	defer listener.Close()
	fmt.Println("Server is listening: ", RegisterAddr)

	gob.Register(EngineMessage{})

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		var msg EngineMessage
		decoder := gob.NewDecoder(conn)
		err = decoder.Decode(&msg)
		if err != nil {
			fmt.Println("Error decoding message:", err)
			continue
		}

		GMapLock.Lock()
		GMessageMap.Inner[msg.Id] = msg
		GUpdate += 1 // 需要更新Map
		GMapLock.Unlock()

		ShowEngineList()
		go DoHeartBeat(msg, conn) // 检测这个engine是否活跃
	}
}
