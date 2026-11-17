package proxy

import (
	"bufio"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

// R-12：HEAD 响应即使带 Content-Length 也没有 body，必须在头部结束处停下，
// 不能按 CL 去读一个永远不来的正文（旧实现会一直等到读超时）。
func TestHeadResponseStopsAtHeader(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = upstream.Close() })

	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "HEAD"

	p := newMatrixProxy(client)
	p.ev = ev
	rka := &ReadKeepAlive{p: p}

	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1024\r\n\r\n"))
		// 故意不关闭连接：旧实现会在这里等满 1024 字节直到超时
	}()

	if err := runWithWatchdog(t, "head-response", rka.proxyKeepAlive); err != nil {
		t.Fatalf("HEAD response must finish at header: %v", err)
	}
	if !rka.emptyBody {
		t.Error("HEAD response must be marked as having no body")
	}
	if len(rka.body) != 0 {
		t.Errorf("HEAD body = %q, want empty", rka.body)
	}
}

// 204 / 304 在协议上没有 body：不需要等上游关闭即可判定响应结束。
func TestBodylessStatusWithoutLength(t *testing.T) {
	for _, status := range []string{"204 No Content", "304 Not Modified"} {
		t.Run(status, func(t *testing.T) {
			client, upstream := net.Pipe()
			t.Cleanup(func() { _ = client.Close(); _ = upstream.Close() })

			rka := &ReadKeepAlive{p: newMatrixProxy(client)}
			go func() {
				_, _ = upstream.Write([]byte("HTTP/1.1 " + status + "\r\n\r\n"))
				// 不关闭连接：响应边界由状态码决定
			}()

			if err := runWithWatchdog(t, status, rka.proxyKeepAlive); err != nil {
				t.Fatalf("%s must finish without waiting for close: %v", status, err)
			}
			if !rka.emptyBody {
				t.Errorf("%s must be marked as having no body", status)
			}
			if len(rka.body) != 0 {
				t.Errorf("%s body = %q, want empty", status, rka.body)
			}
		})
	}
}

// 既没有 Content-Length 也没有 chunked 的普通响应仍按旧语义读到连接关闭。
func TestStatusWithoutLengthReadsUntilClose(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 200 OK\r\n\r\nhello"))
		_ = upstream.Close()
	}()

	rka := &ReadKeepAlive{p: newMatrixProxy(client)}
	if err := runWithWatchdog(t, "until-close", rka.proxyKeepAlive); err != nil {
		t.Fatalf("proxyKeepAlive() error: %v", err)
	}
	if string(rka.body) != "hello" {
		t.Errorf("body = %q, want %q", rka.body, "hello")
	}
	if rka.emptyBody {
		t.Error("200 response with body must not be marked empty")
	}
}

// R-12 验收：上游返回 204 且没有 Content-Length 时，第二次请求必须复用
// 同一条上游连接，而不是重新 dial。
func TestBodylessResponseReusesUpstreamConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var accepts atomic.Int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			go serveKeepAlive204(c)
		}
	}()

	cfg := &listener.ListenCfg{ProxyAddr: ln.Addr().String(), Type: uint16(config.PROXY_HTTP)}
	for i := 0; i < 2; i++ {
		ev := core.NewEvent(&listener.Listener{}, nil)
		ev.RR.Req = request.RequestInit(false)
		ev.RR.Req.Method = "GET"
		ev.RR.Req.Protocol = "HTTP/1.1"
		ev.RR.Req.Headers["Connection"] = "keep-alive"
		ev.RR.Res = response.ResponseInit()

		p, err := getProxy(ev, &ev.RR, cfg)
		if err != nil {
			t.Fatalf("request %d: getProxy: %v", i+1, err)
		}
		if i == 1 && !p.reused {
			t.Fatal("second request must reuse the pooled upstream connection")
		}
		status, err := p.getDataFromServer(ev, []byte("GET / HTTP/1.1\r\nHost: up\r\nConnection: keep-alive\r\n\r\n"))
		if err != nil {
			t.Fatalf("request %d: read from upstream: %v", i+1, err)
		}
		if status != 204 {
			t.Fatalf("request %d: status = %d, want 204", i+1, status)
		}
		if len(ev.RR.Res.Body) != 0 {
			t.Fatalf("request %d: 204 body = %q, want empty", i+1, ev.RR.Res.Body)
		}
		releaseProxy(p)
	}

	if got := accepts.Load(); got != 1 {
		t.Fatalf("upstream accepted %d connections, want 1 (connection not reused)", got)
	}
}

