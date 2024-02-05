package events

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/fliters"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/safe"
	"fast-https/utils/message"
	"strings"
	"time"
)

func HandleEvent(ev *core.Event, fif *fliters.Fliter, shutdown *core.ServerControl) {
	fif.Fif.ListenFliter(ev)

	for !ev.IsClose {
		EventHandler(ev, fif)

		if !ev.EventReuse() {
			break
		}

		if shutdown.Shutdown {
			message.PrintInfo("server shut down")
			return
		}
	}
}

// distribute event
func EventHandler(ev *core.Event, fif *fliters.Fliter) {

	if processRequest(ev, fif) != 1 { // TODO: handle different cases...
		ev.Close()
		return // client close
	}
	ev.Log_append(" " + ev.RR.Req_.Method)
	ev.Log_append(" " + ev.RR.Req_.Path + " \"" +
		ev.RR.Req_.GetHeader("Host") + "\"")

	cfg, ok := fif.Fif.RequestFliter(ev)
	if !ok {
		message.PrintAccess(ev.Conn.RemoteAddr().String(),
			"INFORMAL Event(404)"+ev.Log,
			"\""+ev.RR.Req_.Headers["User-Agent"]+"\"")
		ev.WriteDataClose(response.DefaultNotFound())
	} else {

		cl := safe.Gcl[cfg.ID]

		if !cl.Insert(strings.Split(ev.Conn.RemoteAddr().String(), ":")[0]) {
			safe.CountHandler(ev.RR)
			return
		}

		// according to user's confgure and requets endporint handle events
		ev.RR.CircleHandler.RRHandler = core.GRRCHT[cfg.Type].RRHandler
		ev.RR.CircleHandler.FliterHandler = core.GRRCHT[cfg.Type].FliterHandler
		ev.RR.CircleHandler.ParseCommandHandler = core.GRRCHT[cfg.Type].ParseCommandHandler
		ev.RR.CircleHandler.ParseCommandHandler(cfg, ev)
		if !ev.RR.CircleHandler.FliterHandler(cfg, ev) {
			return
		}
		ev.RR.CircleHandler.RRHandler(cfg, ev)
	}
}

func processRequest(ev *core.Event, fif *fliters.Fliter) int {
	// read data (bytes and str) from socket
	byte_row := ev.ReadData()
	// save requte information to ev.RR.Req_
	if !ev.RR.CircleInit {
		ev.RR.Req_ = request.ReqInit()       // Create a request Object
		ev.RR.Res_ = response.ResponseInit() // Create a res Object
		ev.RR.CircleInit = true
	}
	// fmt.Printf("%p, %p", ev.RR.Req_, ev)
	if byte_row == nil { // client closed
		return 0
	}

	header_read_num := len(byte_row)
	headerOtherData := make([]byte, core.READ_HEADER_BUF_LEN)
	for {
		parse := ev.RR.Req_.ParseHeader(byte_row)
		if parse == request.REQUEST_OK {
			break
		} else if parse == request.REQUEST_NEED_READ_MORE { // parse successed !

			ev.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			datasize, err := ev.Conn.Read(headerOtherData)
			if err != nil { // read error, like time out
				message.PrintWarn("read header time out", parse)
				break
			}
			byte_row = append(byte_row, headerOtherData[:datasize]...)
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

	if !fif.Fif.HttpParseFliter(&ev.RR) {
		return -300
	}

	// parse host
	ev.RR.Req_.ParseHost(ev.LisInfo)

	otherData := make([]byte, core.READ_BODY_BUF_LEN)
	for {
		ev.RR.Req_.ParseBody(byte_row)
		if ev.RR.Req_.RequestBodyValid() {
			break
		} else {
			datasize, err := ev.Conn.Read(otherData)
			if err != nil { // read error, like time out
				message.PrintWarn("read body time out")
				break
			}
			byte_row = append(byte_row, otherData[:datasize]...)
			ev.RR.Req_.TryFixBody(otherData[:datasize])
			if len(ev.RR.Req_.Body) > config.GConfig.Limit.MaxBodySize {
				// body bytes beyond config
				break
			}
		}
	}

	return 1
}
