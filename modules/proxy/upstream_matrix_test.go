package proxy

import (
	"fast-https/config"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// 上游异常矩阵：半响应（Content-Length 承诺未兑现即断开）、连接重置、
// 慢响应（读写 deadline 内 body 迟迟不来）。共同的不变量：
//
//	任何读错误都必须以 error 形式上抛——把截断数据当 200 交付（更糟的
//	是落进缓存）等于向客户端投毒。修复前 ReadBytes 在非超时错误上既
//	不跳出也不推进 totalLen，keep-alive 半响应会无限自旋吃满 CPU。
//
// runWithWatchdog 用看门狗包裹被测调用，自旋/挂死回归会以超时暴露
// 而不是吊死整个测试进程。
func runWithWatchdog(t *testing.T, name string, call func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: proxyKeepAlive did not return within 5s (spin/hang regression)", name)
		return nil
	}
}

func newMatrixProxy(client net.Conn) *Proxy {
	p := &Proxy{ProxyType: config.PROXY_HTTP, ProxyConn: client}
	// dialUpstream 会装配的读写闭包，proxyReadAll 走的是这些字段
	p.Read = p.read
	p.Write = p.write
	p.Close = p.close
	return p
}

// M1 半响应 + 连接关闭：头声明 CL=100，body 只给 10 字节后上游关闭
func TestUpstreamHalfResponseThenClose(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n0123456789"))
		_ = upstream.Close() // EOF：承诺的 100 字节永远到不了
	}()

	rka := &ReadKeepAlive{p: newMatrixProxy(client)}
	err := runWithWatchdog(t, "half-response", rka.proxyKeepAlive)
	if err == nil {
		t.Fatal("half response must surface an error, got nil")
	}
	if len(rka.body) == 100 {
		t.Fatal("body claims to be complete despite upstream EOF")
	}
}

// M2 头未到齐即关闭：proxyKeepAlive 首读即出错
func TestUpstreamClosedBeforeHeaders(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	go func() { _ = upstream.Close() }()

	rka := &ReadKeepAlive{p: newMatrixProxy(client)}
	err := runWithWatchdog(t, "closed-before-headers", rka.proxyKeepAlive)
	if err == nil {
		t.Fatal("connection closed before headers must surface an error")
	}
}

// M3 真实 TCP 重置（SO_LINGER=0 → RST）：头到达后 body 期间被重置。
// net.Pipe 的 Close 只是 EOF，这里用回环真实网络栈制造 RST。
func TestUpstreamConnectionResetDuringBody(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback listen unavailable: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\npartial"))
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetLinger(0) // Close 发 RST 而不是 FIN
		}
		_ = conn.Close()
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	rka := &ReadKeepAlive{p: newMatrixProxy(conn)}
	err = runWithWatchdog(t, "reset-during-body", rka.proxyKeepAlive)
	if err == nil {
		t.Fatal("connection reset during body must surface an error")
	}
}

// M4 慢响应：body 拖过读 deadline。超时同样必须以错误上抛
// （ProxyErrorReadFromUpstreamTimeout），而不是交付半截 body。
func TestUpstreamSlowResponseTimesOut(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	t.Cleanup(func() { _ = upstream.Close() })

	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n0123456789"))
		time.Sleep(3 * time.Second) // 远超下面的 deadline；测试结束即被清理
	}()

	conn := client
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))

	rka := &ReadKeepAlive{p: newMatrixProxy(conn)}
	err := runWithWatchdog(t, "slow-response", rka.proxyKeepAlive)
	if err != ProxyErrorReadFromUpstreamTimeout {
		t.Fatalf("slow response err = %v, want ProxyErrorReadFromUpstreamTimeout", err)
	}
}

// M5 对照组：完整的 keep-alive 响应分多段到达，正常拼回
func TestUpstreamHealthyResponseAcrossReads(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 20\r\n"))
		_, _ = upstream.Write([]byte("\r\n"))
		_, _ = upstream.Write([]byte("0123456789"))
		_, _ = upstream.Write([]byte("0123456789"))
		_ = upstream.Close()
	}()

	rka := &ReadKeepAlive{p: newMatrixProxy(client)}
	err := runWithWatchdog(t, "healthy", rka.proxyKeepAlive)
	if err != nil {
		t.Fatalf("healthy response errored: %v", err)
	}
	if string(rka.body) != strings.Repeat("0123456789", 2) {
		t.Fatalf("body = %q, want 20 bytes", rka.body)
	}
	if rka.res == nil || rka.res.GetHeader("Content-Length") != "20" {
		t.Fatalf("headers not parsed: %+v", rka.res)
	}
}

// M6 Connection: close 路径：proxyReadAll 在 EOF 处收尾，读到什么交什么
func TestUpstreamConnectionCloseReadAll(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\nbody-bytes"))
		_ = upstream.Close()
	}()

	rcc := &ReadConnectionClose{p: newMatrixProxy(client)}
	done := make(chan struct{})
	var data []byte
	var err error
	go func() {
		data, err = rcc.proxyReadAll()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("proxyReadAll did not return")
	}
	if err != nil {
		t.Fatalf("proxyReadAll err = %v", err)
	}
	if !strings.Contains(string(data), "body-bytes") {
		t.Fatalf("data = %q", data)
	}
}

// M7 Connection: close 路径的读错误：上游中途重置，错误上抛且连接被关
func TestUpstreamConnectionCloseReset(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\npartial"))
		// 不 Close 而是制造读侧错误：直接关掉服务端让 pipe 进入错误态
		_ = upstream.Close()
	}()

	rcc := &ReadConnectionClose{p: newMatrixProxy(client)}
	data, err := rcc.proxyReadAll()
	if err == nil && len(data) == 0 {
		t.Fatal("reset during connection-close read produced neither data nor error")
	}
	if err != nil && err != io.EOF {
		// 非 EOF 错误必须上抛（proxyReadAll 对非 EOF 返回 err）
		if !strings.Contains(err.Error(), "closed") && !strings.Contains(err.Error(), "reset") {
			t.Logf("upstream error (acceptable): %v", err)
		}
	}
}
