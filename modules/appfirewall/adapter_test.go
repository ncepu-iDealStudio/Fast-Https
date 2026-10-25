package appfirewall

import (
	"net"
	"testing"
	"time"

	"fast-https/core"
	"fast-https/runtime/request"
)

// nopConn 只提供地址与空读写，测试中不真正传输。
type nopConn struct {
	remote net.Addr
}

func (c *nopConn) Read(b []byte) (int, error)       { return 0, net.ErrClosed }
func (c *nopConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *nopConn) Close() error                     { return nil }
func (c *nopConn) LocalAddr() net.Addr              { return c.remote }
func (c *nopConn) RemoteAddr() net.Addr             { return c.remote }
func (c *nopConn) SetDeadline(time.Time) error      { return nil }
func (c *nopConn) SetReadDeadline(time.Time) error  { return nil }
func (c *nopConn) SetWriteDeadline(time.Time) error { return nil }

func newTestAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 52341}
}

func testEvent(method, uri, body string) *core.Event {
	ev := &core.Event{Conn: &nopConn{remote: newTestAddr()}}
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = method
	ev.RR.Req.Path = uri
	ev.RR.Req.Headers["Host"] = "www.example.com"
	ev.RR.Req.Headers["User-Agent"] = "waf-test"
	if body != "" {
		ev.RR.Req.Body.WriteString(body)
	}
	// 拒绝路径会写 403；测试里吞掉写出
	ev.EventWrite = func(*core.Event, []byte) error { return nil }
	return ev
}

// 适配层字段映射
func TestToHTTPRequestFieldMapping(t *testing.T) {
	ev := testEvent("POST", "/api/submit?x=1&y=2", `{"q":"1"}`)

	r := toHTTPRequest(ev)
	if r.Method != "POST" {
		t.Errorf("Method = %q", r.Method)
	}
	if r.URL.Path != "/api/submit" || r.URL.RawQuery != "x=1&y=2" {
		t.Errorf("URL.Path = %q RawQuery = %q", r.URL.Path, r.URL.RawQuery)
	}
	if r.Host != "www.example.com" {
		t.Errorf("Host = %q", r.Host)
	}
	if r.Header.Get("User-Agent") != "waf-test" {
		t.Errorf("Header User-Agent = %q", r.Header.Get("User-Agent"))
	}
	if r.RemoteAddr == "" {
		t.Error("RemoteAddr should be populated")
	}
	if r.ProtoMajor != 1 {
		t.Errorf("ProtoMajor = %d", r.ProtoMajor)
	}
	body := make([]byte, 16)
	n, _ := r.Body.Read(body)
	if string(body[:n]) != `{"q":"1"}` {
		t.Errorf("body = %q", body[:n])
	}
}

// h2 请求的 Proto 映射
func TestToHTTPRequestH2Proto(t *testing.T) {
	ev := testEvent("GET", "/", "")
	ev.RR.Req.H2 = true
	if r := toHTTPRequest(ev); r.ProtoMajor != 2 {
		t.Errorf("h2 ProtoMajor = %d, want 2", r.ProtoMajor)
	}
}

// wafConfigured 对未知规则名只告警忽略，不构建未知检测器
func TestIsKnownDetector(t *testing.T) {
	for _, name := range wafDetectorNames {
		if !isKnownDetector(name) {
			t.Errorf("detector %q must be known", name)
		}
	}
	for _, name := range []string{"", "sql", "exec", "not-a-detector"} {
		if isKnownDetector(name) {
			t.Errorf("%q must not be a known detector", name)
		}
	}
}
