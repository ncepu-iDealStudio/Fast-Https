package proxy

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- 纯构造与错误对象 ----

func TestNewproxyFields(t *testing.T) {
	p := Newproxy("127.0.0.1:80", int(config.PROXY_HTTP), true)
	if p.ProxyAddr != "127.0.0.1:80" || p.ProxyType != int(config.PROXY_HTTP) || !p.ProxyNeedCache {
		t.Fatalf("Newproxy() = %+v", p)
	}
}

func TestProxyErrorMessage(t *testing.T) {
	if ProxyErrorDialUpstream.Error() == "" {
		t.Fatal("error message must not be empty")
	}
}

func TestDialFailureResponseUsesConfiguredPage(t *testing.T) {
	page := filepath.Join(t.TempDir(), "50x.html")
	os.WriteFile(page, []byte("<h1>custom upstream error</h1>"), 0o644)

	plain := DialFailureResponse(nil)
	if !strings.Contains(string(plain.Body), "500") && plain.FirstLine == "" {
		t.Fatalf("default dial failure response: %+v", plain)
	}

	cfg := &listener.ListenCfg{ErrorProxy: page}
	got := DialFailureResponse(cfg)
	if !strings.Contains(string(got.Body), "custom upstream error") {
		t.Fatalf("configured error page not used: %q", got.Body)
	}
}

// ---- ChangeHeader / ChangeHead ----

func TestChangeHeaderParsesStatusAndStripsServer(t *testing.T) {
	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Headers["Connection"] = "keep-alive"
	ev.RR.Res = response.ResponseInit()

	raw := "HTTP/1.1 404 Not Found\r\nServer: upstream/1.0\r\nContent-Length: 2\r\n\r\nhi"
	code := (&Proxy{}).ChangeHeader(true, []byte(raw), &ev.RR)
	if code != "404" {
		t.Fatalf("status = %q, want 404", code)
	}
	if ev.RR.Res.GetHeader("Server") != "Fast-Https" {
		t.Fatalf("Server header = %q, want Fast-Https", ev.RR.Res.GetHeader("Server"))
	}
	if ev.RR.Res.GetHeader("Content-Length") != "2" {
		t.Fatalf("upstream headers lost: %v", ev.RR.Res.Headers)
	}
}

func TestChangeHeaderGarbageYields509(t *testing.T) {
	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Res = response.ResponseInit()

	code := (&Proxy{}).ChangeHeader(false, []byte("garbage-without-spaces"), &ev.RR)
	if code != "509" {
		t.Fatalf("status = %q, want 509", code)
	}
}

func TestChangeHeadAppliesProxySetHeader(t *testing.T) {
	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Headers["Host"] = "client.example"
	ev.RR.Req.SetHeader("X-Plain", "world", nil)
	ev.RR.CircleCommandVal.Map = map[string]string{"host": "client.example"}

	cfg := &listener.ListenCfg{ProxySetHeader: []config.Header{
		{HeaderKey: "X-Forwarded-Host", HeaderValue: "$host"},
		{HeaderKey: "X-Static", HeaderValue: "static-value"},
	}}
	ChangeHead(cfg, ev)

	if got := ev.RR.Req.GetHeader("X-Forwarded-Host"); got != "client.example" {
		t.Fatalf("X-Forwarded-Host = %q, want client.example", got)
	}
	if got := ev.RR.Req.GetHeader("X-Static"); got != "static-value" {
		t.Fatalf("X-Static = %q", got)
	}
}

func TestProxyFilterHandlerPasses(t *testing.T) {
	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.RR.Req = request.RequestInit(false)
	if !ProxyFilterHandler(&listener.ListenCfg{}, ev) {
		t.Fatal("proxy filter must always pass")
	}
}

// ---- op.go 读写闭包（明文与 TLS）----

