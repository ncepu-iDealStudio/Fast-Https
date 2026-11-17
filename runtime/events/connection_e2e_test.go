package events

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/modules/proxy"
	"fast-https/runtime/listener"
	"fast-https/runtime/response"
)

// startE2EServer 起一个真实 TCP 监听器，把每条连接交给 HandleEvent。
// 处理器注册为 DEVMOD 的桩，回显请求路径与正文，便于断言管线后的结果。
func startE2EServer(t *testing.T) string {
	t.Helper()
	core.RegisterHandler(config.DEVMOD,
		func(*listener.ListenCfg, *core.Event) bool { return true },
		func(_ *listener.ListenCfg, ev *core.Event) {
			echo := "path=" + ev.RR.Req.Path + ";body=" + ev.RR.Req.Body.String()
			if ev.RR.Req.HeaderFold("Expect") != "" {
				echo += ";expect=leaked"
			}
			body := []byte(echo)
			ev.RR.Res = response.ResponseInit()
			ev.RR.Res.SetFirstLine(200, "OK")
			ev.RR.Res.SetHeader("Content-Length", strconv.Itoa(len(body)))
			ev.RR.Res.Body = body
			ev.Reuse = true
			_ = ev.WriteResponse(nil)
		},
		nil)
	t.Cleanup(func() { core.RegisterHandler(config.DEVMOD, nil, nil, nil) })

	lis := &listener.Listener{Port: "0", HostMap: map[string][]listener.ListenCfg{
		"example.com": {{
			ID: 1, Path: "/", PathRe: regexp.MustCompile("^/"), Type: config.DEVMOD,
		}},
	}}
	return startServerWithListener(t, lis)
}

// startServerWithListener 起真实 TCP 监听并把连接交给 HandleEvent。
func startServerWithListener(t *testing.T, lis *listener.Listener) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go HandleEvent(lis, conn, ctx)
		}
	}()
	return ln.Addr().String()
}

// TestRealTCPExpectContinueAndKeepAlive 走真实 TCP：100 必须先于正文，
// 正文到达后处理器能收到完整 body，同一连接的下一个请求复用正常，
// 且 Expect 不会泄漏给 handler（即转发层）。
func TestRealTCPExpectContinueAndKeepAlive(t *testing.T) {
	addr := startE2EServer(t)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := io.WriteString(conn,
		"POST /a HTTP/1.1\r\nHost: example.com\r\n"+
			"Content-Length: 5\r\nExpect: 100-continue\r\n\r\n"); err != nil {
		t.Fatalf("write headers: %v", err)
	}

	br := bufio.NewReader(conn)
	if line, err := br.ReadString('\n'); err != nil || !strings.Contains(line, "100 Continue") {
		t.Fatalf("状态行 = %q (err=%v)，期望 100 Continue", strings.TrimSpace(line), err)
	}
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatalf("read blank line: %v", err)
	}
	if _, err := io.WriteString(conn, "hello"); err != nil {
		t.Fatalf("write body: %v", err)
	}

	resp, err := http.ReadResponse(br, &http.Request{Method: "POST"})
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "path=/a;body=hello") {
		t.Fatalf("body = %q", body)
	}
	if strings.Contains(string(body), "expect=leaked") {
		t.Fatalf("Expect 泄漏到了 handler：%q", body)
	}

	// 同一连接上的第二个请求：keep-alive 复用
	if _, err := io.WriteString(conn,
		"GET /b HTTP/1.1\r\nHost: example.com\r\nConnection: keep-alive\r\n\r\n"); err != nil {
		t.Fatalf("write second request: %v", err)
	}
	resp2, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	if err != nil {
		t.Fatalf("read second response: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	if !strings.Contains(string(body2), "path=/b;body=") {
		t.Fatalf("第二个响应 = %q", body2)
	}
}

