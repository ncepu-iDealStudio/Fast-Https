package proxy

import (
	"strings"

	"fast-https/runtime/listener"
	"fast-https/runtime/request"
)

// hopByHopHeaders 是只对单条连接有意义的头（RFC 9110 §7.6.1）。
// 它们描述的是「本跳」的连接语义，原样带给上游会被误当成对上游连接的
// 指令：例如 TE/trailer 会让上游按分块约定解析，Proxy-Authorization
// 会把只该给代理的凭据泄露给源站。
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection", // 非标准，但客户端普遍用它表达连接语义
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// stripHopByHop 删除单跳头，以及 Connection 里点名的扩展头。
//
// websocket 升级是唯一例外：代理要完成 101 握手，必须把
// `Connection: Upgrade` 与 `Upgrade` 一起带给上游；只保留这两个，
// 客户端 Connection 里其余 token（keep-alive、自定义扩展头）仍然剥掉。
func stripHopByHop(req *request.Request, upgrade string, cfg *listener.ListenCfg) {
	connection := req.GetHeader("Connection")

	for _, header := range hopByHopHeaders {
		req.DelHeader(header)
	}
	for _, token := range strings.Split(connection, ",") {
		token = strings.TrimSpace(token)
		if token == "" || strings.EqualFold(token, "upgrade") {
			continue
		}
		req.DelHeader(token)
	}

	if upgrade == "websocket" {
		req.SetHeader("Connection", "Upgrade", cfg)
		req.SetHeader("Upgrade", upgrade, cfg)
	}
}
