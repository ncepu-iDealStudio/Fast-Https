package events

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
)

// withLimit 在测试期间覆盖 limit 段，结束后恢复原快照。
func withLimit(t *testing.T, apply func(*config.Fast_Https)) {
	t.Helper()
	prev := config.Current()
	t.Cleanup(func() { config.Store(prev, map[string]string{}) })
	next := *prev
	apply(&next)
	config.Store(&next, map[string]string{})
}

// newParseEvent 构造一个只跑 parseRequest 的事件，EventWrite 用同包实现，
// 这样拒绝路径写响应不会因为 EventWrite 为空而 panic。
func newParseEvent(t *testing.T) (*core.Event, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.EventWrite = EventWrite
	return ev, client
}

// readStatusLine 读响应首行，用于断言状态码。
func readStatusLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read status line: %v", err)
	}
	return strings.TrimSpace(line)
}

// TestExpectContinueSends100BeforeBody 覆盖 Expect: 100-continue 的基本语义：
// 客户端等到 100 才发正文，服务端必须先在读 body 前回中间响应。
func TestExpectContinueSends100BeforeBody(t *testing.T) {
	ev, client := newParseEvent(t)
	_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))

	headers := "POST /ok HTTP/1.1\r\nHost: example.com\r\n" +
		"Content-Length: 5\r\nExpect: 100-continue\r\n\r\n"
	done := make(chan int, 1)
	go func() { done <- parseRequest(ev, filters.NewFilter()) }()

	if _, err := client.Write([]byte(headers)); err != nil {
		t.Fatalf("write headers: %v", err)
	}

	r := bufio.NewReader(client)
	// 1xx 是中间响应：先 100 Continue，再空行
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read 100 line: %v", err)
	}
	if !strings.Contains(line, "100 Continue") {
		t.Fatalf("状态行 = %q，期望 100 Continue", strings.TrimSpace(line))
	}
	if blank, err := r.ReadString('\n'); err != nil || strings.TrimSpace(blank) != "" {
		t.Fatalf("100 响应后的空行 = %q (err=%v)", blank, err)
	}

	// 收到 100 之后才发正文
	if _, err := client.Write([]byte("hello")); err != nil {
		t.Fatalf("write body: %v", err)
	}

	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("parseRequest() = %d，期望 1", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parseRequest 未返回")
	}
	if got := ev.RR.Req.Body.String(); got != "hello" {
		t.Fatalf("body = %q，期望 hello", got)
	}
	// 期望已在本机满足，转发给上游前必须去掉
	if v := ev.RR.Req.HeaderFold("Expect"); v != "" {
		t.Fatalf("Expect 头未移除：%q", v)
	}
}

// TestExpectContinueOversizedDeclaredBodyGets413 覆盖 R-13 的配合语义：
// Content-Length 已超限时不能先回 100，否则客户端白传一遍正文。
func TestExpectContinueOversizedDeclaredBodyGets413(t *testing.T) {
	withLimit(t, func(c *config.Fast_Https) { c.Limit.MaxBodySize = 4 })

	ev, client := newParseEvent(t)
	_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))

	headers := "POST /upload HTTP/1.1\r\nHost: example.com\r\n" +
		"Content-Length: 10\r\nExpect: 100-continue\r\n\r\n"
	done := make(chan int, 1)
	go func() { done <- parseRequest(ev, filters.NewFilter()) }()
	if _, err := client.Write([]byte(headers)); err != nil {
		t.Fatalf("write headers: %v", err)
	}

	raw, err := io.ReadAll(client)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp := string(raw)
	if !strings.Contains(resp, " 413 ") {
		t.Fatalf("响应 = %q，期望 413", resp)
	}
	if strings.Contains(resp, "100 Continue") {
		t.Fatalf("超限请求不应先回 100：%q", resp)
	}

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("parseRequest() = %d，超限后应返回 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parseRequest 未返回")
	}
}

