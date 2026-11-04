// Package filters 把配置中的 listen 与 location 规则解析为运行时过滤器。
package filters

import (
	"fast-https/config"
	"fast-https/core"
	"fast-https/infra/logging"
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
	hosts := hostCfgs(ev)

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

// hostCfgs 取当前端口上 Host 对应的 location 列表。
//
// HostMap 的键是 server_name:port，而客户端访问非标准端口时不会把端口写进
// Host（ParseHost 只给 80/443 补端口）。只按 Host 查会把 example.com:8080 上
// 的站点全部落到 default_server，表现为"配了反代却进了别的站点的静态目录"。
// 因此未命中时再按 host:port 试一次。
func hostCfgs(ev *core.Event) []listener.ListenCfg {
	hosts := ev.LisInfo.Hosts()
	host := ev.RR.Req.GetHost()
	if cfgs := hosts[host]; len(cfgs) > 0 {
		return cfgs
	}
	if port := ev.LisInfo.Port; port != "" {
		return hosts[host+":"+port]
	}
	return nil
}

// matchLocation 在给定 server 块的 location 里选最具体的规则，并把剩余路径
// 与匹配位置记录到请求上。
//
// 不能简单地"按配置顺序取第一个命中"：写成 `location /`（静态）+ `/api`
// （反代）时，`/` 是正则包含匹配，会先吃掉所有请求，反代规则永远不生效。
// 这里按 nginx 的前缀语义排序：
//  1. 从路径起点命中的优先于中途命中（`/api` 命中 `/api/x` 优于 `pi`）；
//  2. 同类里匹配更长（更具体）的优先；
//  3. 完全相同时保持配置顺序，先写的生效。
func matchLocation(ev *core.Event, hosts []listener.ListenCfg) (*listener.ListenCfg, bool) {
	path := ev.RR.Req.Path
	best := -1
	bestLen := 0
	bestAtStart := false

	for i := range hosts {
		res := hosts[i].PathRe.FindStringIndex(path)
		if res == nil {
			continue
		}
		atStart := res[0] == 0
		length := res[1] - res[0]
		better := false
		switch {
		case best < 0:
			better = true
		case atStart != bestAtStart:
			// 起点命中一律优先于中途命中。
			better = atStart
		case length > bestLen:
			better = true
		}
		if better {
			best, bestLen, bestAtStart = i, length, atStart
		}
	}

	if best < 0 {
		return nil, false
	}
	cfg := hosts[best]
	res := cfg.PathRe.FindStringIndex(path)
	ev.RR.OriginPath = path[res[1]:]
	ev.RR.PathLocation = res
	return &cfg, true
}

// HostCheckRejects 实现 security.host_check：Host 未命中任何 server_name 时，
// enforce 拒绝（421），warn 只写安全日志，off 保持既有的回落默认 server 行为。
func HostCheckRejects(ev *core.Event) bool {
	if ev == nil || ev.RR.Req == nil || ev.LisInfo == nil {
		return false
	}
	mode := config.Current().Security.HostCheckMode()
	if mode == config.HostCheckOff {
		return false
	}
	host := ev.RR.Req.GetHost()
	if len(hostCfgs(ev)) > 0 {
		return false
	}
	if mode == config.HostCheckEnforce {
		return true
	}
	logging.Safe().Warn().
		Str("reason", "host_check").
		Str("host", host).
		Str("port", ev.LisInfo.Port).
		Msg("host does not match any server_name")
	return false
}

// MethodAllowed 报告该方法是否被该 location 接受。
// allow_methods 为空表示不限制；HEAD 随 GET 一起放行（RFC 9110 §9.3.2）。
func MethodAllowed(cfg *listener.ListenCfg, method string) bool {
	if cfg == nil || len(cfg.AllowMethods) == 0 {
		return true
	}
	for _, allowed := range cfg.AllowMethods {
		if allowed == method {
			return true
		}
	}
	if method == "HEAD" {
		for _, allowed := range cfg.AllowMethods {
			if allowed == "GET" {
				return true
			}
		}
	}
	return false
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