func TestPlainConnWrappers(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	p := Newproxy("", int(config.PROXY_HTTP), false)
	p.ProxyConn = client
	p.Read, p.Write, p.Close = p.read, p.write, p.close

	go func() { _, _ = server.Write([]byte("pong")) }()
	buf := make([]byte, 4)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := p.Read(buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "pong" {
		t.Fatalf("read %q", buf)
	}

	done := make(chan error, 1)
	go func() { _, err := server.Read(buf); done <- err }()
	if _, err := p.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if p.isClosed() {
		t.Fatal("connection should still be open")
	}
	_ = p.Close()
	if !p.isClosed() {
		t.Fatal("close() must mark the proxy closed")
	}
}

// startLocalTLS 起一个回环 TLS 回显上游。
func startLocalTLS(t *testing.T) (addr string, certPool *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	pool := x509.NewCertPool()
	pool.AddCert(&x509.Certificate{})

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		t.Skipf("loopback tls listen unavailable: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return ln.Addr().String(), nil
}

func TestSSLConnWrappers(t *testing.T) {
	addr, _ := startLocalTLS(t)
	raw, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	tlsConn := tls.Client(raw, &tls.Config{
		InsecureSkipVerify: true, // 自签名测试证书
		MinVersion:         tls.VersionTLS12,
	})
	t.Cleanup(func() { _ = tlsConn.Close() })

	p := Newproxy(addr, int(config.PROXY_HTTPS), false)
	p.ProxyTlsConn = tlsConn
	p.Read, p.Write, p.Close = p.readSSL, p.writeSSL, p.closeSSL

	if _, err := p.Write([]byte("hello-tls")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 9)
	tlsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := p.Read(buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "hello-tls" {
		t.Fatalf("echo = %q", buf)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if !p.isClosed() {
		t.Fatal("closeSSL must mark closed")
	}
}

// ---- getProxy / releaseProxy 连接池行为 ----

func TestGetProxyPoolsAndReusesConnection(t *testing.T) {
	// 上游是一个回环 HTTP 服务器
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				buf := bufio.NewReader(conn)
				for {
					line, err := buf.ReadString('\n')
					if err != nil {
						return
					}
					if strings.HasPrefix(line, "PING") {
						conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nPONG"))
						return
					}
				}
			}()
		}
	}()

	cfg := &listener.ListenCfg{ProxyAddr: ln.Addr().String()}
	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.RR.Req = request.RequestInit(false)

	p1, err := getProxy(ev, &ev.RR, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p1.reused {
		t.Fatal("first borrow must be a fresh connection")
	}
	if _, err := p1.Write([]byte("PING\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	p1.ProxyConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := p1.Read(buf); err != nil {
		t.Fatal(err)
	}
	releaseProxy(p1)

	p2, err := getProxy(ev, &ev.RR, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseProxy(p2)
	if !p2.reused {
		t.Fatal("second borrow should reuse the pooled connection")
	}
	if p1.ProxyConn != p2.ProxyConn {
		t.Fatal("pooled connection identity changed")
	}
}

func TestReleaseProxyDiscardsClosedAndNeedClose(t *testing.T) {
	// 已关闭的连接不得归池（先装配闭包，Newproxy 不自带）
	closed, closedPeer := net.Pipe()
	t.Cleanup(func() { _ = closedPeer.Close() })
	p := Newproxy("x", int(config.PROXY_HTTP), false)
	p.ProxyConn = closed
	p.Read, p.Write, p.Close = p.read, p.write, p.close
	_ = p.Close()
	releaseProxy(p) // 不应 panic，也不应入池

	// ProxyNeedClose 的连接归还时直接关闭
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	p2 := Newproxy("y", int(config.PROXY_HTTP), false)
	p2.ProxyConn = client
	p2.Read, p2.Write, p2.Close = p2.read, p2.write, p2.close
	p2.ProxyNeedClose = true
	releaseProxy(p2)
	if !p2.isClosed() {
		t.Fatal("ProxyNeedClose connection must be closed on release")
	}
}

func TestGetProxyDialFailure(t *testing.T) {
	cfg := &listener.ListenCfg{ProxyAddr: "127.0.0.1:1"} // 无人监听
	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.RR.Req = request.RequestInit(false)
	if _, err := getProxy(ev, &ev.RR, cfg); err != ProxyErrorDialUpstream {
		t.Fatalf("dial failure err = %v, want ProxyErrorDialUpstream", err)
	}
}

// websocket 分支：连接由 CircleData 整条持有，不进池
func TestGetProxyWebSocketHoldsConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(io.Discard, conn) }()
		}
	}()

	ev := core.NewEvent(&listener.Listener{}, nil)
	ev.Upgrade = "websocket"
	ev.RR.Req = request.RequestInit(false)
	cfg := &listener.ListenCfg{ProxyAddr: ln.Addr().String()}

	p1, err := getProxy(ev, &ev.RR, cfg)
	if err != nil {
		t.Fatal(err)
	}
	releaseProxy(p1) // websocket：归还必须是 no-op

	p2, err := getProxy(ev, &ev.RR, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 {
		t.Fatal("websocket session must keep the same upstream connection")
	}
	if !ev.RR.ProxyConnInit {
		t.Fatal("ProxyConnInit must be set for websocket sessions")
	}
}

// ---- sendToUpstreamServer / ProxyInit ----

func TestSendToUpstreamClosedConnFails(t *testing.T) {
	client, server := net.Pipe()
	_ = client.Close()
	_ = server.Close()
	p := Newproxy("", int(config.PROXY_HTTP), false)
	p.ProxyConn = client
	p.Read, p.Write, p.Close = p.read, p.write, p.close
	if err := p.sendToUpstreamServer([]byte("x")); err == nil {
		t.Fatal("write to closed connection must fail")
	}
}

func TestProxyInitDialsAndEquips(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(io.Discard, conn) }()
		}
	}()

	p := Newproxy(ln.Addr().String(), int(config.PROXY_HTTP), false)
	ev := core.NewEvent(&listener.Listener{}, nil)
	if err := p.ProxyInit(ev); err != nil {
		t.Fatalf("ProxyInit: %v", err)
	}
	if p.ProxyConn == nil || p.Read == nil || p.Write == nil || p.Close == nil {
		t.Fatal("ProxyInit must equip conn and read/write/close")
	}
	_ = p.Close()

	bad := Newproxy("127.0.0.1:1", int(config.PROXY_HTTP), false)
	if err := bad.ProxyInit(nil); err != ProxyErrorDialUpstream {
		t.Fatalf("dial failure err = %v", err)
	}
}
