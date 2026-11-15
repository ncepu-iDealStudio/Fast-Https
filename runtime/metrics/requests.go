package metrics

import (
	"strconv"
	"strings"
	"sync/atomic"
)

var (
	requestsTotal    atomic.Uint64
	requestsActive   atomic.Int64
	parseErrorsTotal atomic.Uint64
	abortedTotal     atomic.Uint64
	rejectedTotal    atomic.Uint64
	responses2xx     atomic.Uint64
	responses3xx     atomic.Uint64
	responses4xx     atomic.Uint64
	responses5xx     atomic.Uint64
	responsesUnknown atomic.Uint64
)

// RequestBegin 在 EventHandler 入口调用：累计请求和活动请求各加一。
func RequestBegin() {
	requestsTotal.Add(1)
	requestsActive.Add(1)
}

// RequestEnd 在 EventHandler 的 defer 里调用。firstLine 解析失败计入 unknown，
// 不回落成 2xx（statusFromFirstLine 会把空串当成 200，这里不能复用它）。
func RequestEnd(firstLine string) {
	requestsActive.Add(-1)
	code, ok := ParseStatus(firstLine)
	if !ok {
		responsesUnknown.Add(1)
		return
	}
	switch code / 100 {
	case 2:
		responses2xx.Add(1)
	case 3:
		responses3xx.Add(1)
	case 4:
		responses4xx.Add(1)
	case 5:
		responses5xx.Add(1)
	default:
		responsesUnknown.Add(1)
	}
}

// ParseStatus 从 "HTTP/1.1 200 OK" 取出状态码。空串、乱码、越界返回 ok=false。
func ParseStatus(firstLine string) (code int, ok bool) {
	fields := strings.Fields(firstLine)
	if len(fields) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil || n < 100 || n > 999 {
		return 0, false
	}
	return n, true
}

// NoteParseError 计入写出 400 的畸形/超长请求头，以及 H2 fillRequest 失败。
func NoteParseError() {
	parseErrorsTotal.Add(1)
}

// NoteAborted 计入没有写响应就断开的路径：头未收完、body 超时/过大、分块解码失败。
func NoteAborted() {
	abortedTotal.Add(1)
}

// NoteRejected 计入 ConnFilter 拒绝的连接。这些连接已在 accepted/closed 里，不进请求计数。
func NoteRejected() {
	rejectedTotal.Add(1)
}

func RequestsTotal() uint64    { return requestsTotal.Load() }
func RequestsActive() int64    { return requestsActive.Load() }
func ParseErrorsTotal() uint64 { return parseErrorsTotal.Load() }
func AbortedTotal() uint64     { return abortedTotal.Load() }
func RejectedTotal() uint64    { return rejectedTotal.Load() }
func Responses2xx() uint64     { return responses2xx.Load() }
func Responses3xx() uint64     { return responses3xx.Load() }
func Responses4xx() uint64     { return responses4xx.Load() }
func Responses5xx() uint64     { return responses5xx.Load() }
func ResponsesUnknown() uint64 { return responsesUnknown.Load() }