// 204 / 304 即使带 Content-Length 也没有 body：304 的 CL 表示 200 时正文长度，
// 不能按它去读，否则会一直等到读超时。
func TestBodylessStatusWithContentLength(t *testing.T) {
	cases := []string{
		"HTTP/1.1 204 No Content\r\nContent-Length: 10\r\n\r\n",
		"HTTP/1.1 304 Not Modified\r\nContent-Length: 123\r\n\r\n",
	}
	for _, raw := range cases {
		t.Run(strings.Fields(raw)[1], func(t *testing.T) {
			client, upstream := net.Pipe()
			t.Cleanup(func() { _ = client.Close(); _ = upstream.Close() })

			go func() { _, _ = upstream.Write([]byte(raw)) }()

			rka := &ReadKeepAlive{p: newMatrixProxy(client)}
			if err := runWithWatchdog(t, "bodyless-with-length", rka.proxyKeepAlive); err != nil {
				t.Fatalf("proxyKeepAlive() error: %v", err)
			}
			if !rka.emptyBody {
				t.Error("bodyless status must not read Content-Length bytes")
			}
			if len(rka.body) != 0 {
				t.Errorf("body = %q, want empty", rka.body)
			}
		})
	}
}

// 空 body 响应是否复用连接，取决于上游是否显式要求关闭以及协议版本。
func TestEmptyBodyReuseDecision(t *testing.T) {
	cases := []struct {
		name        string
		method      string
		resp        string
		wantAccepts int64
	}{
		{"204-default-keepalive", "GET", "HTTP/1.1 204 No Content\r\n\r\n", 1},
		{"204-explicit-close", "GET", "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n", 2},
		{"204-http10-without-keepalive", "GET", "HTTP/1.0 204 No Content\r\n\r\n", 2},
		{"head-with-content-length", "HEAD", "HTTP/1.1 200 OK\r\nContent-Length: 1024\r\n\r\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runProxyTwice(t, tc.method, tc.resp); got != tc.wantAccepts {
				t.Fatalf("upstream accepted %d connections, want %d", got, tc.wantAccepts)
			}
		})
	}
}

// runProxyTwice 用真实 TCP 上游处理两次代理请求，返回上游 accept 次数。
func runProxyTwice(t *testing.T, method, resp string) int64 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var accepts atomic.Int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			go serveEmptyBody(c, resp)
		}
	}()

	cfg := &listener.ListenCfg{ProxyAddr: ln.Addr().String(), Type: uint16(config.PROXY_HTTP)}
	req := []byte(method + " / HTTP/1.1\r\nHost: up\r\nConnection: keep-alive\r\n\r\n")
	for i := 0; i < 2; i++ {
		ev := core.NewEvent(&listener.Listener{}, nil)
		ev.RR.Req = request.RequestInit(false)
		ev.RR.Req.Method = method
		ev.RR.Req.Protocol = "HTTP/1.1"
		ev.RR.Req.Headers["Connection"] = "keep-alive"
		ev.RR.Res = response.ResponseInit()

		p, err := getProxy(ev, &ev.RR, cfg)
		if err != nil {
			t.Fatalf("request %d: getProxy: %v", i+1, err)
		}
		if _, err := p.getDataFromServer(ev, req); err != nil {
			t.Fatalf("request %d: read from upstream: %v", i+1, err)
		}
		releaseProxy(p)
	}
	return accepts.Load()
}

// serveKeepAlive204 在一个连接上循环读请求，每个请求都回 204 No Content。
func serveKeepAlive204(c net.Conn) {
	serveEmptyBody(c, "HTTP/1.1 204 No Content\r\n\r\n")
}

// serveEmptyBody 在一个连接上循环读请求，每个请求都返回给定的空 body 响应。
func serveEmptyBody(c net.Conn, resp string) {
	defer c.Close()
	br := bufio.NewReader(c)
	for {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\r\n" {
				break
			}
		}
		if _, err := c.Write([]byte(resp)); err != nil {
			return
		}
	}
}
