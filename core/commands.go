package core

import (
	"fast-https/runtime/listener"
	"strings"
)

func DefaultParseCommandHandler(cfg *listener.ListenCfg, ev *Event) {
	ip := ClientIP(ev)
	// 与转发阶段共用同一套策略，避免 $proxy_add_x_forwarded_for 与
	// 实际发出的 X-Forwarded-For 语义漂移。解析阶段读的是原始请求头，
	// ChangeHead 的改写不会造成重复追加。
	xForWardFor := forwardedForChain(cfg, ev, false)

	ev.RR.CircleCommandVal.Map = map[string]string{
		"request_method":            ev.RR.Req.Method,
		"request_uri":               ev.RR.Req.Path,
		"host":                      ev.RR.Req.GetHost(),
		"proxy_host":                cfg.ProxyAddr,
		"remote_addr":               ip,
		"proxy_add_x_forwarded_for": xForWardFor,
	}
}

func (ev *Event) GetCommandParsedStr(inputString string) string {
	out := inputString
	for key, value := range ev.RR.CircleCommandVal.Map {
		out = strings.Replace(out, "$"+key, value, -1) // 只替换第一次出现的关键词
	}
	return out
}
