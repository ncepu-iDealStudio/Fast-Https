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

// distribute event
// LisType(2) tcp proxy
func HandleEvent(ev *core.Event) {
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
		ev.Write_bytes_close(response.DefaultNotFound())
	} else {

		if !safe.Gcl[cfg.ID].Insert1(strings.Split(ev.Conn.RemoteAddr().String(), ":")[0]) {
			safe.CountHandler(ev.RR)
			return
		}

		switch cfg.Type {
		case config.LOCAL:
			if HandelSlash(cfg, ev) {
				return
			}
			// according to user's confgure and requets endporint handle events
			StaticEvent(cfg, ev)
			return
		case config.PROXY_HTTP, config.PROXY_HTTPS:
			// according to user's confgure and requets endporint handle events
			ev.RR.CircleHandler.RRHandler = core.GRRCHT[config.PROXY_HTTP].RRHandler
			ev.RR.CircleHandler.FliterHandler = core.GRRCHT[config.PROXY_HTTP].FliterHandler
			ev.RR.CircleHandler.FliterHandler(cfg, ev)
			ev.RR.CircleHandler.RRHandler(cfg, ev)
			return
		}
	}
}

func FliterHostPath(ev *core.Event) (listener.ListenCfg, bool) {
	hosts := ev.Lis_info.HostMap[ev.RR.Req_.GetHeader("Host")]
	var cfg listener.ListenCfg
	ok := false
	for _, cfg = range hosts {
		re := regexp.MustCompile(cfg.Path) // we can compile this when load config
		res := re.FindStringIndex(ev.RR.Req_.Path)
		if res != nil {
			originPath := ev.RR.Req_.Path[res[1]:]
			ev.RR.OriginPath = originPath
			ev.RR.PathLocation = res
			ok = true
			break
		}
	}
	return cfg, ok
}

func HandelSlash(cfg listener.ListenCfg, ev *core.Event) (flag bool) {
	if ev.RR.OriginPath == "" && cfg.Path != "/" {
		event_301(ev, ev.RR.Req_.Path[ev.RR.PathLocation[0]:ev.RR.PathLocation[1]]+"/")
		return true
	}
	return false
}

func processRequest(ev *core.Event) int {
	// read data (bytes and str) from socket
	byte_row, str_row := (ev).Read_data()
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
