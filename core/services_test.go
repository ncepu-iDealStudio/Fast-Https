package core

import (
	"fast-https/runtime/listener"
	"testing"
)

func TestServicesDefaultAllows(t *testing.T) {
	previous := services
	t.Cleanup(func() { services = previous })
	services = Services{}

	ev := &Event{}
	cfg := &listener.ListenCfg{}
	if ConnRejected(ev) || ListenHandled(ev) || LocationRejected(cfg, ev) || AuthRejected(cfg, ev) {
		t.Fatal("unset hooks should allow the event")
	}
	RunAfterListen()
}

func TestServicesUsesConfiguredHooks(t *testing.T) {
	previous := services
	t.Cleanup(func() { services = previous })

	var after int
	SetServices(Services{
		RejectConn:     func(*Event) bool { return true },
		HandleListen:   func(*Event) bool { return true },
		RejectLocation: func(*listener.ListenCfg, *Event) bool { return true },
		RejectAuth:     func(*listener.ListenCfg, *Event) bool { return true },
		AfterListen:    func() { after++ },
	})
	ev := &Event{}
	cfg := &listener.ListenCfg{}
	if !ConnRejected(ev) || !ListenHandled(ev) || !LocationRejected(cfg, ev) || !AuthRejected(cfg, ev) {
		t.Fatal("configured hooks should refuse the event")
	}
	RunAfterListen()
	if after != 1 {
		t.Fatalf("AfterListen calls = %d", after)
	}
}
