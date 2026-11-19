package proxy

import (
	"bufio"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
)

// forwardedCase 是一条 X-Forwarded-For 策略输入，字段见
// testdata/forwarded-chain.jsonl。
type forwardedCase struct {
	Name           string   `json:"name"`
	ClientIP       string   `json:"client_ip"`
	InboundXFF     string   `json:"inbound_xff"`
	TrustedProxies []string `json:"trusted_proxies"`
	MaxHops        int      `json:"max_hops"`
	WantXFF        string   `json:"want_xff"`
}

// TestForwardedChainCases 用 forwarded-chain.jsonl 锁定四类输入：
// 无 XFF、客户端伪造、多层代理、超跳数。
func TestForwardedChainCases(t *testing.T) {
	cases := readForwardedCases(t)
	if len(cases) < 4 {
		t.Fatalf("用例不足：%d", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			cfg := &listener.ListenCfg{PreserveHost: true, ForwardedForMaxHops: c.MaxHops}
			for _, raw := range c.TrustedProxies {
				prefix, err := netip.ParsePrefix(raw)
				if err != nil {
					t.Fatalf("用例里的 CIDR 不合法 %q：%v", raw, err)
				}
				cfg.TrustedProxies = append(cfg.TrustedProxies, prefix)
			}
			ev := core.NewEvent(&listener.Listener{}, &stubConn{remote: c.ClientIP})
			ev.RR.Req = request.RequestInit(false)
			ev.RR.Req.Path = "/f"
			ev.RR.Req.SetHeader("Host", "upstream.example", cfg)
			if c.InboundXFF != "" {
				ev.RR.Req.SetHeader("X-Forwarded-For", c.InboundXFF, cfg)
			}

			ChangeHead(cfg, ev)

			if got := ev.RR.Req.GetHeader("X-Forwarded-For"); got != c.WantXFF {
				t.Fatalf("X-Forwarded-For = %q, want %q", got, c.WantXFF)
			}
		})
	}
}

// TestPreserveHostDisabled 断言显式关闭时 Host 改成上游地址。
func TestPreserveHostDisabled(t *testing.T) {
	cfg := &listener.ListenCfg{ProxyAddr: "http://127.0.0.1:8081/api", PreserveHost: false}
	ev := core.NewEvent(&listener.Listener{}, &stubConn{remote: "203.0.113.9"})
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.SetHeader("Host", "client.example", cfg)

	ChangeHead(cfg, ev)

	if got := ev.RR.Req.GetHeader("Host"); got != "127.0.0.1:8081" {
		t.Fatalf("Host = %q, want upstream host", got)
	}
}

// TestPreserveHostDefault 断言默认行为是原样透传客户端 Host。
func TestPreserveHostDefault(t *testing.T) {
	cfg := &listener.ListenCfg{ProxyAddr: "127.0.0.1:8081", PreserveHost: true}
	ev := core.NewEvent(&listener.Listener{}, &stubConn{remote: "203.0.113.9"})
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.SetHeader("Host", "client.example", cfg)

	ChangeHead(cfg, ev)

	if got := ev.RR.Req.GetHeader("Host"); got != "client.example" {
		t.Fatalf("Host = %q, want client Host", got)
	}
}

// TestExplicitProxySetHeaderOverridesXFF 断言显式 proxy_set_header 仍能覆盖
// 自动生成的 X-Forwarded-For（与 nginx 后写生效一致）。
func TestExplicitProxySetHeaderOverridesXFF(t *testing.T) {
	cfg := &listener.ListenCfg{
		PreserveHost: true,
		ProxySetHeader: []config.Header{
			{HeaderKey: "X-Forwarded-For", HeaderValue: "203.0.113.1"},
		},
	}
	ev := core.NewEvent(&listener.Listener{}, &stubConn{remote: "10.0.0.5"})
	ev.RR.Req = request.RequestInit(false)

	ChangeHead(cfg, ev)

	if got := ev.RR.Req.GetHeader("X-Forwarded-For"); got != "203.0.113.1" {
		t.Fatalf("显式配置应覆盖自动值，got %q", got)
	}
}

func readForwardedCases(t *testing.T) []forwardedCase {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "forwarded-chain.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []forwardedCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var c forwardedCase
		if err := json.Unmarshal(line, &c); err != nil {
			t.Fatalf("解析用例失败：%v", err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases
}

// stubConn 只提供 RemoteAddr，用于构造事件；其余方法不会被请求策略调用。
type stubConn struct{ remote string }

func (c *stubConn) Read([]byte) (int, error)         { return 0, os.ErrClosed }
func (c *stubConn) Write([]byte) (int, error)        { return 0, os.ErrClosed }
func (c *stubConn) Close() error                     { return nil }
func (c *stubConn) LocalAddr() net.Addr              { return nil }
func (c *stubConn) RemoteAddr() net.Addr             { return stubAddr(c.remote) }
func (c *stubConn) SetDeadline(time.Time) error      { return nil }
func (c *stubConn) SetReadDeadline(time.Time) error  { return nil }
func (c *stubConn) SetWriteDeadline(time.Time) error { return nil }

type stubAddr string

func (a stubAddr) Network() string { return "tcp" }
func (a stubAddr) String() string  { return net.JoinHostPort(string(a), "54321") }
