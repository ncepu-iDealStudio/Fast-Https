package core

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/core/timer"
	"fast-https/utils/message"
	"io"
	"net"
	"reflect"
	"time"
	"unsafe"
)

const (
	READ_BUF_LEN = 4096
)

// request and response circle
type RRcircle struct {
	Req_ *request.Req
	Res_ *response.Response

	CircleInit bool
	IsCircle   bool // default is true
	CircleNum  int
	// uri after re
	OriginPath    string
	PathLocation  []int
	ProxyConnInit bool

	CircleHandler RRcircleHandler
	Ev            *Event
	CircleData    interface{}
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

	IsClose    bool
	ReadReady  bool
	WriteReady bool
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

func (ev *Event) CheckIfTimeOut(err error) bool {
	opErr := (*net.OpError)(unsafe.Pointer(reflect.ValueOf(err).Pointer()))
	if opErr.Err.Error() == "i/o timeout" {
		return true
	} else {
		return false
	}
}

// read data from EventFd
// attention: row str only can be used when parse FirstLine or Headers
// because request body maybe contaions '\0'
func (ev *Event) ReadData() ([]byte, string) {
	now := time.Now()
	ev.Conn.SetReadDeadline(now.Add(time.Second * 30))
	buffer := make([]byte, READ_BUF_LEN)
	n, err := ev.Conn.Read(buffer)
	if err != nil {
		if err == io.EOF { // read None, remoteAddr is closed
			message.PrintInfo(ev.Conn.RemoteAddr(), " closed")
			return nil, ""
		}
		if ev.CheckIfTimeOut(err) {
			message.PrintWarn("read timeout")
			return nil, ""
		} else { // other error can not handle temporarily
			message.PrintErr("Error --core reading from client", err)
		}
		return nil, ""
	}
	str_row := string(buffer[:n])
	buffer = buffer[:n]
	return buffer, str_row // return row str or bytes
}

func (ev *Event) WriteData(data []byte) error {
	ev.Conn.SetWriteDeadline(time.Now().Add(time.Second * 30))
	for len(data) > 0 {
		n, err := ev.Conn.Write(data)
		if err != nil {
			if ev.CheckIfTimeOut(err) {
				message.PrintWarn("write timeout")
				return err
			} else { // other error can not handle temporarily
				message.PrintErr("Error --core writing to client ", err)
				return err
			}
		}
		data = data[n:]
	}
	return nil
}

// only close the connection
func (ev *Event) Close() {
	if !ev.IsClose {
		err := ev.Conn.Close()
		if err != nil {
			message.PrintErr("Error --core Close", err)
		}
	} else {
		message.PrintWarn("--core repeat close")
	}
	ev.IsClose = true
}

func (ev *Event) WriteDataClose(data []byte) {
	ev.WriteData(data)
	ev.Close()
}

type ServerControl struct {
	Shutdown bool
}
