package core_test

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
	"testing"
)

func TestMissingHandlersWithoutRegistration(t *testing.T) {
	servers := []config.Server{{
		Path: []config.Path{{PathType: config.LOCAL}},
	}}
	if err := core.MissingHandlers(servers); err == nil {
		t.Fatal("local handler should be reported missing before registration")
	}
}

func TestMissingHandlersAfterExplicitRegister(t *testing.T) {
	core.RRHandlerRegister(config.LOCAL, func(*listener.ListenCfg, *core.Event) bool { return true }, func(*listener.ListenCfg, *core.Event) {}, nil)
	t.Cleanup(func() {
		core.RRHandlerRegister(config.LOCAL, nil, nil, nil)
	})

	servers := []config.Server{{
		Path: []config.Path{{PathType: config.LOCAL}},
	}}
	if err := core.MissingHandlers(servers); err != nil {
		t.Fatalf("registered local handler should pass: %v", err)
	}
}

func TestProxyTCPDoesNotRequireRequestHandler(t *testing.T) {
	servers := []config.Server{{
		Path: []config.Path{{PathType: config.PROXY_TCP}},
	}}
	if err := core.MissingHandlers(servers); err != nil {
		t.Fatalf("tcp proxy is handled by the listen filter: %v", err)
	}
}
