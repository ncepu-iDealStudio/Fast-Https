package proxy

import (
	"fast-https/config"
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

func testZone(name, path string) config.CacheZone {
	return config.CacheZone{Name: name, Path: path, Valid: []string{"any:1h"}}
}

// 缓存键由区名与 $host$request_uri 组成，站点不再配置键。
func TestCacheKeyIncludesZoneHostAndURI(t *testing.T) {
	pc := NewProxyCache(testZone("edge", "./cache"))
	ev := newCacheKeyEvent(t, "example.com", "/a/b?x=1")

	got := pc.CacheKey(ev)
	want := cache.GetMd5("edge|example.com/a/b?x=1")
	if got != want {
		t.Fatalf("CacheKey() = %q, want md5(%q) = %q", got, "edge|example.com/a/b?x=1", want)
	}
	if len(got) != 32 {
		t.Errorf("cache key %q should be a 32 char md5 hex string", got)
	}
}

// 两个区即使 URI 相同也不能命中彼此的条目。
func TestCacheKeyIsolatesZones(t *testing.T) {
	ev := newCacheKeyEvent(t, "example.com", "/index.html")
	a := NewProxyCache(testZone("a", "./cache"))
	b := NewProxyCache(testZone("b", "./cache"))
	if a.CacheKey(ev) == b.CacheKey(ev) {
		t.Fatal("different zones must not share a cache key")
	}
}

// 没有区名或没有目录时不发缓存（键为空）。
func TestCacheKeyRequiresZoneAndPath(t *testing.T) {
	ev := newCacheKeyEvent(t, "example.com", "/a")
	cases := map[string]*ProxyCache{
		"empty zone": NewProxyCache(testZone("", "./cache")),
		"empty path": NewProxyCache(testZone("edge", "")),
	}
	for name, pc := range cases {
		if got := pc.CacheKey(ev); got != "" {
			t.Errorf("%s: CacheKey() = %q, want empty", name, got)
		}
	}
}

// Host 大小写与端口不参与归一化：不同写法必须是不同的键，
// 否则 example.com 与 EXAMPLE.com:8080 会互相顶掉缓存。
func TestCacheKeyDistinguishesHostCaseAndPort(t *testing.T) {
	pc := NewProxyCache(testZone("edge", "./cache"))

	seen := map[string]string{
		"lower":           pc.CacheKey(newCacheKeyEvent(t, "example.com", "/index.html")),
		"upper":           pc.CacheKey(newCacheKeyEvent(t, "Example.COM", "/index.html")),
		"withPort":        pc.CacheKey(newCacheKeyEvent(t, "example.com:8080", "/index.html")),
		"withDefaultPort": pc.CacheKey(newCacheKeyEvent(t, "example.com:80", "/index.html")),
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

// 同一请求重复取值必须稳定，不能因为 map 遍历顺序不同而变化。
func TestCacheKeyIsDeterministic(t *testing.T) {
	pc := NewProxyCache(testZone("edge", "./cache"))

	first := pc.CacheKey(newCacheKeyEvent(t, "example.com", "/a/b?x=1&y=2"))
	for i := 0; i < 20; i++ {
		if got := pc.CacheKey(newCacheKeyEvent(t, "example.com", "/a/b?x=1&y=2")); got != first {
			t.Fatalf("iteration %d: key %q, want %q", i, got, first)
		}
	}
}
