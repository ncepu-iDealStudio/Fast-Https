package proxy

import (
	"strconv"
	"strings"

	"fast-https/config"
	"fast-https/core"
	"fast-https/modules/compress"
	"fast-https/runtime/listener"
	"fast-https/runtime/response"
)

// compressResponse 按 location 的压缩配置压缩上游响应体。
//
// 上游响应会先完整读进 Res.Body，因此这里可以直接改正文与头部；
// 缓存写入发生在调用本函数之前，缓存里始终保存未压缩内容。
func compressResponse(cfg *listener.ListenCfg, ev *core.Event) {
	if cfg == nil || cfg.Zip == config.ZIP_NONE || ev == nil || ev.RR.Req == nil || ev.RR.Res == nil {
		return
	}
	// 升级连接（websocket 等）是双向流，压缩会破坏协议语义。
	if ev.Upgrade != "" {
		return
	}
	addVary(ev.RR.Res, "Accept-Encoding")

	// HEAD 没有正文，上游的 Content-Length 已经描述 GET 的长度。
	if ev.RR.Req.Method == "HEAD" {
		return
	}
	// 上游已经压过：不能二次压缩，也不能改它的长度。
	if ev.RR.Res.Headers["Content-Encoding"] != "" {
		return
	}
	status := responseStatus(ev.RR.Res)
	if status < 200 || status == 204 || status == 206 || status == 304 {
		return
	}
	// 部分内容带 Content-Range，压缩后 offset 语义不成立。
	if ev.RR.Res.Headers["Content-Range"] != "" {
		return
	}
	body := ev.RR.Res.Body
	if len(body) == 0 {
		return
	}
	if cfg.ZipMinLength > 0 && len(body) < cfg.ZipMinLength {
		return
	}
	if compress.Skip(ev.RR.Res.Headers["Content-Type"], cfg.ZipSkipTypes) {
		return
	}
	encoding := compress.Wanted(cfg.Zip, ev.RR.Req.HeaderFold("Accept-Encoding"))
	if encoding == "" {
		return
	}
	out, err := compress.CompressBytes(body, encoding)
	if err != nil {
		return
	}
	ev.RR.Res.Body = out
	ev.RR.Res.SetHeader("Content-Encoding", encoding)
	// 上游如果是 chunked，正文已被解码，压缩后必须用长度分帧。
	ev.RR.Res.DelHeader("Transfer-Encoding")
	ev.RR.Res.SetHeader("Content-Length", strconv.Itoa(len(out)))
}

// responseStatus 从状态行取状态码，解析失败返回 0。
func responseStatus(res *response.Response) int {
	parts := strings.SplitN(res.FirstLine, " ", 3)
	if len(parts) < 2 {
		return 0
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return code
}

// addVary 追加一个 Vary 值；已有同名项时不重复。
func addVary(res *response.Response, token string) {
	cur := res.Headers["Vary"]
	if cur == "" {
		res.SetHeader("Vary", token)
		return
	}
	for _, part := range strings.Split(cur, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return
		}
	}
	res.SetHeader("Vary", cur+", "+token)
}
