package events

import (
	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"net"
	"regexp"
	"strings"
	"testing"
	"time"
)

// allowMethodsListener 造一个只允许 GET 的站点。
func allowMethodsListener() *listener.Listener {
	re := regexp.MustCompile("^/")
	return &listener.Listener{Port: "18082", HostMap: map[string][]listener.ListenCfg{
		"example.com": {{
			ID:           1,
			Path:         "/",
			PathRe:       re,
			Type:         config.DEVMOD,
			AllowMethods: []string{"GET"},
		}},
	}}
}

func emitRequest(t *testing.T, lis *listener.Listener, method, path, host string) string {
	t.Helper()
	resetMetrics(t)
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	ev := core.NewEvent(lis, server)
	ev.EventWrite = func(ev *core.Event, _ []byte) error {
		ev.Conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err := ev.Conn.Write(ev.ResponseBytes())
		return err
	}
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = method
	ev.RR.Req.Path = path
	ev.RR.Req.Headers["Host"] = host
	ev.RR.Req.Protocol = "HTTP/1.1"
	ev.RR.Res = response.ResponseInit()

	collected := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := client.Read(buf)
		collected <- string(buf[:n])
	}()

	EventHandler(ev, filters.NewFilter())
	select {
	case got := <-collected:
		return got
	case <-time.After(3 * time.Second):
		t.Fatal("no response written")
		return ""
	}
}

func TestEventHandlerRejectsDisallowedMethod(t *testing.T) {
	core.RegisterHandler(config.DEVMOD,
		func(cfg *listener.ListenCfg, ev *core.Event) bool { return true },
		func(cfg *listener.ListenCfg, ev *core.Event) {
			ev.RR.Res.SetFirstLine(200, "OK")
			ev.RR.Res.SetBody([]byte("ok"))
			ev.WriteResponseClose(nil)
		},
		nil)
	t.Cleanup(func() { core.RegisterHandler(config.DEVMOD, nil, nil, nil) })

	lis := allowMethodsListener()
	if got := emitRequest(t, lis, "GET", "/", "example.com"); !strings.Contains(got, "200 OK") {
		t.Fatalf("GET should pass: %q", got)
	}

	got := emitRequest(t, lis, "POST", "/", "example.com")
	if !strings.Contains(got, "405") {
		t.Fatalf("POST should be 405: %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "allow: get") {
		t.Fatalf("405 must carry Allow header: %q", got)
	}
}

func TestEventHandlerHostCheckEnforce(t *testing.T) {
	previous := *config.Current()
	previousMIME := config.MIME()
	t.Cleanup(func() { config.Store(&previous, previousMIME) })
	config.Store(&config.Fast_Https{
		Security: &config.SecurityConfig{HostCheck: config.HostCheckEnforce},
	}, previousMIME)

	core.RegisterHandler(config.DEVMOD,
		func(cfg *listener.ListenCfg, ev *core.Event) bool { return true },
		func(cfg *listener.ListenCfg, ev *core.Event) {
			ev.RR.Res.SetFirstLine(200, "OK")
			ev.WriteResponseClose(nil)
		},
		nil)
	t.Cleanup(func() { core.RegisterHandler(config.DEVMOD, nil, nil, nil) })

	lis := allowMethodsListener()
	if got := emitRequest(t, lis, "GET", "/", "unknown.example"); !strings.Contains(got, "421") {
		t.Fatalf("unknown host should be 421 in enforce mode: %q", got)
	}

	// off 模式（默认）不做 Host 校验：不返回 421，交由原本的回落逻辑处理
	// （这个合成 listener 没有默认 server，所以最终是 404）。
	config.Store(&config.Fast_Https{}, previousMIME)
	if got := emitRequest(t, lis, "GET", "/", "unknown.example"); strings.Contains(got, "421") {
		t.Fatalf("off mode must not reject on host mismatch: %q", got)
	}
}
