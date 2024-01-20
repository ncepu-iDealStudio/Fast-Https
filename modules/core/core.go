package core

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/core/timer"
	"fast-https/utils/message"
	"io"
	"net"
	"strings"
	"time"
)

const (
	READ_BUF_LEN      = 2048
	READ_BODY_BUF_LEN = 4096
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
	ParseCommandHandler func(listener.ListenCfg, *Event)
	FliterHandler       func(listener.ListenCfg, *Event) bool
	RRHandler           func(listener.ListenCfg, *Event)
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
	handler func(listener.ListenCfg, *Event), cmd func(listener.ListenCfg, *Event)) {
	GRRCHT[Type].FliterHandler = fliter
	GRRCHT[Type].RRHandler = handler
	if cmd != nil {
		GRRCHT[Type].ParseCommandHandler = cmd
	} else {
		GRRCHT[Type].ParseCommandHandler = DefaultParseCommandHandler
	}
}

func DefaultParseCommandHandler(cfg listener.ListenCfg, ev *Event) {
	ip := ""
	index := strings.LastIndex(ev.Conn.RemoteAddr().String(), ":")
	// 如果找到了该字符
	if index != -1 {
		// 截取字符串，不包括该字符及其后面的字符
		ip = ev.Conn.RemoteAddr().String()[:index]
	}

	xForWardFor := ev.RR.Req_.GetHeader("X-Forwarded-For")
	if xForWardFor == "" {
		xForWardFor = ip
	} else {
		xForWardFor = xForWardFor + ", " + ip
	}

	ev.RR.CircleCommandVal.Map = map[string]string{
		"request_method":            ev.RR.Req_.Method,
		"request_uri":               ev.RR.Req_.Path,
		"host":                      ev.RR.Req_.GetHeader("Host"),
		"proxy_host":                cfg.Proxy_addr,
		"remote_addr":               ip,
		"proxy_add_x_forwarded_for": xForWardFor,
	}
}

func (ev *Event) GetCommandParsedStr(inputString string) string {
	out := inputString
	for key, value := range ev.RR.CircleCommandVal.Map {
		out = strings.Replace(out, "$"+key, value, -1) // 只替换第一次出现的关键词
	}
	return out
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
			message.PrintWarn("Warn --core read timeout")
			return nil, ""
		} else { // other error can not handle temporarily
			message.PrintErr("Error --core reading from client", err.Error())
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
				message.PrintWarn("Warn  --core write timeout")
				return err
			} else { // other error can not handle temporarily
				message.PrintWarn("Error --core writing to client ", err)
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
		message.PrintWarn("Warn --core repeat close")
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
