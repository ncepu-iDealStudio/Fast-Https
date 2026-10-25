package response

import (
	"net/http"
	"sync/atomic"
	"time"
)

// dateCache 按 Unix 秒缓存格式化结果：每个响应都要带 Date 头，
// net/http 同款策略——同一秒内所有响应复用同一个字符串，
// 避免每请求一次的时区格式化与分配。
var (
	dateValue atomic.Pointer[string]
	dateSec   atomic.Int64
)

// CachedDate 返回当前秒的 RFC 7231 日期串
// （IMF-fixdate："Mon, 02 Jan 2006 15:04:05 GMT"）。
// 并发安全；跨秒边界时由先到的请求负责刷新。
func CachedDate() string {
	sec := time.Now().Unix()
	if v := dateValue.Load(); v != nil && dateSec.Load() == sec {
		return *v
	}
	s := time.Now().UTC().Format(http.TimeFormat)
	dateValue.Store(&s)
	dateSec.Store(sec)
	return s
}
