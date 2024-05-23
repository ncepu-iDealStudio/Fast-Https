package engine

import (
	"fast-https/config"
	"fmt"
	"sync"
	"unsafe"
)

// master 注册中心的ip和端口
const RegisterAddr string = "127.0.0.1:5000"

const EngineArrayMax int = 32

// 由EngineMessage.Id和EngineMessage构成的map
// master和slave都有一份
// master拥有读写权限，slave只有读权限

// 当有新的slave注册时master写map，新增engine slave
// 当 heart bit 失败时master写map，删除engine slave
var GMessageMap GMessageMapContainer

type GMessageMapContainer struct {
	Update bool
	Inner  [EngineArrayMax]EngineMessage
}

// 此map的读写应该是多协程的需要加锁
var GMapLock sync.RWMutex

// 标志当前是master还是slave
var GCurrentIsMaster bool

// 当有新的slave注册或者心跳包发现有slava下线则会设置为true
var GUpdate int
var GUpdateLock sync.RWMutex

type Addr struct {
	Ip   string
	Port int
}

type EngineMessage struct {
	Type     int // 0 add, 1 delete
	IsMaster bool
	Id       int // master is 0
	AddrInfo Addr
}

func ShowEngineList() {
	fmt.Println("========== current engine list ==========")
	for i := 0; i < EngineArrayMax; i++ {
		msg := GMessageMap.Inner[i]
		if msg.AddrInfo.Ip != "" {
			fmt.Println("	id: ", msg.Id, "addr: ", msg.AddrInfo.Ip, ":", msg.AddrInfo.Port)
		}
	}
	fmt.Println("=========================================")
}

func EngineInit() {
	fmt.Printf("sizeof GMessageMapContainer{}: %d\n", unsafe.Sizeof(GMessageMapContainer{}))
	// GMessageMap = make(map[string]EngineMessage)
	GUpdate = 0
	if config.GConfig.ServerEngine.IsMaster {
		GCurrentIsMaster = true
		go MasterInit()
	} else {
		GCurrentIsMaster = false
		SlaveInit()
	}
}
