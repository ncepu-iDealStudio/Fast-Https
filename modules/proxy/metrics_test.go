package proxy

import (
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/metrics"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

func newProxyEvent(t *testing.T) *core.Event {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	go func() {
		_, _ = io.Copy(io.Discard, client)
	}()
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Res = response.ResponseInit()
	ev.EventWrite = func(*core.Event, []byte) error { return nil }
	return ev
}

func TestDialFailCounted(t *testing.T) {
	ResetMetricsForTest()
	cfg := &listener.ListenCfg{ProxyAddr: "127.0.0.1:1"}
	ev := newProxyEvent(t)

	if _, err := getProxy(ev, &ev.RR, cfg); err != ProxyErrorDialUpstream {
		t.Fatalf("err = %v, want ProxyErrorDialUpstream", err)
	}
	st, _ := Snapshot()
	if st.DialFailTotal != 1 {
		t.Fatalf("dial_fail_total = %d, want 1", st.DialFailTotal)
	}

	ProxyEvent(cfg, ev)
	st, _ = Snapshot()
	if st.DialFailTotal != 2 {
		t.Fatalf("ProxyEvent dial_fail_total = %d, want 2", st.DialFailTotal)
	}
	if ev.RR.Res == nil || ev.RR.Res.FirstLine == "" {
		t.Fatal("dial failure must still write the existing error response")
	}
}

func TestWebsocketConvertNotDial(t *testing.T) {
	ResetMetricsForTest()
	ev := newProxyEvent(t)
	ev.Upgrade = "websocket"
	ev.RR.ProxyConnInit = true
	ev.RR.CircleData = "not-a-proxy"
	if _, err := getProxy(ev, &ev.RR, &listener.ListenCfg{ProxyAddr: "127.0.0.1:1"}); err != ProxyErrorDialUpstream {
		t.Fatalf("err = %v, want ProxyErrorDialUpstream", err)
	}
	st, _ := Snapshot()
	if st.DialFailTotal != 0 {
		t.Fatalf("websocket convert must not count as dial_fail, got %d", st.DialFailTotal)
	}
}

func TestReusedConnRetrySucceeds(t *testing.T) {
	ResetMetricsForTest()
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
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
				_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"))
			}(conn)
		}
	}()

	dead, peer := net.Pipe()
	t.Cleanup(func() { _ = dead.Close() })
	_ = peer.Close()

	ev := newProxyEvent(t)

	p := Newproxy(ln.Addr().String(), int(config.PROXY_HTTP), false)
	p.ProxyConn = dead
	p.Read, p.Write, p.Close = p.read, p.write, p.close
	p.reused = true
	p.ev = ev

	_, err = p.getDataFromServer(ev, []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
	if err != nil {
		t.Fatalf("retry should succeed, err = %v", err)
	}
	st, _ := Snapshot()
	if st.DialFailTotal != 0 || st.SendTimeoutTotal != 0 || st.ReadTimeoutTotal != 0 {
		t.Fatalf("successful retry counted as failure: %+v", st)
	}
}

func TestReusedConnRetryDialFails(t *testing.T) {
	ResetMetricsForTest()
	dead, peer := net.Pipe()
	t.Cleanup(func() { _ = dead.Close() })
	_ = peer.Close()

	ev := newProxyEvent(t)

	p := Newproxy("127.0.0.1:1", int(config.PROXY_HTTP), false)
	p.ProxyConn = dead
	p.Read, p.Write, p.Close = p.read, p.write, p.close
	p.reused = true
	p.ev = ev

	_, err := p.getDataFromServer(ev, []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
	if err != ProxyErrorDialUpstream {
		t.Fatalf("err = %v, want ProxyErrorDialUpstream", err)
	}
	st, _ := Snapshot()
	if st.DialFailTotal != 1 {
		t.Fatalf("dial_fail_total = %d, want 1", st.DialFailTotal)
	}
	if st.SendTimeoutTotal != 0 || st.ReadTimeoutTotal != 0 {
		t.Fatalf("only dial_fail should increase: %+v", st)
	}
}

func TestReadTimeoutCountedOnce(t *testing.T) {
	ResetMetricsForTest()
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = upstream.Close() })
	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n0123456789"))
		time.Sleep(3 * time.Second)
	}()
	_ = client.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	rka := &ReadKeepAlive{p: newMatrixProxy(client)}
	err := runWithWatchdog(t, "slow-response-metrics", rka.proxyKeepAlive)
	if err != ProxyErrorReadFromUpstreamTimeout {
		t.Fatalf("err = %v, want ProxyErrorReadFromUpstreamTimeout", err)
	}
	st, _ := Snapshot()
	if st.ReadTimeoutTotal != 1 {
		t.Fatalf("read_timeout_total = %d, want 1", st.ReadTimeoutTotal)
	}
	if st.DialFailTotal != 0 {
		t.Fatalf("read timeout must not count as dial_fail, got %d", st.DialFailTotal)
	}
}

