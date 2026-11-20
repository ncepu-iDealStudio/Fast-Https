package core

import (
	"fast-https/runtime/listener"
	"sort"
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
	if !strings.Contains(inputString, "$") || len(ev.RR.CircleCommandVal.Map) == 0 {
		return inputString
	}
	// 按变量名长度降序构造 Replacer：单遍扫描输入串，已替换出的变量值
	// （如 $request_uri 或 $host 中夹带的 "$proxy_host"）不会被二次展开，
	// 且结果完全确定，不受 map 随机遍历顺序影响。
	keys := make([]string, 0, len(ev.RR.CircleCommandVal.Map))
	for k := range ev.RR.CircleCommandVal.Map {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	oldnew := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		oldnew = append(oldnew, "$"+k, ev.RR.CircleCommandVal.Map[k])
	}
	return strings.NewReplacer(oldnew...).Replace(inputString)
}
