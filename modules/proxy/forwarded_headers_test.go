package proxy

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
)

// echoUpstream 起一个回环上游，把收到的请求头原样回显给测试。
// 返回上游地址与「取回显内容」的通道。
func echoUpstream(t *testing.T) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	got := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		buf := bufio.NewReader(conn)
		var sb strings.Builder
		for {
			line, err := buf.ReadString('\n')
			sb.WriteString(line)
			if err != nil || strings.TrimSpace(line) == "" {
				break
			}
		}
		got <- sb.String()
	}()
	return ln.Addr().String(), got
}

// TestProxyForwardsHopByHopHeadersVerbatim 记录当前行为：代理把客户端请求
// 的字节原样写给上游，不做 hop-by-hop 头剥离。
//
// 这是一个「锁定现状」的用例，不是「期望行为」：按 RFC 7230 §6.1，
// Connection / Keep-Alive / Proxy-Authorization / TE 这类头只对单跳有意义，
// 转发前应当剥掉。当前实现缺少这一步，见 plans/edge-case-hardening-plan.md
// 的 R-06。等实现补上后，这个用例应当改成断言「上游收不到这些头」。
func TestProxyForwardsHopByHopHeadersVerbatim(t *testing.T) {
	addr, got := echoUpstream(t)

	cfg := &listener.ListenCfg{ProxyAddr: addr}
	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.RR.Req = request.RequestInit(false)

	p, err := getProxy(ev, &ev.RR, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { releaseProxy(p) })

	req := "GET /x HTTP/1.1\r\n" +
		"Host: upstream.example\r\n" +
		"Connection: keep-alive, X-Hop\r\n" +
		"X-Hop: should-be-dropped\r\n" +
		"Proxy-Authorization: Basic Zm9vOmJhcg==\r\n" +
		"TE: trailers\r\n" +
		"\r\n"
	if _, err := p.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}

	select {
	case raw := <-got:
		for _, h := range []string{"Connection:", "X-Hop:", "Proxy-Authorization:", "TE:"} {
			if !strings.Contains(raw, h) {
				// 已经剥离了——说明 R-06 实现完成，把本用例改成反向断言
				t.Fatalf("%s 已被剥离，请把本用例改为断言「上游收不到 hop-by-hop 头」", h)
			}
		}
		t.Logf("现状：hop-by-hop 头被原样转发（%d 字节）", len(raw))
	case <-time.After(3 * time.Second):
		t.Fatal("上游没有收到请求")
	}
}

// TestProxyDoesNotAddXForwardedFor 记录当前行为：代理不添加 X-Forwarded-For，
// 也不修改 Host。
//
// 因此「客户端伪造 XFF 导致上游看到假链路」这条反馈在本项目不成立
// （根本没有这个头）；代价是上游拿不到真实来源，属于功能缺失，
// 见计划里的 R-05。
func TestProxyDoesNotAddXForwardedFor(t *testing.T) {
	addr, got := echoUpstream(t)

	cfg := &listener.ListenCfg{ProxyAddr: addr}
	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.RR.Req = request.RequestInit(false)

	p, err := getProxy(ev, &ev.RR, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { releaseProxy(p) })

	// 客户端自带一个伪造的 XFF
	req := "GET /x HTTP/1.1\r\n" +
		"Host: upstream.example\r\n" +
		"X-Forwarded-For: 10.0.0.1\r\n" +
		"\r\n"
	if _, err := p.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}

	select {
	case raw := <-got:
		if strings.Contains(raw, "X-Forwarded-For: 10.0.0.1, ") ||
			strings.Contains(raw, "X-Forwarded-For: 10.0.0.1,") {
			t.Fatal("代理追加了客户端地址，实现已改变，请更新核验表与 R-05")
		}
		t.Logf("现状：XFF 原样透传（未追加真实来源），需要 R-05")
	case <-time.After(3 * time.Second):
		t.Fatal("上游没有收到请求")
	}
}
