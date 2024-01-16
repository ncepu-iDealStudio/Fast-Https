package events

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/safe"
	"fast-https/utils/message"
	"regexp"
	"strings"
)

func HandleEvent(ev *core.Event, shutdown *core.ServerControl) {
	for !ev.IsClose {
		EventHandler(ev)

		if !ev.EventReuse() {
			break
		}

		if shutdown.Shutdown {
			message.PrintInfo("server shut down")
			ev.Close()
			return
		}
	}
}

// distribute event
// LisType(2) tcp proxy
func EventHandler(ev *core.Event) {
	// handle tcp proxy
	if ev.Lis_info.LisType == 2 {
		ProxyEventTcp(ev.Conn, ev.Lis_info.Cfg[0].Proxy_addr)
		return
	}
	if processRequest(ev) == 0 {
		return // client close
	}
	ev.Log_append(" " + ev.RR.Req_.Method)
	ev.Log_append(" " + ev.RR.Req_.Path + " \"" +
		ev.RR.Req_.GetHeader("Host") + "\"")

	cfg, ok := FliterHostPath(ev)
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
		if !ev.RR.CircleHandler.FliterHandler(cfg, ev) {
			return
		}
		ev.RR.CircleHandler.RRHandler(cfg, ev)
	}
}

func FliterHostPath(ev *core.Event) (listener.ListenCfg, bool) {
	hosts := ev.Lis_info.HostMap[ev.RR.Req_.GetHeader("Host")]
	// fmt.Println(hosts)
	var cfg listener.ListenCfg

	for _, cfg = range hosts {
		re := regexp.MustCompile(cfg.Path) // we can compile this when load config
		res := re.FindStringIndex(ev.RR.Req_.Path)
		if res != nil {
			originPath := ev.RR.Req_.Path[res[1]:]
			ev.RR.OriginPath = originPath
			ev.RR.PathLocation = res

			return cfg, true
		}
	}

	hosts2 := ev.Lis_info.HostMap[config.DEFAULT_PORT]
	for _, cfg = range hosts2 {
		re := regexp.MustCompile(cfg.Path) // we can compile this when load config
		res := re.FindStringIndex(ev.RR.Req_.Path)
		if res != nil {
			originPath := ev.RR.Req_.Path[res[1]:]
			ev.RR.OriginPath = originPath
			ev.RR.PathLocation = res

			return cfg, true
		}
	}

	return cfg, false
}

func processRequest(ev *core.Event) int {
	// read data (bytes and str) from socket
	byte_row, str_row := (ev).ReadData()
	// save requte information to ev.RR.Req_
	if !ev.RR.CircleInit {
		ev.RR.Req_ = request.ReqInit()       // Create a request Object
		ev.RR.Res_ = response.ResponseInit() // Create a res Object
		ev.RR.CircleInit = true
	}
	// fmt.Printf("%p, %p", ev.RR.Req_, ev)
	if byte_row == nil { // client closed
		ev.Close()
		return 0
	} else {
		ev.RR.Req_.HttpParse(str_row)
		ev.RR.Req_.ParseBody(byte_row)
		// parse host
		ev.RR.Req_.ParseHost(ev.Lis_info)
	}
	return 1
}
