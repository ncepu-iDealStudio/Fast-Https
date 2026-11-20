package proxy

import "strings"

// upstreamTarget 按 nginx 语义生成发往上游的请求目标。
//
// proxy_pass 带 URI 时，用该 URI 替换命中的 location 前缀；不带 URI 时
// 原样转发请求路径（Vite 默认行为）。只在命中从路径起点开始时替换，
// 非前缀命中的正则 location 保持原路径。
//
//	location /api + proxy_pass http://backend/     /api/users?x=1 → /users?x=1
//	location /api + proxy_pass http://backend/v1   /api/users      → /v1/users
func upstreamTarget(origin string, loc []int, passPath string) string {
	if passPath == "" || len(loc) < 2 || loc[0] != 0 {
		return origin
	}
	path, query := origin, ""
	if i := strings.IndexByte(origin, '?'); i >= 0 {
		path, query = origin[:i], origin[i:]
	}
	end := loc[1]
	if end < 0 || end > len(path) {
		end = len(path)
	}
	rest := path[end:]
	// 路径正好等于 location 前缀时，按 nginx 规则原样使用 proxy_pass 的 URI：
	// /v1 → /v1?x=1，/v1/ → /v1/?x=1，/ → /?x=1。
	if rest == "" {
		return passPath + query
	}
	prefix := strings.TrimSuffix(passPath, "/")
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	return prefix + rest + query
}
