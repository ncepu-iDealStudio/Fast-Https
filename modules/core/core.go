package core

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/core/timer"
	"fast-https/utils/message"
	"fmt"
	"net"
	"reflect"
	"unsafe"
)

// request and response circle
type RRcircle struct {
	Req_      *request.Req
	Res_      *response.Response
	CircleNum int
	// uri after re
	OriginPath   string
	PathLocation []int
	ProxyConn    net.Conn
	// Handler
}

// each request event is saved in this struct
type Event struct {
	Conn     net.Conn
	Lis_info listener.Listener
	Timer    *timer.Timer
	Log      string
	Type     uint64
	RR       RRcircle
}

func (ev *Event) Write_bytes(data []byte) {
	for len(data) > 0 {
		n, err := ev.Conn.Write(data)
		if err != nil {
			opErr := (*net.OpError)(unsafe.Pointer(reflect.ValueOf(err).Pointer()))
			if opErr.Err.Error() == "i/o timeout" {
				message.PrintWarn("write timeout")
				return
			}
			fmt.Println("Error writing to client 193:", err)
			return
		}
		data = data[n:]
	}
}

// only close the connection
func (ev *Event) Close() {
	err := ev.Conn.Close()
	if err != nil {
		fmt.Println("Error Close:", err)
	}
}

func (ev *Event) Write_bytes_close(data []byte) {
	ev.Write_bytes(data)
	ev.Close()
}
