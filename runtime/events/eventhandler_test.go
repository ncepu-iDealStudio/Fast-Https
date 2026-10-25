package events

import (
	"io"
	"net"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

// EventHandler 全链路：filter 链 → 路由匹配 → 钩子 → 注册的模块处理器。
// 用一个桩处理器占据未注册的 DEVMOD 类型码，不依赖任何 modules 包。
func TestEventHandlerDispatchesToRegisteredHandler(t *testing.T) {
	var dispatched atomic.Int32
	core.RegisterHandler(config.DEVMOD,
		func(cfg *listener.ListenCfg, ev *core.Event) bool { return true },
		func(cfg *listener.ListenCfg, ev *core.Event) {
			dispatched.Add(1)
			ev.RR.Res = response.ResponseInit()
			ev.RR.Res.SetFirstLine(200, "OK")
			ev.RR.Res.SetHeader("Content-Length", "10")
			ev.RR.Res.Body = []byte("dispatched")
			ev.WriteResponseClose(nil)
		},
		func(cfg *listener.ListenCfg, ev *core.Event) { // 自定义命令解析器
			ev.RR.CircleCommandVal.Map = map[string]string{"stub": "1"}
		})
	t.Cleanup(func() { core.RegisterHandler(config.DEVMOD, nil, nil, nil) })

	// 手工构造的 Listener（未 publish）回退读 HostMap 字段
	re := regexp.MustCompile("^/app")
	lis := &listener.Listener{Port: "18080", HostMap: map[string][]listener.ListenCfg{
		"example.com": {{
			ID: 1, Path: "/app", PathRe: re, Type: config.DEVMOD,
		}},
	}}

	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(lis, server)
	ev.EventWrite = func(ev *core.Event, _ []byte) error {
		ev.Conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err := ev.Conn.Write(ev.ResponseBytes())
		return err
	}
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "GET"
	ev.RR.Req.Path = "/app/page"
	ev.RR.Req.Headers["Host"] = "example.com"
	ev.RR.Req.Protocol = "HTTP/1.1"
	ev.RR.Res = response.ResponseInit()

	collect := startReader(t, client)
	EventHandler(ev, filters.NewFilter())
	got := collect()

	if dispatched.Load() != 1 {
		t.Fatalf("handler dispatched %d times, want 1", dispatched.Load())
	}
	if !strings.Contains(got, "dispatched") {
		t.Fatalf("response: %q", got)
	}
	// 命令解析器生效
	if ev.RR.CircleCommandVal.Map["stub"] != "1" {
		t.Fatal("custom parse-command handler not invoked")
	}
	// 路由记录：剩余路径
	if ev.RR.OriginPath != "/page" {
		t.Fatalf("OriginPath = %q, want /page", ev.RR.OriginPath)
	}
}

// 无匹配路由：404 + 连接关闭
func TestEventHandlerUnknownRouteYields404(t *testing.T) {
	lis := &listener.Listener{Port: "18080", HostMap: map[string][]listener.ListenCfg{}}

	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(lis, server)
	ev.EventWrite = func(ev *core.Event, _ []byte) error {
		ev.Conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err := ev.Conn.Write(ev.ResponseBytes())
		return err
	}
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "GET"
	ev.RR.Req.Path = "/nowhere"
	ev.RR.Req.Headers["Host"] = "missing.example"
	ev.RR.Req.Protocol = "HTTP/1.1"
	ev.RR.Res = response.ResponseInit()

	collect := startReader(t, client)
	EventHandler(ev, filters.NewFilter())
	got := collect()

	if !strings.Contains(got, "404") {
		t.Fatalf("expected 404, got %q", got)
	}
}

// GFilterHostPath：Host 精确匹配优先，未命中回落 DEFAULT_PORT 通配
func TestGFilterHostPathFallbacksToDefaultPort(t *testing.T) {
	re := regexp.MustCompile("^/api")
	lis := &listener.Listener{Port: "18080", HostMap: map[string][]listener.ListenCfg{
		"vip.example": {{ID: 1, Path: "/api", PathRe: re, Type: config.DEVMOD}},
		config.DEFAULT_PORT: {{ID: 2, Path: "/api", PathRe: re, Type: config.DEVMOD}},
	}}

	// 未知 Host → DEFAULT_PORT 通配命中
	ev := core.NewEvent(lis, nil)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Path = "/api/x"
	ev.RR.Req.Headers["Host"] = "stranger.example"
	fif := filters.NewFilter()

	cfg, ok := fif.Fif.RequestFilter(ev)
	if !ok {
		t.Fatal("default-port fallback must match")
	}
	if cfg.ID != 2 {
		t.Fatalf("matched cfg.ID = %d, want 2 (default port entry)", cfg.ID)
	}

	// 已知 Host → 精确条目
	ev2 := core.NewEvent(lis, nil)
	ev2.RR.Req = request.RequestInit(false)
	ev2.RR.Req.Path = "/api/x"
	ev2.RR.Req.Headers["Host"] = "vip.example"
	cfg2, ok := fif.Fif.RequestFilter(ev2)
	if !ok || cfg2.ID != 1 {
		t.Fatalf("host exact match: cfg=%+v ok=%v", cfg2, ok)
	}

	// PathRe 不匹配 → 未命中
	ev3 := core.NewEvent(lis, nil)
	ev3.RR.Req = request.RequestInit(false)
	ev3.RR.Req.Path = "/other"
	ev3.RR.Req.Headers["Host"] = "vip.example"
	if _, ok := fif.Fif.RequestFilter(ev3); ok {
		t.Fatal("unmatched path must not match any location")
	}
}

// startReader 复用 fragment_test 的并发读取模式
func startReader(t *testing.T, client net.Conn) func() string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		buf, _ := io.ReadAll(client)
		done <- string(buf)
	}()
	return func() string {
		t.Helper()
		select {
		case got := <-done:
			return got
		case <-time.After(3 * time.Second):
			t.Fatal("response never arrived")
			return ""
		}
	}
}
