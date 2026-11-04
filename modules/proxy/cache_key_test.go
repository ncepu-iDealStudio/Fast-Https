package proxy

import (
	"fast-https/core"
	"fast-https/modules/cache"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"net"
	"testing"
)

// newCacheKeyEvent 复现真实链路：先由 DefaultParseCommandHandler 填好变量表，
// 再让 CacheKey 取 $host / $request_uri。Host 来自请求头原文，
// $request_uri 是解析后的 Path（含查询串）。
func newCacheKeyEvent(t *testing.T, host, path string) *core.Event {
	t.Helper()

	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})

	req := request.RequestInit(false)
	req.Method = "GET"
	req.Path = path
	req.Headers["Host"] = host

	ev := &core.Event{Conn: server, RR: core.RRcircle{Req: req}}
	core.DefaultParseCommandHandler(&listener.ListenCfg{}, ev)
	return ev
}

func TestCacheKeyTable(t *testing.T) {
	cases := []struct {
		name string
		key  string
		path string
		host string
		// raw 是替换变量之后的原文，键是它的 MD5；空串表示不发缓存。
		raw string
	}{
		{name: "host and uri", key: "$host$request_uri", path: "/a/b?x=1", host: "example.com", raw: "example.com/a/b?x=1"},
		{name: "uri only ignores host", key: "$request_uri", path: "/a/b?x=1", host: "example.com", raw: "/a/b?x=1"},
		{name: "empty key config", key: "", path: "/a/b", host: "example.com", raw: ""},
		{name: "empty cache path", key: "$request_uri", path: "/a/b", host: "example.com", raw: ""},
		{name: "no host header", key: "$host$request_uri", path: "/a/b", host: "", raw: "/a/b"},
		{name: "literal prefix kept", key: "/cache/$request_uri", path: "/a/b", host: "h", raw: "/cache//a/b"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cachePath := "./cache"
			if c.name == "empty cache path" {
				cachePath = ""
			}
			pc := NewProxyCache(c.key, []string{"any:1h"}, cachePath, nil, 0)
			ev := newCacheKeyEvent(t, c.host, c.path)

			got := pc.CacheKey(ev)
			if c.raw == "" {
				if got != "" {
					t.Fatalf("CacheKey() = %q, want empty", got)
				}
				return
			}
			if want := cache.GetMd5(c.raw); got != want {
				t.Fatalf("CacheKey() = %q, want md5(%q) = %q", got, c.raw, want)
			}
			if len(got) != 32 {
				t.Errorf("cache key %q should be a 32 char md5 hex string", got)
			}
		})
	}
}

// Host 大小写与端口不参与归一化：不同写法必须是不同的键，
// 否则 example.com 与 EXAMPLE.com:8080 会互相顶掉缓存。
func TestCacheKeyDistinguishesHostCaseAndPort(t *testing.T) {
	pc := NewProxyCache("$host$request_uri", []string{"any:1h"}, "./cache", nil, 0)

	lower := pc.CacheKey(newCacheKeyEvent(t, "example.com", "/index.html"))
	upper := pc.CacheKey(newCacheKeyEvent(t, "Example.COM", "/index.html"))
	withPort := pc.CacheKey(newCacheKeyEvent(t, "example.com:8080", "/index.html"))
	withDefaultPort := pc.CacheKey(newCacheKeyEvent(t, "example.com:80", "/index.html"))

	seen := map[string]string{
		"lower":           lower,
		"upper":           upper,
		"withPort":        withPort,
		"withDefaultPort": withDefaultPort,
	}
	unique := map[string]string{}
	for name, key := range seen {
		if key == "" {
			t.Fatalf("%s: empty cache key", name)
		}
		if other, dup := unique[key]; dup {
			t.Fatalf("%s and %s share cache key %q", name, other, key)
		}
		unique[key] = name
	}
}

// 只按 $request_uri 做键时，Host 的写法差异不能影响命中。
func TestCacheKeyURIIgnoresHostVariants(t *testing.T) {
	pc := NewProxyCache("$request_uri", []string{"any:1h"}, "./cache", nil, 0)

	base := pc.CacheKey(newCacheKeyEvent(t, "example.com", "/app.js?v=1"))
	for _, host := range []string{"EXAMPLE.com", "example.com:443", "", "other.example"} {
		if got := pc.CacheKey(newCacheKeyEvent(t, host, "/app.js?v=1")); got != base {
			t.Errorf("host %q produced key %q, want %q", host, got, base)
		}
	}
}

// 同一请求重复取值必须稳定，不能因为 map 遍历顺序不同而变化。
func TestCacheKeyIsDeterministic(t *testing.T) {
	pc := NewProxyCache("$host$request_uri", []string{"any:1h"}, "./cache", nil, 0)

	first := pc.CacheKey(newCacheKeyEvent(t, "example.com", "/a/b?x=1&y=2"))
	for i := 0; i < 20; i++ {
		if got := pc.CacheKey(newCacheKeyEvent(t, "example.com", "/a/b?x=1&y=2")); got != first {
			t.Fatalf("iteration %d: key %q, want %q", i, got, first)
		}
	}
}
