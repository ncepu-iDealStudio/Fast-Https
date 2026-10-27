package proxy

import (
	"net"
	"testing"
	"time"

	"fast-https/config"
)

// withFakeNow 注入测试时钟，控制空闲判定
func withFakeNow(t *testing.T, now *time.Time) {
	t.Helper()
	prev := timeNow
	timeNow = func() time.Time { return *now }
	t.Cleanup(func() { timeNow = prev })
}

func newPooledProxy(t *testing.T) (*Proxy, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	p := &Proxy{
		ProxyAddr: "pipe",
		ProxyType: config.PROXY_HTTP,
		ProxyConn: client,
		Read:      func(b []byte) (int, error) { return client.Read(b) },
		Write:     func(b []byte) (int, error) { return client.Write(b) },
	}
	p.Close = func() error {
		p.closed = true
		return client.Close()
	}
	return p, server
}

func anotherPooledProxy(t *testing.T) *Proxy {
	p, _ := newPooledProxy(t)
	return p
}

// 归还后借出的是同一条连接
func TestPoolReusesConnection(t *testing.T) {
	cp := newConnPool(4, time.Minute)
	p, _ := newPooledProxy(t)
	cp.put("up", p)

	got, err := cp.get("up", func() (*Proxy, error) {
		t.Fatal("should not dial when idle connection available")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != p {
		t.Fatal("pool returned a different connection")
	}
	if !got.reused {
		t.Error("reused flag should be set on pooled connections")
	}
}

// 超过每上游空闲上限的连接直接关闭
func TestPoolClosesBeyondMaxIdle(t *testing.T) {
	cp := newConnPool(1, time.Minute)
	p1, _ := newPooledProxy(t)
	p2, _ := newPooledProxy(t)
	cp.put("up", p1)
	cp.put("up", p2) // 超限，立即关闭

	if !p2.isClosed() {
		t.Error("connection beyond max idle should be closed immediately")
	}
	if got, _ := cp.get("up", func() (*Proxy, error) {
		t.Fatal("only one idle connection expected")
		return nil, nil
	}); got != p1 {
		t.Fatal("expected the first connection to be returned")
	}
}

// 空闲超过 TTL 的连接在借出时被丢弃
func TestPoolDiscardsExpiredIdle(t *testing.T) {
	now := time.Now()
	withFakeNow(t, &now)
	cp := newConnPool(4, 30*time.Second)
	p, _ := newPooledProxy(t)
	cp.put("up", p)

	now = now.Add(time.Minute)
	dialed := false
	got, err := cp.get("up", func() (*Proxy, error) {
		dialed = true
		return anotherPooledProxy(t), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dialed {
		t.Fatal("expired connection should trigger a fresh dial")
	}
	if got == p {
		t.Fatal("expired connection should not be returned")
	}
	if !p.isClosed() {
		t.Error("expired connection should be closed")
	}
}

// 已关闭的连接不会被归还进池
func TestPoolSkipsClosedConnections(t *testing.T) {
	cp := newConnPool(4, time.Minute)
	p, server := newPooledProxy(t)
	_ = p.Close()
	_ = server.Close()
	cp.put("up", p)

	dialed := false
	got, err := cp.get("up", func() (*Proxy, error) {
		dialed = true
		return anotherPooledProxy(t), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dialed || got == p {
		t.Fatal("closed connection must not be reused")
	}
}

// normalizeUpstreamAddr 补默认端口，并作为池的分组键
func TestNormalizeUpstreamAddr(t *testing.T) {
	cases := []struct {
		in   string
		typ  int
		want string
	}{
		{"example.com", config.PROXY_HTTP, "example.com:80"},
		{"example.com", config.PROXY_HTTPS, "example.com:443"},
		{"example.com:8080", config.PROXY_HTTP, "example.com:8080"},
	}
	for _, c := range cases {
		if got := normalizeUpstreamAddr(c.in, c.typ); got != c.want {
			t.Errorf("normalizeUpstreamAddr(%q, %d) = %q, want %q", c.in, c.typ, got, c.want)
		}
	}
}
