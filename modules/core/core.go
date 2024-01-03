package core

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/core/timer"
	"fmt"
	"io"
	"net"
)

// request and response circle
type RRcircle struct {
	Req_ *request.Req
	Res_ *response.Response

	CircleInit bool
	IsCircle   bool // default is true
	CircleNum  int
	// uri after re
	OriginPath   string
	PathLocation []int
	ProxyConn    net.Conn

	CircleHandler RRcircleHandler
	Ev            *Event
}

// callback item
type RRcircleHandler struct {
	FliterHandler func(listener.ListenCfg, *Event) bool
	RRHandler     func(listener.ListenCfg, *Event)
}

// global RRcircle Handler Table
// I think array is the best struct to
// store these handlers ...
var GRRCHT [10]RRcircleHandler

// each request event is saved in this struct
type Event struct {
	Conn     net.Conn
	Lis_info listener.Listener
	Timer    *timer.Timer
	Log      string
	Type     uint64
	RR       RRcircle
	Reuse    bool
	IsClose  bool
}

func (ev *Event) EventReuse() bool {
	return ev.Reuse
}

func RRHandlerRegister(Type int, fliter func(listener.ListenCfg, *Event) bool,
	handler func(listener.ListenCfg, *Event)) {
	GRRCHT[Type].FliterHandler = fliter
	GRRCHT[Type].RRHandler = handler
}

func NewEvent(l listener.Listener, conn net.Conn) *Event {
	return &Event{
		Conn:     conn,
		Lis_info: l,
		Timer:    nil,
	}
}

func (ev *Event) Log_append(log string) {
	ev.Log = ev.Log + log
}

func (ev *Event) Log_clear() {
	ev.Log = ""
}

// read data from EventFd
// attention: row str only can be used when parse FirstLine or Headers
// because request body maybe contaions '\0'
func (ev *Event) ReadData() ([]byte, string) {
	buffer := make([]byte, 1024*4)
	n, err := ev.Conn.Read(buffer)
	if err != nil {
		if err == io.EOF || n == 0 { // read None, remoteAddr is closed
			// message.PrintInfo(ev.Conn.RemoteAddr(), " closed")
			return nil, ""
		}
		// opErr := (*net.OpError)(unsafe.Pointer(reflect.ValueOf(err).Pointer()))
		// if opErr.Err.Error() == "i/o timeout" {
		// 	message.PrintWarn("read timeout")
		// 	return nil, ""
		// }
		fmt.Println("Error reading from client 176:", err)
		return nil, ""
	}
	str_row := string(buffer[:n])
	// buffer = buffer[:n]
	return buffer, str_row // return row str or bytes
}

func (ev *Event) WriteData(data []byte) {
	for len(data) > 0 {
		n, err := ev.Conn.Write(data)
		if err != nil {
			// opErr := (*net.OpError)(unsafe.Pointer(reflect.ValueOf(err).Pointer()))
			// if opErr.Err.Error() == "i/o timeout" {
			// 	message.PrintWarn("write timeout")
			// 	return
			// }
			fmt.Println("Error writing to client 46:", err)
			return
		}
		data = data[n:]
	}
}

// only close the connection
func (ev *Event) Close() {
	err := ev.Conn.Close()
	if err != nil {
		fmt.Println("Error Close 57:", err)
	}
	ev.IsClose = true
}

func (ev *Event) WriteDataClose(data []byte) {
	ev.WriteData(data)
	ev.Close()
}
