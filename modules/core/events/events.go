package events

import (
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/proxy"
	"fast-https/modules/safe"
	"fast-https/utils/message"
	"regexp"
	"strings"
)

// distribute event
// LisType(2) tcp proxy
func Handle_event(ev *core.Event) {
	// handle tcp proxy
	if ev.Lis_info.LisType == 2 {
		Proxy_event_tcp(ev.Conn, ev.Lis_info.Cfg[0].Proxy_addr)
		return
	}
	if process_request(ev) == 0 {
		return // client close
	}
	ev.Log_append(" " + ev.RR.Req_.Method)
	ev.Log_append(" " + ev.RR.Req_.Path + " \"" +
		ev.RR.Req_.Get_header("Host") + "\"")

	cfg, ok := FliterHostPath(ev)
	if !ok {
		message.PrintAccess(ev.Conn.RemoteAddr().String(),
			"INFORMAL Event(404)"+ev.Log,
			"\""+ev.RR.Req_.Headers["User-Agent"]+"\"")
		ev.Write_bytes_close(response.Default_not_found())
	} else {

		if !safe.Gcl.Insert1(strings.Split(ev.Conn.RemoteAddr().String(), ":")[0]) {
			safe.CountHandler(ev.RR)
			return
		}

		switch cfg.Type {
		case 0:
			if HandelSlash(cfg, ev) {
				return
			}
			// according to user's confgure and requets endporint handle events
			Static_event(cfg, ev)
			return
		case 1, 2:
			ChangeHead(cfg, ev)
			// according to user's confgure and requets endporint handle events
			proxy.Proxy_event(cfg, ev)
			return
		}
	}
}

func FliterHostPath(ev *core.Event) (listener.ListenCfg, bool) {
	hosts := ev.Lis_info.HostMap[ev.RR.Req_.Get_header("Host")]
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
		_event_301(ev, ev.RR.Req_.Path[ev.RR.PathLocation[0]:ev.RR.PathLocation[1]]+"/")
		return true
	}
	return false
}

func ChangeHead(cfg listener.ListenCfg, ev *core.Event) {
	for _, item := range cfg.ProxySetHeader {
		if item.HeaderKey == 100 {
			if item.HeaderValue == "$host" {
				ev.RR.Req_.Set_header("Host", cfg.Proxy_addr, cfg)
			}
		}
	}
	ev.RR.Req_.Set_header("Host", cfg.Proxy_addr, cfg)
	ev.RR.Req_.Set_header("Connection", "close", cfg)
	ev.RR.Req_.Flush()
}

func process_request(ev *core.Event) int {
	// read data (bytes and str) from socket
	byte_row, str_row := (ev).Read_data()
	// save requte information to ev.RR.Req_
	if !ev.RR.CircleInit {
		ev.RR.Req_ = request.Req_init()       // Create a request Object
		ev.RR.Res_ = response.Response_init() // Create a res Object
		ev.RR.CircleInit = true
	}
	// fmt.Printf("%p, %p", ev.RR.Req_, ev)
	if byte_row == nil { // client closed
		ev.Close()
		return 0
	} else {
		ev.RR.Req_.Http_parse(str_row)
		ev.RR.Req_.Parse_body(byte_row)
		// parse host
		ev.RR.Req_.Parse_host(ev.Lis_info)
	}
	return 1
}