func TestCountersSurviveReload(t *testing.T) {
	ResetMetricsForTest()
	ev := newProxyEvent(t)
	if _, err := getProxy(ev, &ev.RR, &listener.ListenCfg{ProxyAddr: "127.0.0.1:1"}); err != ProxyErrorDialUpstream {
		t.Fatal(err)
	}
	before, _ := Snapshot()
	metrics.NoteReloadSuccess()
	after, _ := Snapshot()
	if after.DialFailTotal != before.DialFailTotal || after.DialFailTotal != 1 {
		t.Fatalf("reload reset upstream counters: before=%+v after=%+v", before, after)
	}
}

func TestDialFailSplitByAddress(t *testing.T) {
	ResetMetricsForTest()
	ev1 := newProxyEvent(t)
	ev2 := newProxyEvent(t)
	if _, err := getProxy(ev1, &ev1.RR, &listener.ListenCfg{ProxyAddr: "127.0.0.1:1"}); err != ProxyErrorDialUpstream {
		t.Fatal(err)
	}
	if _, err := getProxy(ev2, &ev2.RR, &listener.ListenCfg{ProxyAddr: "127.0.0.2:1"}); err != ProxyErrorDialUpstream {
		t.Fatal(err)
	}
	st, _ := Snapshot()
	if st.DialFailTotal != 2 || len(st.Upstreams) != 2 {
		t.Fatalf("stats = %+v, want total 2 and two addresses", st)
	}
	if st.Upstreams[0].Address != "127.0.0.1:1" || st.Upstreams[1].Address != "127.0.0.2:1" {
		t.Fatalf("addresses = %+v", st.Upstreams)
	}
	if st.Upstreams[0].DialFailTotal != 1 || st.Upstreams[1].DialFailTotal != 1 {
		t.Fatalf("per-address dial = %+v", st.Upstreams)
	}
}

func TestMetricAddrDropsUserinfo(t *testing.T) {
	ResetMetricsForTest()
	noteDialFail("http://user:s3cret@10.1.2.3:9000/api")
	st, _ := Snapshot()
	if st.DialFailTotal != 1 || len(st.Upstreams) != 1 {
		t.Fatalf("stats = %+v", st)
	}
	if st.Upstreams[0].Address != "10.1.2.3:9000" {
		t.Fatalf("address = %q", st.Upstreams[0].Address)
	}
	raw := st.Upstreams[0].Address
	if strings.Contains(raw, "user") || strings.Contains(raw, "s3cret") || strings.Contains(raw, "/api") {
		t.Fatalf("address leaked userinfo or path: %q", raw)
	}
}

func TestUpstreamAddrCapKeepsTotal(t *testing.T) {
	ResetMetricsForTest()
	for i := 0; i < maxUpstreamAddrs+1; i++ {
		noteDialFail("10.0.0." + strconv.Itoa(i) + ":9")
	}
	st, _ := Snapshot()
	if st.DialFailTotal != uint64(maxUpstreamAddrs+1) {
		t.Fatalf("total = %d", st.DialFailTotal)
	}
	if len(st.Upstreams) != maxUpstreamAddrs {
		t.Fatalf("listed = %d, want %d", len(st.Upstreams), maxUpstreamAddrs)
	}
}

func TestPoolIdleCounter(t *testing.T) {
	now := time.Now()
	withFakeNow(t, &now)
	cp := newConnPool(8, 30*time.Second)

	p1, _ := newPooledProxy(t)
	p2, _ := newPooledProxy(t)
	p3, _ := newPooledProxy(t)
	cp.put("up", p1)
	cp.put("up", p2)
	cp.put("up", p3)
	if cp.idleCount.Load() != 3 || cp.idleLen() != 3 {
		t.Fatalf("after put idleCount=%d idleLen=%d, want 3/3", cp.idleCount.Load(), cp.idleLen())
	}

	got, err := cp.get("up", func() (*Proxy, error) {
		t.Fatal("should not dial")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if cp.idleCount.Load() != 2 || cp.idleLen() != 2 {
		t.Fatalf("after get idleCount=%d idleLen=%d, want 2/2", cp.idleCount.Load(), cp.idleLen())
	}
	cp.put("up", got)
	if cp.idleCount.Load() != 3 || cp.idleLen() != 3 {
		t.Fatalf("after put-back idleCount=%d idleLen=%d, want 3/3", cp.idleCount.Load(), cp.idleLen())
	}

	now = now.Add(time.Minute)
	n := cp.sweepExpired()
	if n != 3 {
		t.Fatalf("sweepExpired = %d, want 3", n)
	}
	if cp.idleCount.Load() != 0 || cp.idleLen() != 0 {
		t.Fatalf("after sweep idleCount=%d idleLen=%d, want 0/0", cp.idleCount.Load(), cp.idleLen())
	}
}
