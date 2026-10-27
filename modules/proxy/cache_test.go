package proxy

import (
	"fast-https/core"
	"fast-https/modules/cache"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"testing"
)

func TestStoreSkipsNonCacheableMethod(t *testing.T) {
	dir := t.TempDir()
	pc := NewProxyCache("$request_uri", []string{"any:1h"}, dir, nil, 0)
	ev := &core.Event{RR: core.RRcircle{
		Req: &request.Request{Method: "POST", Headers: map[string]string{}},
		Res: &response.Response{
			FirstLine: "HTTP/1.1 200 OK",
			Headers:   map[string]string{},
			Body:      []byte("created"),
		},
	}}

	pc.Store(ev, 200)

	if _, ok := cache.GCacheContainer.ReadCache(pc.CacheKey(ev)); ok {
		t.Error("POST response must not be cached by default")
	}
}

func TestExpireFor(t *testing.T) {
	cases := []struct {
		name   string
		valid  []string
		status int
		want   int
	}{
		{"exact match", []string{"200:1h", "404:1m"}, 200, 3600},
		{"falls back to any", []string{"200:1h", "any:30m"}, 404, 1800},
		{"exact wins over any", []string{"any:30m", "404:1h"}, 404, 3600},
		{"no rule", []string{"200:1h"}, 500, 0},
		{"empty config", nil, 200, 0},
		{"malformed entry is skipped", []string{"200", "any:10m"}, 404, 600},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pc := NewProxyCache("$host$request_uri", c.valid, "./cache", nil, 0)
			if got := pc.ExpireFor(c.status); got != c.want {
				t.Errorf("ExpireFor(%d) = %d, want %d", c.status, got, c.want)
			}
		})
	}
}

func TestMethodAllowed(t *testing.T) {
	cases := []struct {
		name    string
		methods []string
		method  string
		want    bool
	}{
		{"default allows GET", nil, "GET", true},
		{"default allows HEAD", nil, "HEAD", true},
		{"default matches case-insensitively", nil, "get", true},
		{"default rejects POST", nil, "POST", false},
		{"default rejects PUT", nil, "PUT", false},
		{"default rejects DELETE", nil, "DELETE", false},
		{"configured allows POST", []string{"GET", "POST"}, "POST", true},
		{"configured still rejects PUT", []string{"GET", "POST"}, "PUT", false},
		{"configured tolerates spaces", []string{" get "}, "GET", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pc := NewProxyCache("k", nil, "./cache", c.methods, 0)
			if got := pc.methodAllowed(c.method); got != c.want {
				t.Errorf("methodAllowed(%q) = %v, want %v", c.method, got, c.want)
			}
		})
	}
}

func TestCacheableHeadersDropsHopByHopAndSetCookie(t *testing.T) {
	headers := map[string]string{
		"Content-Type":      "text/html",
		"Connection":        "keep-alive",
		"Transfer-Encoding": "chunked",
		"Date":              "yesterday",
		"Content-Length":    "3",
		"ETag":              `"abc"`,
		"Set-Cookie":        "session=abc123; HttpOnly",
	}
	got := cacheableHeaders(headers)

	for _, key := range []string{"Content-Type", "ETag"} {
		if got[key] != headers[key] {
			t.Errorf("%s should be kept, got %q", key, got[key])
		}
	}
	for _, key := range []string{"Connection", "Transfer-Encoding", "Date", "Content-Length", "Set-Cookie"} {
		if _, ok := got[key]; ok {
			t.Errorf("%s should be dropped", key)
		}
	}
}

func TestCachedResponseRebuildsFraming(t *testing.T) {
	entry := &cache.CacheEntry{
		FirstLine: "HTTP/1.1 200 OK",
		Status:    200,
		Headers:   map[string]string{"Content-Type": "text/plain"},
		Data:      []byte("hello"),
	}
	res := cachedResponse(entry)

	if res.FirstLine != "HTTP/1.1 200 OK" {
		t.Errorf("FirstLine = %q", res.FirstLine)
	}
	if string(res.Body) != "hello" {
		t.Errorf("Body = %q", res.Body)
	}
	if res.Headers["Content-Length"] != "5" {
		t.Errorf("Content-Length = %q, want 5", res.Headers["Content-Length"])
	}
	if res.Headers["Content-Type"] != "text/plain" {
		t.Errorf("Content-Type = %q", res.Headers["Content-Type"])
	}
	if res.Headers["Date"] == "" {
		t.Error("Date should be regenerated")
	}
}
