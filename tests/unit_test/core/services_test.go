package core_test

import (
	"net"
	"strings"
	"testing"

	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
)

func TestRegisteredTypesSorted(t *testing.T) {
	core.RegisterHandler(700, func(*listener.ListenCfg, *core.Event) bool { return true },
		func(*listener.ListenCfg, *core.Event) {}, nil)
	core.RegisterHandler(300, func(*listener.ListenCfg, *core.Event) bool { return true },
		func(*listener.ListenCfg, *core.Event) {}, nil)
	t.Cleanup(func() {
		core.RegisterHandler(700, nil, nil, nil)
		core.RegisterHandler(300, nil, nil, nil)
	})

	types := core.RegisteredTypes()
	if len(types) < 2 || types[0] > types[1] {
		t.Fatalf("RegisteredTypes() not sorted: %v", types)
	}
	found := false
	for _, v := range types {
		if v == 700 {
			found = true
		}
	}
	if !found {
		t.Fatalf("registered type 700 missing from %v", types)
	}
}

// 撤销注册后 Handler 必须查不到
func TestRegisterHandlerRevoke(t *testing.T) {
	core.RegisterHandler(701,
		func(*listener.ListenCfg, *core.Event) bool { return true },
		func(*listener.ListenCfg, *core.Event) {}, nil)
	if _, ok := core.Handler(701); !ok {
		t.Fatal("handler must be registered")
	}
	core.RegisterHandler(701, nil, nil, nil)
	if _, ok := core.Handler(701); ok {
		t.Fatal("revoked handler must disappear")
	}
}

func TestServiceHooksNilSafeAndInvoked(t *testing.T) {
	// 未注入时全部安全放行
	if core.WAFRejected(&listener.ListenCfg{}, nil) {
		t.Fatal("nil WAF hook must not reject")
	}
	core.RunAfterListen()
	core.RunBeforeStop()
	core.RunAfterStop()

	// 注入后被调用（SetServices 整体替换，结束后还原为空）
	called := map[string]bool{}
	origServices := core.Services{}
	core.SetServices(core.Services{
		RejectWAF: func(cfg *listener.ListenCfg, ev *core.Event) bool {
			called["waf"] = true
			return false
		},
		AfterStop: func() { called["afterStop"] = true },
	})
	defer core.SetServices(origServices)

	if core.WAFRejected(&listener.ListenCfg{}, nil) {
		t.Fatal("hook returning false must not reject")
	}
	core.RunAfterStop()
	if !called["waf"] || !called["afterStop"] {
		t.Fatalf("hooks not invoked: %v", called)
	}
}

func TestEventReuseFlag(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	ev := core.NewEvent(&listener.Listener{}, server)
	if ev.EventReuse() {
		t.Fatal("fresh event must not be reusable")
	}
	ev.Reuse = true
	if !ev.EventReuse() {
		t.Fatal("Reuse flag must be reflected")
	}
}

// DefaultParseCommandHandler：RemoteAddr 提取 IP、XFF 追加、变量表填充
func TestDefaultParseCommandHandler(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "GET"
	ev.RR.Req.Path = "/page?x=1"
	ev.RR.Req.Headers["Host"] = "h.example"

	core.DefaultParseCommandHandler(&listener.ListenCfg{ProxyAddr: "up:9000"}, ev)

	m := ev.RR.CircleCommandVal.Map
	if m["request_method"] != "GET" || m["request_uri"] != "/page?x=1" {
		t.Fatalf("request vars: %v", m)
	}
	if m["proxy_host"] != "up:9000" {
		t.Fatalf("proxy_host = %q", m["proxy_host"])
	}
	// 无 XFF 时以客户端 IP 起始
	if !strings.HasSuffix(m["proxy_add_x_forwarded_for"], m["remote_addr"]) {
		t.Fatalf("xff = %q remote = %q", m["proxy_add_x_forwarded_for"], m["remote_addr"])
	}

	// 已有 XFF：追加而非覆盖
	ev2 := core.NewEvent(&listener.Listener{}, server)
	ev2.RR.Req = request.RequestInit(false)
	ev2.RR.Req.Headers["X-Forwarded-For"] = "9.9.9.9"
	core.DefaultParseCommandHandler(&listener.ListenCfg{}, ev2)
	m2 := ev2.RR.CircleCommandVal.Map
	if !strings.HasPrefix(m2["proxy_add_x_forwarded_for"], "9.9.9.9, ") {
		t.Fatalf("existing XFF must be appended: %q", m2["proxy_add_x_forwarded_for"])
	}
}
