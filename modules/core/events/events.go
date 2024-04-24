package events

import (
	"fast-https/config"
	"fast-https/modules/auth"
	"fast-https/modules/core"
	"fast-https/modules/core/filters"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/safe"
	"fast-https/utils/message"
	"net"
	"strings"
	"time"
)

func HandleEvent(l *listener.Listener, conn net.Conn, shutdown *core.ServerControl) {
	ev := core.NewEvent(l, conn)

	fif := filters.NewFilter() // Filter interface
	// ev.EventWrite = core.EventWriteEarly
	ev.EventWrite = EventWrite

	for !ev.IsClose {
		// websocket and tcp proxy through this
		if fif.Fif.ListenFilter(ev) {
			break
		}

		if parseRequest(ev, fif) != 1 { // TODO: handle different cases...
			ev.Close()
			break // client close
		}

		EventHandler(ev, fif)

		if !ev.EventReuse() {
			break
		}

		if shutdown.Shutdown {
			message.PrintInfo("server shut down")
			break
		}
	}
}

// distribute event
func EventHandler(ev *core.Event, fif *filters.Filter) {

	cfg, ok := fif.Fif.RequestFilter(ev)
	if !ok {
		// core.Log(&ev.Log, ev, "")
		ev.RR.Res_ = response.DefaultNotFound()
		ev.WriteResponseClose(nil)
		return
	}
	// found specific "servername && url"

	cl := safe.Gcl[cfg.ID]
	ip := ""
	index := strings.LastIndex(ev.Conn.RemoteAddr().String(), ":")
	// 如果找到了该字符
	if index != -1 {
		// 截取字符串，不包括该字符及其后面的字符
		ip = ev.Conn.RemoteAddr().String()[:index]
	}
	if !cl.Insert(ip) {
		safe.CountHandler(ev.RR)
		return
	}

	if !auth.AuthHandler(cfg, ev) {
		return
	}
	ev.Type = cfg.Type
	// according to user's confgure and requets endporint handle events
	ev.RR.CircleHandler.RRHandler = core.GRRCHT[cfg.Type].RRHandler
	ev.RR.CircleHandler.FilterHandler = core.GRRCHT[cfg.Type].FilterHandler
	ev.RR.CircleHandler.ParseCommandHandler = core.GRRCHT[cfg.Type].ParseCommandHandler
	ev.RR.CircleHandler.ParseCommandHandler(cfg, ev)
	if !ev.RR.CircleHandler.FilterHandler(cfg, ev) {
		return
	}
	ev.RR.CircleHandler.RRHandler(cfg, ev)
}

func parseRequest(ev *core.Event, fif *filters.Filter) int {
	// read data (bytes and str) from socket
	byte_row := ev.ReadRequest()
	// save request information to ev.RR.Req_
	if !ev.RR.CircleInit {
		ev.RR.Req_ = request.ReqInit(false)  // Create a request Object
		ev.RR.Res_ = response.ResponseInit() // Create a res Object
		ev.RR.CircleInit = true
	}
	// fmt.Printf("%p, %p", ev.RR.Req_, ev)
	if byte_row == nil { // client closed
		return 0
	}

	header_read_num := len(byte_row)
	// headerOtherData := make([]byte, core.READ_HEADER_BUF_LEN)
	for {
		parse := ev.RR.Req_.ParseHeader(byte_row)
		if parse == request.RequestOk {
			break
		} else if parse == request.RequestNeedReadMore { // parse successed !

			ev.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			datasize, err := ev.Conn.Read(ev.RR.ReqBuf)
			if err != nil { // read error, like time out
				message.PrintWarn("read header time out", parse)
				break
			}
			byte_row = append(byte_row, ev.RR.ReqBuf[:datasize]...)
			header_read_num += datasize
			if header_read_num > config.GConfig.Limit.MaxHeaderSize {
				// header bytes beyond config
				break
			}

		} else {
			message.PrintWarn("invalide request", -200)
			return -200 // invade request
		}
	}

	if !fif.Fif.HttpParseFilter(&ev.RR) {
		return -300
	}

	// parse host
	ev.RR.Req_.ParseHost(*ev.LisInfo)

	// headerOtherData := make([]byte, core.READ_BODY_BUF_LEN)
	for {
		ev.RR.Req_.ParseBody(byte_row)
		if ev.RR.Req_.RequestBodyValid() {
			break
		} else {
			datasize, err := ev.Conn.Read(ev.RR.ReqBuf)
			if err != nil { // read error, like time out
				message.PrintWarn("read body time out")
				break
			}
			byte_row = append(byte_row, ev.RR.ReqBuf[:datasize]...)
			ev.RR.Req_.TryFixBody(ev.RR.ReqBuf[:datasize])
			if len(ev.RR.Req_.Body) > config.GConfig.Limit.MaxBodySize {
				// body bytes beyond config
				break
			}
		}
	}

	return 1
}

func EventWrite(ev *core.Event, _data []byte) error {
	//fmt.Printf("%p", ev)
	ev.Conn.SetWriteDeadline(time.Now().Add(time.Second * 30))
	data := ev.RR.Res_.GenerateResponse()
	// data := []byte("HTTP/1.1 200 OK\r\nContent-Length: 11\r\n\r\nhello world")
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
