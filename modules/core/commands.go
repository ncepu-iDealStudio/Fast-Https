package core

import (
	"fast-https/modules/core/listener"
	"strings"
)

func DefaultParseCommandHandler(cfg *listener.ListenCfg, ev *Event) {
	ip := ""
	index := strings.LastIndex(ev.Conn.RemoteAddr().String(), ":")
	// 如果找到了该字符
	if index != -1 {
		// 截取字符串，不包括该字符及其后面的字符
		ip = ev.Conn.RemoteAddr().String()[:index]
	}

	xForWardFor := ev.RR.Req.GetHeader("X-Forwarded-For")
	if xForWardFor == "" {
		xForWardFor = ip
	} else {
		xForWardFor = xForWardFor + ", " + ip
	}

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
