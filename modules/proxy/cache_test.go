package proxy

import (
	"fast-https/modules/cache"
	"testing"
)

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
			pc := NewProxyCache("$host$request_uri", c.valid, "./cache", 0)
			if got := pc.ExpireFor(c.status); got != c.want {
				t.Errorf("ExpireFor(%d) = %d, want %d", c.status, got, c.want)
			}
		})
	}
}

func TestCacheableHeadersDropsHopByHop(t *testing.T) {
	headers := map[string]string{
		"Content-Type":      "text/html",
		"Connection":        "keep-alive",
		"Transfer-Encoding": "chunked",
		"Date":              "yesterday",
		"Content-Length":    "3",
		"ETag":              `"abc"`,
	}
	got := cacheableHeaders(headers)

	for _, key := range []string{"Content-Type", "ETag"} {
		if got[key] != headers[key] {
			t.Errorf("%s should be kept, got %q", key, got[key])
		}
	}
	for _, key := range []string{"Connection", "Transfer-Encoding", "Date", "Content-Length"} {
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
