package proxy

import (
	"testing"
	"time"

	"fast-https/core"
	"fast-https/modules/cache"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

func newCacheEvent(method, cacheControl string) *core.Event {
	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = method
	if cacheControl != "" {
		ev.RR.Req.Headers["Cache-Control"] = cacheControl
	}
	ev.RR.CircleCommandVal.Map = map[string]string{
		"request_uri": "/cached-page",
		"host":        "example.com",
	}
	return ev
}

func waitCached(t *testing.T, key string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := cache.GCacheContainer.Index.Touch(key); ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("cache entry %s never written", key)
}

func untrack(key string) {
	cache.GCacheContainer.Index.Remove(key)
}

// Serve：命中缓存时按条目回放；方法、no-cache 指令、未配置键都判未命中
func TestProxyCacheServe(t *testing.T) {
	pc := NewProxyCache("$request_uri", []string{"any:60"}, t.TempDir(), nil, 0)

	key := cache.GetMd5("/cached-page")
	cache.GCacheContainer.WriteCache(key, 60, 0, t.TempDir(), cache.CacheEntry{
		FirstLine: "HTTP/1.1 200 OK",
		Status:    200,
		Headers:   map[string]string{"Content-Type": "text/html"},
		Data:      []byte("served-from-cache"),
	})
	waitCached(t, key)
	t.Cleanup(func() { untrack(key) })

	t.Run("hit replays entry", func(t *testing.T) {
		ev := newCacheEvent("GET", "")
		ev.RR.Res = response.ResponseInit()
		status, ok := pc.Serve(ev)
		if !ok || status != 200 {
			t.Fatalf("Serve() = %d,%v want 200,true", status, ok)
		}
		if string(ev.RR.Res.Body) != "served-from-cache" {
			t.Fatalf("body = %q", ev.RR.Res.Body)
		}
		if ev.RR.Res.GetHeader("Content-Length") != "17" {
			t.Fatalf("Content-Length = %q, want 18", ev.RR.Res.GetHeader("Content-Length"))
		}
	})

	t.Run("non-cacheable method misses", func(t *testing.T) {
		if _, ok := pc.Serve(newCacheEvent("POST", "")); ok {
			t.Fatal("POST must not read cache")
		}
	})

	t.Run("client no-cache directive misses", func(t *testing.T) {
		for _, cc := range []string{"no-cache", "no-store", "max-age=0, no-cache"} {
			if _, ok := pc.Serve(newCacheEvent("GET", cc)); ok {
				t.Fatalf("Cache-Control %q must bypass cache", cc)
			}
		}
	})

	t.Run("missing key config never serves", func(t *testing.T) {
		empty := NewProxyCache("", nil, "", nil, 0)
		if _, ok := empty.Serve(newCacheEvent("GET", "")); ok {
			t.Fatal("unconfigured key must miss")
		}
	})
}

// Store：方法过滤、状态码时长、逐跳/逐用户头剔除
func TestProxyCacheStore(t *testing.T) {
	dir := t.TempDir()
	pc := NewProxyCache("$request_uri", []string{"200:30s", "any:5s"}, dir, []string{"GET"}, 0)

	t.Run("expire zero for unlisted status skips store", func(t *testing.T) {
		noRules := NewProxyCache("$request_uri", nil, dir, nil, 0)
		ev := newCacheEvent("GET", "")
		ev.RR.Res = response.ResponseInit()
		ev.RR.Res.Body = []byte("x")
		noRules.Store(ev, 200)
		if _, ok := cache.GCacheContainer.Index.Touch(cache.GetMd5("/cached-page")); ok {
			t.Fatal("no valid rules must not cache")
		}
	})

	t.Run("stores only allowed methods", func(t *testing.T) {
		ev := newCacheEvent("POST", "")
		ev.RR.Res = response.ResponseInit()
		ev.RR.Res.Body = []byte("never-cached")
		pc.Store(ev, 200)
		if _, ok := cache.GCacheContainer.Index.Touch(cache.GetMd5("/cached-page")); ok {
			t.Fatal("POST must not be stored")
		}
	})

	t.Run("stores entry without hop-by-hop headers", func(t *testing.T) {
		ev := newCacheEvent("GET", "")
		ev.RR.Res = response.ResponseInit()
		ev.RR.Res.FirstLine = "HTTP/1.1 200 OK"
		ev.RR.Res.Headers["Connection"] = "keep-alive"
		ev.RR.Res.Headers["Set-Cookie"] = "session=secret"
		ev.RR.Res.Headers["Content-Type"] = "text/html"
		ev.RR.Res.Body = []byte("stored-body")
		pc.Store(ev, 200)

		key := cache.GetMd5("/cached-page")
		waitCached(t, key)
		t.Cleanup(func() { untrack(key) })

		entry, ok := cache.GCacheContainer.ReadCache(key)
		if !ok {
			t.Fatal("entry missing")
		}
		if _, leaked := entry.Headers["Connection"]; leaked {
			t.Error("Connection header must be dropped")
		}
		if _, leaked := entry.Headers["Set-Cookie"]; leaked {
			t.Error("Set-Cookie header must be dropped")
		}
		if entry.Headers["Content-Type"] != "text/html" {
			t.Errorf("Content-Type lost: %v", entry.Headers)
		}
		if string(entry.Data) != "stored-body" {
			t.Errorf("data = %q", entry.Data)
		}
	})
}

func TestClientDisablesCache(t *testing.T) {
	pc := &ProxyCache{}
	cases := map[string]bool{
		"":                false,
		"max-age=60":      false,
		"no-cache":        true,
		"NO-STORE":        true,
		"no-cache, other": true,
	}
	for header, want := range cases {
		ev := newCacheEvent("GET", header)
		if got := pc.clientDisablesCache(ev); got != want {
			t.Errorf("Cache-Control %q = %v, want %v", header, got, want)
		}
	}
}
