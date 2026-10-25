// adapter.go 把自研解析结果适配成 GuardianWAF 需要的 *http.Request。
package appfirewall

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"

	"fast-https/core"
)

// toHTTPRequest 从已解析的 core.Event 合成检测专用的 *http.Request。
//
// 调用时机在 body 完整缓冲之后（受 MaxBodySize 上限约束），
// Body 是内存引用，无二次读 socket。合成的 Request 只供检测，
// 不进入任何响应路径。
//
// 故意不做 URL 解码：原始形态对检测编码绕过更有利。
func toHTTPRequest(ev *core.Event) *http.Request {
	req := ev.RR.Req
	path, query := splitRequestURI(req.Path)

	h := make(http.Header, len(req.Headers))
	for k, v := range req.Headers {
		// 键在解析层已 canonical 化，直接赋值避免二次规范化
		h[k] = []string{v}
	}

	r := &http.Request{
		Method:        req.Method,
		URL:           &url.URL{Path: path, RawQuery: query},
		Host:          req.GetHost(),
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(req.Body.Bytes())),
		ContentLength: int64(req.Body.Len()),
		RemoteAddr:    ev.Conn.RemoteAddr().String(),
		RequestURI:    req.Path,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
	}
	if req.H2 {
		r.Proto, r.ProtoMajor = "HTTP/2", 2
	}
	return r
}

// splitRequestURI 把 RequestURI 切成路径与原始 query。
// "?" 后的内容原样作为 RawQuery（不做解码）。
func splitRequestURI(uri string) (string, string) {
	if i := strings.IndexByte(uri, '?'); i >= 0 {
		return uri[:i], uri[i+1:]
	}
	return uri, ""
}
