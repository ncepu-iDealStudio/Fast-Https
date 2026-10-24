// Package core 定义所有功能模块共享的请求事件、处理器注册表与响应写出契约。
package core

import (
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"fast-https/infra/logging"
	"io"
	"net"
	"sync"
	"time"
)

const (
	READ_HEADER_BUF_LEN = 4096
	READ_BODY_BUF_LEN   = 4096
)

type RRcircleCommandVal struct {
	Map map[string]string
}

// callback item
type RRcircleHandler struct {
	ParseCommandHandler func(*listener.ListenCfg, *Event)
	FilterHandler       func(*listener.ListenCfg, *Event) bool
	RRHandler           func(*listener.ListenCfg, *Event)
}

// request and response circle
type RRcircle struct {
	Req    *request.Request
	ReqBuf []byte
	Res    *response.Response
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

// each request event is saved in this struct
type Event struct {
	// this server is multi-platform,
	// crypto/tls package uses the same interfaces
	// so we use net.Conn
	Conn    net.Conn
	Stream  interface{}
	LisInfo *listener.Listener
	// Log     Logger
	Type    uint16
	Upgrade string
	RR      RRcircle
	Reuse   bool

	EventWrite func(*Event, []byte) error
	IsClose    bool
	ReadReady  bool
	WriteReady bool
	closeOnce  sync.Once
}

func (ev *Event) EventReuse() bool { return ev.Reuse }

func NewEvent(l *listener.Listener, conn net.Conn) *Event {
	each_event := Event{
		Conn:    conn,
		LisInfo: l,
		Reuse:   false,
		// Log:     *NewLogger(),

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
// only for HTTP/1.1
func (ev *Event) ReadRequest() []byte {
	now := time.Now()
	ev.Conn.SetReadDeadline(now.Add(time.Second * 30))
	header_buffer := make([]byte, READ_HEADER_BUF_LEN)
	// buffer := header_buffer
	n, err := ev.Conn.Read(header_buffer)
	if err != nil {
		if err == io.EOF { // read None, remoteAddr is closed
			logging.PrintInfo(ev.Conn.RemoteAddr(), " closed")
			return nil
		}
		if ev.CheckIfTimeOut(err) {
			logging.PrintWarn("Warn --core " + ev.Conn.RemoteAddr().String() + " read timeout")
			return nil
		} else { // other error can not handle temporarily
			logging.PrintWarn("Error --core "+ev.Conn.RemoteAddr().String()+" reading from client", err.Error())
		}
		return nil
	}

	header_buffer = header_buffer[:n]
	return header_buffer // return row str or bytes
}

func (ev *Event) WriteResponse(data []byte) error {
	return ev.EventWrite(ev, data)
}

// ResponseBytes is the bytes written for this response.
// HEAD keeps the headers, including Content-Length, and omits the body.
func (ev *Event) ResponseBytes() []byte {
	if ev.RR.Req != nil && ev.RR.Req.Method == "HEAD" {
		return ev.RR.Res.GenerateHeaderBytes()
	}
	return ev.RR.Res.GenerateResponse()
}

// only close the connection
// only for HTTP/1.1
// Close closes the connection once. Later calls do nothing and do not log.
func (ev *Event) Close() {
	ev.closeOnce.Do(func() {
		ev.IsClose = true
		if ev.Conn != nil {
			_ = ev.Conn.Close()
		}
	})
}

func (ev *Event) WriteResponseClose(data []byte) {
	ev.WriteResponse(data)
	if !ev.RR.Req.H2 { // TODO: impove this
		ev.Close()
	}
}
