package proxy

import (
	"fast-https/core"
	"fast-https/infra/timeutil"
	"fast-https/modules/cache"
	"fast-https/runtime/response"
	"strconv"
	"strings"
	"time"
)

// defaultCacheMethods 是未配置 proxy_cache.methods 时允许读写缓存的方法。
// 只放行幂等方法：其余方法可能改变上游状态，缓存它们的响应会造成数据错乱。
var defaultCacheMethods = []string{"GET", "HEAD"}

// ProxyCache 保存某个 location 的 proxy_cache 配置。
type ProxyCache struct {
	key     string
	valid   []string
	path    string
	methods []string
	maxSize int
}

// NewProxyCache 构造一个 location 的缓存策略。
// methods 为空时使用 defaultCacheMethods。
func NewProxyCache(key string, valid []string, path string, methods []string, maxSize int) *ProxyCache {
	return &ProxyCache{
		key:     key,
		valid:   valid,
		path:    path,
		methods: methods,
		maxSize: maxSize,
	}
}

// methodAllowed 判断该请求方法是否允许读写缓存。
func (pc *ProxyCache) methodAllowed(method string) bool {
	methods := pc.methods
	if len(methods) == 0 {
		methods = defaultCacheMethods
	}
	for _, allowed := range methods {
		if strings.EqualFold(strings.TrimSpace(allowed), method) {
			return true
		}
	}
	return false
}

// CacheKey 返回该请求的缓存键；未配置 key 或 path 时返回空串。
func (pc *ProxyCache) CacheKey(ev *core.Event) string {
	if pc.key == "" || pc.path == "" {
		return ""
	}
	return cache.GetMd5(ev.GetCommandParsedStr(pc.key))
}

// ExpireFor 返回状态码对应的缓存时长（秒），0 表示该响应不缓存。
//
// 精确匹配优先，"any" 作为兜底；格式错误的配置项直接跳过而不是 panic。
func (pc *ProxyCache) ExpireFor(status int) int {
	code := strconv.Itoa(status)
	exact := -1
	fallback := 0
	for _, item := range pc.valid {
		name, duration, ok := strings.Cut(item, ":")
		if !ok {
			continue
		}
		switch name = strings.TrimSpace(name); name {
		case "any":
			fallback = timeutil.ParseTime(strings.TrimSpace(duration))
		case code:
			exact = timeutil.ParseTime(strings.TrimSpace(duration))
		}
	}
	if exact >= 0 {
		return exact
	}
	return fallback
}

// Serve 命中缓存时把条目还原进 ev.RR.Res，并返回状态码。
// 未命中、客户端要求不走缓存、或者键为空都返回 false。
func (pc *ProxyCache) Serve(ev *core.Event) (int, bool) {
	if !pc.methodAllowed(ev.RR.Req.Method) {
		return 0, false
	}
	if pc.clientDisablesCache(ev) {
		return 0, false
	}
	key := pc.CacheKey(ev)
	if key == "" {
		return 0, false
	}
	entry, ok := cache.GCacheContainer.ReadCache(key)
	if !ok {
		return 0, false
	}
	ev.RR.Res = cachedResponse(entry)
	return entry.Status, true
}

// Store 把当前响应写入缓存。status 为上游返回的状态码。
func (pc *ProxyCache) Store(ev *core.Event, status int) {
	if !pc.methodAllowed(ev.RR.Req.Method) {
		return
	}
	key := pc.CacheKey(ev)
	if key == "" || ev.RR.Res == nil {
		return
	}
	expire := pc.ExpireFor(status)
	if expire <= 0 {
		return
	}
	cache.GCacheContainer.WriteCache(key, expire, pc.maxSize, pc.path, cache.CacheEntry{
		FirstLine: ev.RR.Res.FirstLine,
		Status:    status,
		Headers:   cacheableHeaders(ev.RR.Res.Headers),
		Data:      ev.RR.Res.Body,
	})
}

// clientDisablesCache 判断请求是否显式要求不使用缓存。
func (pc *ProxyCache) clientDisablesCache(ev *core.Event) bool {
	directive := strings.ToLower(ev.RR.Req.Headers["Cache-Control"])
	return strings.Contains(directive, "no-cache") || strings.Contains(directive, "no-store")
}

// droppedHeaders 是回放缓存响应时必须剔除的头。
var droppedHeaders = map[string]struct{}{
	// 逐跳头：只对当次连接有效，不能存也不能回放。
	"Connection":          {},
	"Keep-Alive":          {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	// 逐用户头：回放给其他客户端会造成会话串号。
	"Set-Cookie": {},
	// 回放时按当前时间与缓存体长度重写。
	"Date":           {},
	"Content-Length": {},
}

// cacheableHeaders 复制一份可缓存的头。
func cacheableHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		if _, skip := droppedHeaders[key]; skip {
			continue
		}
		out[key] = value
	}
	return out
}

// cachedResponse 把缓存条目还原成响应。
func cachedResponse(entry *cache.CacheEntry) *response.Response {
	res := response.ResponseInit()
	res.FirstLine = entry.FirstLine
	for key, value := range entry.Headers {
		res.Headers[key] = value
	}
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", time.Now().String())
	res.SetHeader("Content-Length", strconv.Itoa(len(entry.Data)))
	res.Body = entry.Data
	return res
}
