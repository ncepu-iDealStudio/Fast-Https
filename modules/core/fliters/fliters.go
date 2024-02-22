package fliters

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
	"fast-https/modules/proxy_tcp"
	"fast-https/modules/safe"
	"regexp"
)

type FliterInterface struct {
	ConnFliter      func(*core.Event) bool                       // 这是针对 建立连接的 fliter
	ListenFliter    func(*core.Event)                            // 这是针对 能够有效建立连接的事件的 fliter
	HttpParseFliter func(*core.RRcircle) bool                    //这是针对 HTTP请求解析的 fliter
	RequestFliter   func(*core.Event) (listener.ListenCfg, bool) //这是针对 HTTP请求目的的 fliter
}

type Fliter struct {
	Fif FliterInterface
}

func GConnFliter(each_event *core.Event) bool {
	if !safe.Bucket(each_event) {
		return false
	}

	if safe.IsInBlacklist(each_event) {
		return false
	}
	return true
}

func GListenFliter(ev *core.Event) {
	// handle tcp proxy
	if ev.LisInfo.LisType == config.PROXY_TCP {
		proxy_tcp.ProxyEventTcp(ev.Conn, ev.LisInfo.Cfg[0].ProxyAddr)
	}
}

func GHttpParseFliter(rr *core.RRcircle) bool {
	return true
}

func GFliterHostPath(ev *core.Event) (listener.ListenCfg, bool) {
	hosts := ev.LisInfo.HostMap[ev.RR.Req_.GetHeader("Host")]
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

	hosts2 := ev.LisInfo.HostMap[config.DEFAULT_PORT]
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

func NewFliter() *Fliter {
	return &Fliter{
		Fif: FliterInterface{
			ConnFliter:      GConnFliter,
			ListenFliter:    GListenFliter,
			HttpParseFliter: GHttpParseFliter,
			RequestFliter:   GFliterHostPath,
		},
	}
}