// TestRealTCPProxyDropsSatisfiedExpect 覆盖「客户端 Expect → 代理 → 上游」：
// 代理先回 100，读正文后转发，上游既收到完整正文也收不到 Expect。
func TestRealTCPProxyDropsSatisfiedExpect(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("upstream listen: %v", err)
	}
	t.Cleanup(func() { _ = upstream.Close() })

	got := make(chan string, 1)
	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			got <- ""
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		var head strings.Builder
		contentLength := 0
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				got <- ""
				return
			}
			head.WriteString(line)
			if line == "\r\n" {
				break
			}
			if k := strings.ToLower(line); strings.HasPrefix(k, "content-length:") {
				contentLength, _ = strconv.Atoi(strings.TrimSpace(line[len("Content-Length:"):]))
			}
		}
		body := make([]byte, contentLength)
		if contentLength > 0 {
			_, _ = io.ReadFull(br, body)
		}
		got <- head.String() + "|BODY=" + string(body)
		_, _ = io.WriteString(conn,
			"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
	}()

	proxy.Register()
	t.Cleanup(func() {
		core.RegisterHandler(config.PROXY_HTTP, nil, nil, nil)
		core.RegisterHandler(config.PROXY_HTTPS, nil, nil, nil)
	})

	lis := &listener.Listener{Port: "0", HostMap: map[string][]listener.ListenCfg{
		"example.com": {{
			ID: 1, Path: "/", PathRe: regexp.MustCompile("^/"),
			Type: config.PROXY_HTTP, ProxyAddr: upstream.Addr().String(), PreserveHost: true,
		}},
	}}
	addr := startServerWithListener(t, lis)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := io.WriteString(conn,
		"POST /p HTTP/1.1\r\nHost: example.com\r\n"+
			"Content-Length: 5\r\nExpect: 100-continue\r\n\r\n"); err != nil {
		t.Fatalf("write headers: %v", err)
	}
	br := bufio.NewReader(conn)
	if line, err := br.ReadString('\n'); err != nil || !strings.Contains(line, "100 Continue") {
		t.Fatalf("状态行 = %q (err=%v)，期望 100 Continue", strings.TrimSpace(line), err)
	}
	_, _ = br.ReadString('\n')
	if _, err := io.WriteString(conn, "hello"); err != nil {
		t.Fatalf("write body: %v", err)
	}

	resp, err := http.ReadResponse(br, &http.Request{Method: "POST"})
	if err != nil {
		t.Fatalf("read proxy response: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	select {
	case seen := <-got:
		if !strings.Contains(seen, "BODY=hello") {
			t.Fatalf("上游正文 = %q", seen)
		}
		if strings.Contains(strings.ToLower(seen), "\r\nexpect:") {
			t.Fatalf("上游收到了不应转发的 Expect：%q", seen)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("上游没有收到请求")
	}
}

func TestRealTCPHeaderTooLarge431(t *testing.T) {
	addr := startE2EServer(t)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	raw := "GET / HTTP/1.1\r\nHost: example.com\r\nX-Big: " +
		strings.Repeat("a", config.DEFAULT_MAX_HEADER_SIZE+1024) + "\r\n\r\n"
	go func() { _, _ = io.WriteString(conn, raw) }()

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(line, "431") {
		t.Fatalf("状态行 = %q，期望 431", strings.TrimSpace(line))
	}
}

// TestRealTCPBodyTooLarge413 走真实 TCP：请求体超过上限必须得到 413。
func TestRealTCPBodyTooLarge413(t *testing.T) {
	withLimit(t, func(c *config.Fast_Https) { c.Limit.MaxBodySize = 64 })
	addr := startE2EServer(t)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	body := strings.Repeat("x", 128)
	raw := "POST /up HTTP/1.1\r\nHost: example.com\r\nContent-Length: 128\r\n\r\n" + body
	go func() { _, _ = io.WriteString(conn, raw) }()

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(line, "413") {
		t.Fatalf("状态行 = %q，期望 413", strings.TrimSpace(line))
	}
}
