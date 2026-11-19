package core

import (
	"net"
	"net/netip"
	"strings"

	"fast-https/infra/logging"
	"fast-https/runtime/listener"
)

// ClientIP 返回 TCP 直连对端的 IP（不含端口）。对端地址取不到时返回空串。
func ClientIP(ev *Event) string {
	if ev == nil || ev.Conn == nil {
		return ""
	}
	addr := ev.Conn.RemoteAddr()
	if addr == nil {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr.String()); err == nil {
		return host
	}
	return strings.Trim(addr.String(), "[]")
}

// ForwardedForChain 计算转发给上游的 X-Forwarded-For。
//
// 默认不信任客户端自带的 X-Forwarded-For：直连对端不在 trusted_proxies
// 里时，入站整条丢弃，只写真实对端地址。对端受信时才保留入站链并追加
// 对端地址。max_hops 大于 0 且链长已达上限时停止追加，并记一条 Warn。
func ForwardedForChain(cfg *listener.ListenCfg, ev *Event) string {
	return forwardedForChain(cfg, ev, true)
}

// forwardedForChain 是策略本体。warn 只在转发阶段为真：解析阶段
// （$proxy_add_x_forwarded_for）与转发阶段会用同一个值，避免重复告警。
func forwardedForChain(cfg *listener.ListenCfg, ev *Event, warn bool) string {
	ip := ClientIP(ev)
	chain := ""
	if trustedPeer(cfg, ip) && ev != nil && ev.RR.Req != nil {
		chain = strings.Join(nonEmptyItems(ev.RR.Req.GetHeader("X-Forwarded-For")), ", ")
	}
	if cfg != nil && cfg.ForwardedForMaxHops > 0 && len(nonEmptyItems(chain)) >= cfg.ForwardedForMaxHops {
		if warn {
			warnForwardedForLimit(cfg, ev, chain)
		}
		return chain
	}
	if chain == "" {
		return ip
	}
	if ip == "" {
		return chain
	}
	return chain + ", " + ip
}

// trustedPeer 报告直连对端是否命中 trusted_proxies。
func trustedPeer(cfg *listener.ListenCfg, ip string) bool {
	if cfg == nil || len(cfg.TrustedProxies) == 0 || ip == "" {
		return false
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	// 双栈监听会把 IPv4 客户端表示为 ::ffff:a.b.c.d，先还原成 IPv4
	// 再匹配 10.0.0.0/8 这类前缀，否则受信判断会漏掉。
	addr = addr.Unmap()
	for _, prefix := range cfg.TrustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// nonEmptyItems 按逗号切分并去掉空白项，链长按这个结果计数。
func nonEmptyItems(chain string) []string {
	var out []string
	for _, item := range strings.Split(chain, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func warnForwardedForLimit(cfg *listener.ListenCfg, ev *Event, chain string) {
	host, path := "", ""
	if ev != nil && ev.RR.Req != nil {
		host, path = ev.RR.Req.GetHost(), ev.RR.Req.Path
	}
	logging.ConsoleLog.Warn().Msgf(
		"proxy.forwarded_for.max_hops reached, client address not appended: host=%s path=%s hops=%d max_hops=%d remote_ip=%s",
		host, path, len(nonEmptyItems(chain)), cfg.ForwardedForMaxHops, ClientIP(ev))
}
