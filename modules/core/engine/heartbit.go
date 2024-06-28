package engine

import (
	"encoding/gob"
	"fmt"
	"net"
	"time"
)

// master向slave发送心跳包，slave只需返回ok
// 若有 GMessageMap 的更新心跳包就会带着最新的 Map

func HeartBeatInit() {

}

func DoHeartBeat(slave EngineMessage, conn net.Conn) {
	fmt.Println("[engine]: DoHeartBeat check slave ", slave.Id)
	gob.Register(GMessageMapContainer{})

	for {
		time.Sleep(time.Second * 3)
		GUpdateLock.Lock()
		update := GUpdate
		GUpdateLock.Unlock()

		if update > 0 {
			encoder := gob.NewEncoder(conn)
			err := encoder.Encode(&GMessageMap)
			if err != nil {
				fmt.Println("[heartbeat]write update error", err)
			}
			GUpdateLock.Lock()
			GUpdate -= 1
			GUpdateLock.Unlock()
		} else {
			encoder := gob.NewEncoder(conn)
			err := encoder.Encode(&GMessageMap)
			if err != nil { // 发送心跳包失败 有slave掉线 update 要加1
				fmt.Println("[heartbeat]write error")
				GUpdateLock.Lock()
				GUpdate += 1
				GUpdateLock.Unlock()

				GMessageMap.Inner[slave.Id] = EngineMessage{}
				ShowEngineList()
				conn.Close()
				break
			}
		}

		res := make([]byte, 10)
		n, err := conn.Read(res)
		if n != 2 || err != nil { // 接受心跳包失败 有slave掉线 update 要加1
			fmt.Println("[heartbeat]read error")
			GUpdateLock.Lock()
			GUpdate += 1
			GUpdateLock.Unlock()

			GMessageMap.Inner[slave.Id] = EngineMessage{}
			ShowEngineList()
			conn.Close()
			break
		}

	}
}
