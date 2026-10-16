package filters

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
)

type FilterInterface struct {
	ConnFilter      func(*core.Event) bool                        // 这是针对 建立连接的 filter
	ListenFilter    func(*core.Event) bool                        // 这是针对 能够有效建立连接的事件的 filter
	HttpParseFilter func(*core.RRcircle) bool                     //这是针对 HTTP请求解析的 filter
	RequestFilter   func(*core.Event) (*listener.ListenCfg, bool) //这是针对 HTTP请求目的的 filter
}

type Filter struct {
	Fif FilterInterface
}

func GConnFilter(each_event *core.Event) bool {
	return !core.ConnRejected(each_event)
}

func GListenFilter(ev *core.Event) bool {
	return core.ListenHandled(ev)
}

func GHttpParseFilter(rr *core.RRcircle) bool {
	conn := rr.Req.GetConnection()

	if conn == "Upgrade" && rr.Req.GetUpgrade() == "websocket" {
		rr.Ev.Upgrade = "websocket"
		rr.Ev.Reuse = true
	}
	return true
}

func GFilterHostPath(ev *core.Event) (*listener.ListenCfg, bool) {
	hosts := ev.LisInfo.HostMap[ev.RR.Req.GetHost()]
	// fmt.Println(hosts)
	var cfg listener.ListenCfg

	for _, cfg = range hosts {
		// we can compile this when load config
		res := cfg.PathRe.FindStringIndex(ev.RR.Req.Path)
		if res != nil {
			originPath := ev.RR.Req.Path[res[1]:]
			ev.RR.OriginPath = originPath
			ev.RR.PathLocation = res

			return &cfg, true
		}
	}

	hosts2 := ev.LisInfo.HostMap[config.DEFAULT_PORT]
	for _, cfg = range hosts2 {
		// we can compile this when load config
		res := cfg.PathRe.FindStringIndex(ev.RR.Req.Path)
		if res != nil {
			originPath := ev.RR.Req.Path[res[1]:]
			ev.RR.OriginPath = originPath
			ev.RR.PathLocation = res

			return &cfg, true
		}
	}

	return &cfg, false
}

func NewFilter() *Filter {
	return &Filter{
		Fif: FilterInterface{
			ConnFilter:      GConnFilter,
			ListenFilter:    GListenFilter,
			HttpParseFilter: GHttpParseFilter,
			RequestFilter:   GFilterHostPath,
		},
	}
}
