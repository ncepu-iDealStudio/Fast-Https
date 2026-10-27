// Package filters 把配置中的 listen 与 location 规则解析为运行时过滤器。
package filters

import (
	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
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
	hosts := ev.LisInfo.Hosts()[ev.RR.Req.GetHost()]

	if len(hosts) == 0 {
		// Host 没命中任何 server_name 时按 nginx 的 default_server 语义
		// 回落到该端口第一个 server 块：否则换 IP、换别名访问会整站 404。
		hosts = ev.LisInfo.DefaultHosts()
	}
	if cfg, ok := matchLocation(ev, hosts); ok {
		return cfg, true
	}

	// 兼容手工构造的 Listener：HostMap 里以 DEFAULT_PORT 为键的通配项
	if cfg, ok := matchLocation(ev, ev.LisInfo.Hosts()[config.DEFAULT_PORT]); ok {
		return cfg, true
	}

	return &listener.ListenCfg{}, false
}

// matchLocation 在给定 server 块的 location 里找第一条命中的规则，
// 并把剩余路径与匹配位置记录到请求上。
func matchLocation(ev *core.Event, hosts []listener.ListenCfg) (*listener.ListenCfg, bool) {
	for _, item := range hosts {
		cfg := item
		res := cfg.PathRe.FindStringIndex(ev.RR.Req.Path)
		if res == nil {
			continue
		}
		ev.RR.OriginPath = ev.RR.Req.Path[res[1]:]
		ev.RR.PathLocation = res
		return &cfg, true
	}
	return nil, false
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