// TestHeaderTooLargeReturns431 覆盖头部一次到齐与分片到达两条路径，
// 两者都必须回 431 而不是静默放行。
func TestHeaderTooLargeReturns431(t *testing.T) {
	withLimit(t, func(c *config.Fast_Https) { c.Limit.MaxHeaderSize = 64 })

	cases := map[string]string{
		"一次到齐": "GET / HTTP/1.1\r\nHost: example.com\r\nX-Big: " +
			strings.Repeat("a", 120) + "\r\n\r\n",
		"分片到达": "GET / HTTP/1.1\r\nHost: example.com\r\nX-Big: " +
			strings.Repeat("b", 120),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			ev, client := newParseEvent(t)
			_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
			done := make(chan int, 1)
			go func() { done <- parseRequest(ev, filters.NewFilter()) }()
			go func() { _, _ = client.Write([]byte(raw)) }()

			status := readStatusLine(t, bufio.NewReader(client))
			if !strings.Contains(status, "431") {
				t.Fatalf("状态行 = %q，期望 431", status)
			}
			select {
			case code := <-done:
				if code != 0 {
					t.Fatalf("parseRequest() = %d，期望 0", code)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("parseRequest 未返回")
			}
		})
	}
}

// TestMalformedHeaderStillReturns400 锁定畸形头仍回 400，不能被 431 吞掉。
func TestMalformedHeaderStillReturns400(t *testing.T) {
	ev, client := newParseEvent(t)
	_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
	done := make(chan int, 1)
	go func() { done <- parseRequest(ev, filters.NewFilter()) }()
	go func() {
		_, _ = client.Write([]byte("GET / HTTP/1.1\r\nBad Header\r\n\r\n"))
	}()

	status := readStatusLine(t, bufio.NewReader(client))
	if !strings.Contains(status, "400") {
		t.Fatalf("状态行 = %q，期望 400", status)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("parseRequest() = %d，期望 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parseRequest 未返回")
	}
}

// TestKeepAliveResetsRequestAndResponse 覆盖 keep-alive 复用同一个 RRcircle：
// 上一请求的凭据、查询串、body，以及上一响应的头部都不能串到下一请求。
// 静态层直接往 rr.Res.Headers 写头，不复位会让下一个响应带着
// 上一个的 Content-Encoding / Set-Cookie。
func TestKeepAliveResetsRequestAndResponse(t *testing.T) {
	ev, client := newParseEvent(t)
	fif := filters.NewFilter()

	first := "GET /a?x=1 HTTP/1.1\r\nHost: first.example\r\n" +
		"Cookie: sid=secret\r\nAuthorization: Basic abc\r\n" +
		"Accept-Encoding: gzip\r\n\r\n"
	go func() { _, _ = client.Write([]byte(first)) }()
	if code := parseRequest(ev, fif); code != 1 {
		t.Fatalf("第一个请求 parseRequest() = %d", code)
	}

	// 模拟上一请求的响应状态与 handler 留下的路径/升级标记
	ev.RR.Res.SetHeader("Content-Encoding", "gzip")
	ev.RR.Res.SetHeader("Set-Cookie", "sid=secret")
	ev.RR.OriginPath = "/stale"
	ev.Upgrade = "websocket"

	second := "GET /b HTTP/1.1\r\nHost: second.example\r\n\r\n"
	go func() { _, _ = client.Write([]byte(second)) }()
	if code := parseRequest(ev, fif); code != 1 {
		t.Fatalf("第二个请求 parseRequest() = %d", code)
	}

	for _, h := range []string{"Cookie", "Authorization", "Accept-Encoding"} {
		if v, ok := ev.RR.Req.Headers[h]; ok {
			t.Fatalf("请求头 %s 残留：%q", h, v)
		}
	}
	if len(ev.RR.Req.Query) != 0 {
		t.Fatalf("查询串残留：%v", ev.RR.Req.Query)
	}
	if ev.RR.Req.Body.Len() != 0 {
		t.Fatalf("body 残留：%q", ev.RR.Req.Body.String())
	}
	if ev.RR.Req.Path != "/b" || ev.RR.Req.GetHost() != "second.example" {
		t.Fatalf("标量字段未更新：path=%q host=%q",
			ev.RR.Req.Path, ev.RR.Req.GetHost())
	}

	if len(ev.RR.Res.Headers) != 0 {
		t.Fatalf("响应头残留：%v", ev.RR.Res.Headers)
	}
	if ev.RR.Res.FirstLine != "" {
		t.Fatalf("响应状态行残留：%q", ev.RR.Res.FirstLine)
	}
	if ev.RR.OriginPath != "" || ev.RR.PathLocation != nil {
		t.Fatalf("路径匹配状态残留：origin=%q loc=%v",
			ev.RR.OriginPath, ev.RR.PathLocation)
	}
	if ev.Upgrade != "" {
		t.Fatalf("升级标记残留：%q", ev.Upgrade)
	}
}
