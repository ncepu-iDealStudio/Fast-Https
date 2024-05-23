package engine

import (
	"encoding/gob"
	"fast-https/config"
	"fmt"
	"net"
)

func SlaveInit() {
	gob.Register(EngineMessage{})

	local_engine := EngineMessage{
		IsMaster: false,
		Id:       config.GConfig.ServerEngine.Id,
		AddrInfo: Addr{
			Ip:   config.GConfig.ServerEngine.SlaveIp,
			Port: config.GConfig.ServerEngine.SlavePort,
		},
	}

	conn, err := net.Dial("tcp", RegisterAddr)
	if err != nil {
		fmt.Println("[engine-slave]: Error connecting to server:")
		return
	}

	encoder := gob.NewEncoder(conn)
	err = encoder.Encode(&local_engine)
	if err != nil {
		fmt.Println("[engine-slave]: Error encoding message:", err)
		return
	}

	handleHeartBeat(conn)
}

func handleHeartBeat(conn net.Conn) {
	gob.Register(GMessageMapContainer{})

	for {
		newContainer := GMessageMapContainer{}
		decoder := gob.NewDecoder(conn)
		err := decoder.Decode(&newContainer)
		if err != nil {
			fmt.Println("[engine-slave]: Error decoding message:", err)
			break
		}
		show := false
		if GMessageMap != newContainer {
			show = true
		}
		GMessageMap = newContainer
		if show {
			ShowEngineList()
		}

		//fmt.Println(GMessageMap)
		fmt.Println("[engine-slave]: reply heart beat ok")
		n, err := conn.Write([]byte("ok"))
		if n != 2 || err != nil {
			fmt.Println("[engine-slave]: Error reply heart beat:", err)
			break
		}

	}

	conn.Close()
}
