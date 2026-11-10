package proxy

import (
	"net"
	"testing"

	"fast-https/config"
	"fast-https/runtime/listener"
)

func TestFailoverUsesNextPeer(t *testing.T) {
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

	ev := newProxyEvent(t)
	ev.RR.Req.Method = "GET"
	ev.RR.Req.Path = "/"
	ev.RR.Req.Protocol = "HTTP/1.1"
	cfg := &listener.ListenCfg{
		Type:       config.PROXY_HTTP,
		ProxyAddr:  "127.0.0.1:1",
		ProxyPeers: []string{"127.0.0.1:1", ln.Addr().String()},
	}
	ProxyEvent(cfg, ev)
	if ev.RR.Res == nil || ev.RR.Res.FirstLine == "" {
		t.Fatal("no response after failover")
	}
	if ev.RR.Res.FirstLine[:12] != "HTTP/1.1 200" && len(ev.RR.Res.FirstLine) >= 12 {
		t.Fatalf("status line = %q", ev.RR.Res.FirstLine)
	}
}

func TestDrainRefusesLastPeer(t *testing.T) {
	groups := [][]string{{"10.0.0.1:8080"}, {"10.0.0.1:8080", "10.0.0.2:8080"}}
	if err := EvaluateDrain(groups, nil, "10.0.0.1:8080"); err != ErrLastUpstream {
		t.Fatalf("err = %v", err)
	}
	if err := EvaluateDrain([][]string{{"http://10.0.0.1:8080", "http://10.0.0.2:8080"}}, nil, "10.0.0.2:8080"); err != nil {
		t.Fatalf("two-peer drain = %v", err)
	}
}
