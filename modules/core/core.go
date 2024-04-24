package core

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/core/timer"
	"fast-https/utils/message"
	"io"
	"net"
	"time"
)

const (
	READ_HEADER_BUF_LEN = 2048
	READ_BODY_BUF_LEN   = 4096
)

// request and response circle
type RRcircle struct {
	Req_   *request.Req
	ReqBuf []byte
	Res_   *response.Response
	ResBuf []byte

	CircleInit bool
	IsCircle   bool // default is true
	CircleNum  int
	// uri after re
	OriginPath    string
	PathLocation  []int
	ProxyConnInit bool

	CircleHandler    RRcircleHandler
	CircleCommandVal RRcircleCommandVal
	Ev               *Event
	CircleData       interface{}
}

type RRcircleCommandVal struct {
	Map map[string]string
}

// callback item
type RRcircleHandler struct {
	ParseCommandHandler func(*listener.ListenCfg, *Event)
	FilterHandler       func(*listener.ListenCfg, *Event) bool
	RRHandler           func(*listener.ListenCfg, *Event)
}

// global RRcircle Handler Table
// I think array is the best struct to
// store these handlers ...
var GRRCHT [10]RRcircleHandler

// each request event is saved in this struct
type Event struct {
	// this server is multi-platform,
	// crypto/tls package uses the same interfaces
	// so we use net.Conn
	Conn    net.Conn
	Stream  interface{}
	LisInfo *listener.Listener
	Timer   *timer.Timer
	Log     Logger
	Type    uint16
	Upgrade string
	RR      RRcircle
	Reuse   bool

	EventWrite func(*Event, []byte) error
	IsClose    bool
	ReadReady  bool
	WriteReady bool
}

func (ev *Event) EventReuse() bool { return ev.Reuse }

func RRHandlerRegister(Type int, filter func(*listener.ListenCfg, *Event) bool,
	handler func(*listener.ListenCfg, *Event), cmd func(*listener.ListenCfg, *Event)) {
	GRRCHT[Type].FilterHandler = filter
	GRRCHT[Type].RRHandler = handler
	if cmd != nil {
		GRRCHT[Type].ParseCommandHandler = cmd
	} else {
		GRRCHT[Type].ParseCommandHandler = DefaultParseCommandHandler
	}
}

func NewEvent(l *listener.Listener, conn net.Conn) *Event {
	each_event := Event{
		Conn:    conn,
		LisInfo: l,
		Timer:   nil,
		Reuse:   false,
		Log:     *NewLogger(),

		IsClose:    false, // not close
		ReadReady:  true,  // need read
		WriteReady: false, // needn't write
		RR: RRcircle{
			Ev:            nil, // include each other
			IsCircle:      true,
			CircleInit:    false,
			ProxyConnInit: false,
			CircleCommandVal: RRcircleCommandVal{
				Map: make(map[string]string), // init CircleCommandVal map
			},
		},
	}
	each_event.RR.Ev = &each_event

	return &each_event
}

func (ev *Event) CheckIfTimeOut(err error) bool {
	// opErr := (*net.OpError)(unsafe.Pointer(reflect.ValueOf(err).Pointer()))
	// if opErr.Err.Error() == "i/o timeout" {
	// 	return true
	// } else {
	// 	return false
	// }
	if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
		return true
	} else {
		return false
	}
}

// read data from EventFd
// attention: row str only can be used when parse FirstLine or Headers
// because request body maybe contaions '\0'
// only for HTTP/1.1
func (ev *Event) ReadRequest() []byte {
	now := time.Now()
	ev.Conn.SetReadDeadline(now.Add(time.Second * 30))
	ev.RR.ReqBuf = make([]byte, READ_HEADER_BUF_LEN)
	buffer := ev.RR.ReqBuf
	n, err := ev.Conn.Read(buffer)
	if err != nil {
		if err == io.EOF { // read None, remoteAddr is closed
			message.PrintInfo(ev.Conn.RemoteAddr(), " closed")
			return nil
		}
		if ev.CheckIfTimeOut(err) {
			message.PrintWarn("Warn --core " + ev.Conn.RemoteAddr().String() + " read timeout")
			return nil
		} else { // other error can not handle temporarily
			message.PrintWarn("Error --core "+ev.Conn.RemoteAddr().String()+" reading from client", err.Error())
		}
		return nil
	}

	buffer = buffer[:n]
	return buffer // return row str or bytes
}

func (ev *Event) WriteResponse(data []byte) error {
	return ev.EventWrite(ev, data)
}

// only close the connection
// only for HTTP/1.1
func (ev *Event) Close() {
	if !ev.IsClose {
		err := ev.Conn.Close()
		if err != nil {
			message.PrintErr("Error --core Close ", err)
		}
	} else {
		message.PrintWarn("Warn --core repeat close ")
	}
	ev.IsClose = true
}

func (ev *Event) WriteResponseClose(data []byte) {
	ev.WriteResponse(data)
	if !ev.RR.Req_.H2 { // TODO: impove this
		ev.Close()
	}
}

func EventWriteEarly(ev *Event, _data []byte) error {
	//fmt.Printf("%p", ev)
	ev.Conn.SetWriteDeadline(time.Now().Add(time.Second * 30))
	data := ev.RR.Res_.GenerateResponse()
	// data := []byte("HTTP/1.1 200 OK\r\n\r\nhello world")
	for len(data) > 0 {
		n, err := ev.Conn.Write(data)
		if err != nil {
			if ev.CheckIfTimeOut(err) {
				message.PrintWarn("Warn  --core " + ev.Conn.RemoteAddr().String() + " write timeout")
				return err
			} else { // other error can not handle temporarily
				message.PrintWarn("Error --core "+ev.Conn.RemoteAddr().String()+" writing to client ", err.Error())
				return err
			}
		}
		data = data[n:]
	}
	return nil
}

type ServerControl struct {
	Shutdown bool
}
