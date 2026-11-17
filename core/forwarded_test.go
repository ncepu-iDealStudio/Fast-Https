package core

import (
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"fast-https/runtime/listener"
	"fast-https/runtime/request"
)

func TestForwardedForChainPolicy(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	cases := []struct {
		name    string
		peer    string
		inbound string
		cfg     listener.ListenCfg
		want    string
	}{
		{
			name: "不受信对端丢弃入站伪造链",
			peer: "203.0.113.9", inbound: "10.0.0.1",
			cfg:  listener.ListenCfg{},
			want: "203.0.113.9",
		},
		{
			name: "受信对端保留并追加",
			peer: "10.0.0.5", inbound: "198.51.100.7",
			cfg:  listener.ListenCfg{TrustedProxies: trusted},
			want: "198.51.100.7, 10.0.0.5",
		},
		{
			name: "达到链长上限不再追加",
			peer: "10.0.0.5", inbound: "198.51.100.7, 10.0.0.9",
			cfg:  listener.ListenCfg{TrustedProxies: trusted, ForwardedForMaxHops: 2},
			want: "198.51.100.7, 10.0.0.9",
		},
		{
			name: "0 表示不限制",
			peer: "10.0.0.5", inbound: "198.51.100.7, 10.0.0.9",
			cfg:  listener.ListenCfg{TrustedProxies: trusted, ForwardedForMaxHops: 0},
			want: "198.51.100.7, 10.0.0.9, 10.0.0.5",
		},
		{
			name: "IPv4-mapped 对端按 IPv4 匹配",
			peer: "::ffff:10.0.0.5", inbound: "198.51.100.7",
			cfg:  listener.ListenCfg{TrustedProxies: trusted},
			want: "198.51.100.7, ::ffff:10.0.0.5",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev := NewEvent(&listener.Listener{}, &testConn{remote: c.peer})
			ev.RR.Req = request.RequestInit(false)
			ev.RR.Req.Path = "/p"
			if c.inbound != "" {
				ev.RR.Req.SetHeader("X-Forwarded-For", c.inbound, &c.cfg)
			}
			if got := ForwardedForChain(&c.cfg, ev); got != c.want {
				t.Fatalf("ForwardedForChain = %q, want %q", got, c.want)
			}
		})
	}
}

func TestClientIPHandlesIPv6(t *testing.T) {
	ev := NewEvent(&listener.Listener{}, &testConn{remote: "2001:db8::1"})
	if got := ClientIP(ev); got != "2001:db8::1" {
		t.Fatalf("ClientIP = %q", got)
	}
}

type testConn struct{ remote string }

func (c *testConn) Read([]byte) (int, error)         { return 0, os.ErrClosed }
func (c *testConn) Write([]byte) (int, error)        { return 0, os.ErrClosed }
func (c *testConn) Close() error                     { return nil }
func (c *testConn) LocalAddr() net.Addr              { return nil }
func (c *testConn) RemoteAddr() net.Addr             { return testAddr(c.remote) }
func (c *testConn) SetDeadline(time.Time) error      { return nil }
func (c *testConn) SetReadDeadline(time.Time) error  { return nil }
func (c *testConn) SetWriteDeadline(time.Time) error { return nil }

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return net.JoinHostPort(string(a), "443") }
