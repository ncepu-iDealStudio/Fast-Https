package response

import (
	"fast-https/config"
	"strconv"
	"strings"
)

// ApplySecurityHeaders 在响应写出前统一补齐 security.headers 里的安全头。
// 幂等：模块已经写过同名头时不覆盖，因此重复调用安全。
// 默认只开 X-Content-Type-Options（无兼容风险），其余必须显式配置。
func ApplySecurityHeaders(res *Response) {
	if res == nil || res.Headers == nil {
		return
	}
	sec := config.Current().Security
	if sec.NoSniffEnabled() {
		setIfEmpty(res, "X-Content-Type-Options", "nosniff")
	}
	if sec == nil {
		return
	}
	setIfEmpty(res, "Strict-Transport-Security", sec.Headers.HSTS)
	setIfEmpty(res, "X-Frame-Options", sec.Headers.FrameOptions)
	setIfEmpty(res, "Referrer-Policy", sec.Headers.ReferrerPolicy)
	setIfEmpty(res, "Content-Security-Policy", sec.Headers.CSP)
}

func setIfEmpty(res *Response, key, value string) {
	if value == "" || res.Headers[key] != "" {
		return
	}
	res.SetHeader(key, value)
}

// DefaultMethodNotAllowed 返回 405，并带上 location 允许的方法列表。
func DefaultMethodNotAllowed(allow []string) *Response {
	res := ResponseInit()
	res.SetFirstLine(405, "METHODNOTALLOWED")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", CachedDate())
	if len(allow) > 0 {
		res.SetHeader("Allow", strings.Join(allow, ", "))
	}

	res.SetHeader("Content-Type", "text/html")
	body := []byte(DefaultResponseBody("405 Method Not Allowed",
		"This location does not accept the request method."))
	res.SetHeader("Content-Length", strconv.Itoa(len(body)))
	res.SetBody(body)
	return res
}

// DefaultMisdirected 返回 421：Host 未匹配任何 server_name 且 host_check=enforce。
func DefaultMisdirected() *Response {
	res := ResponseInit()
	res.SetFirstLine(421, "MISDIRECTEDREQUEST")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", CachedDate())

	res.SetHeader("Content-Type", "text/html")
	body := []byte(DefaultResponseBody("421 Misdirected Request",
		"The request was sent to a server that cannot produce a response for this host."))
	res.SetHeader("Content-Length", strconv.Itoa(len(body)))
	res.SetBody(body)
	return res
}
