package events

import (
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

// TestEventHandlerEarlyRequestShortCircuitsRouting 确认前置钩子在 location
// 匹配之前生效：命中后请求不再进入路由，也不会落到注册的处理器上。
// ACME http-01 的应答依赖这个顺序（站点根目录里通常没有挑战文件）。
func TestEventHandlerEarlyRequestShortCircuitsRouting(t *testing.T) {
	resetMetrics(t)
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
		nil)
	t.Cleanup(func() { core.RegisterHandler(config.DEVMOD, nil, nil, nil) })

	core.SetServices(core.Services{
		HandleEarlyRequest: func(ev *core.Event) bool {
			if ev.RR.Req.Path != "/.well-known/acme-challenge/token" {
				return false
			}
			ev.RR.Res = response.ResponseInit()
			ev.RR.Res.SetFirstLine(200, "OK")
			ev.RR.Res.SetHeader("Content-Type", "text/plain")
			ev.RR.Res.SetHeader("Content-Length", "9")
			ev.RR.Res.Body = []byte("keyauth-1")
			ev.WriteResponseClose(nil)
			return true
		},
	})
	t.Cleanup(func() { core.SetServices(core.Services{}) })

	re := regexp.MustCompile("^/")
	lis := &listener.Listener{Port: "18081", HostMap: map[string][]listener.ListenCfg{
		"example.com": {{ID: 1, Path: "/", PathRe: re, Type: config.DEVMOD}},
	}}

	emit := func(path string) string {
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
		ev.RR.Req.Path = path
		ev.RR.Req.Headers["Host"] = "example.com"
		ev.RR.Req.Protocol = "HTTP/1.1"
		ev.RR.Res = response.ResponseInit()

		collect := startReader(t, client)
		EventHandler(ev, filters.NewFilter())
		return collect()
	}

	challenge := emit("/.well-known/acme-challenge/token")
	if !strings.Contains(challenge, "keyauth-1") {
		t.Fatalf("challenge response: %q", challenge)
	}
	if dispatched.Load() != 0 {
		t.Fatalf("challenge request reached the router: dispatched %d times", dispatched.Load())
	}

	// 其它路径照常进入路由。
	if got := emit("/index.html"); !strings.Contains(got, "dispatched") {
		t.Fatalf("normal response: %q", got)
	}
	if dispatched.Load() != 1 {
		t.Fatalf("normal request dispatched %d times, want 1", dispatched.Load())
	}
}
