package server

import (
	"errors"
	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"testing"
)

// TestReloadAlignsHooksBeforeListeners 确认 BeforeListenReload 跑在
// ReloadListenCfg 之前：钩子执行时监听器表还是旧的。
//
// 顺序有实际后果——listenSsl 建 tls.Config 时读 ACME 的 acme-tls/1
// provider，钩子晚一步，新建的监听器就不会通告 acme-tls/1。
func TestReloadAlignsHooksBeforeListeners(t *testing.T) {
	// 配置里一个 server 都没有：ReloadListenCfg 会把旧端口全部移除，
	// 不触发任何真实绑定。
	previous := *config.Current()
	next := previous
	next.Servers = nil
	config.Store(&next, config.MIME())
	t.Cleanup(func() { config.Store(&previous, config.MIME()) })

	oldListener := &listener.Listener{Port: "18093", LisType: 0}
	previousListeners := listener.GLisinfos
	listener.GLisinfos = []*listener.Listener{oldListener}
	t.Cleanup(func() { listener.GLisinfos = previousListeners })

	var calls int
	core.SetServices(core.Services{
		BeforeListenReload: func() {
			calls++
			// 此刻旧监听器表还没被替换，说明钩子确实在 ReloadListenCfg 之前。
			if got := len(listener.SnapshotListeners()); got != 1 {
				t.Errorf("BeforeListenReload saw %d listeners, want the old table (1)", got)
			}
		},
	})
	t.Cleanup(func() { core.SetServices(core.Services{}) })

	previousReload := reloadConfig
	reloadConfig = func() error { return nil }
	t.Cleanup(func() { reloadConfig = previousReload })

	s := &Server{Listens: []*listener.Listener{oldListener}}
	s.Reload()

	if calls != 1 {
		t.Fatalf("BeforeListenReload calls = %d, want 1", calls)
	}
	if got := len(listener.SnapshotListeners()); got != 0 {
		t.Fatalf("listeners after reload = %d, want 0", got)
	}
}

// TestReloadSkipsHooksWhenConfigRejected 确认配置校验失败时不动钩子：
// 旧配置继续生效，钩子自然也不该跟着换。
func TestReloadSkipsHooksWhenConfigRejected(t *testing.T) {
	previousReload := reloadConfig
	reloadConfig = func() error { return errors.New("reload rejected by test") }
	t.Cleanup(func() { reloadConfig = previousReload })

	var calls int
	core.SetServices(core.Services{
		BeforeListenReload: func() { calls++ },
	})
	t.Cleanup(func() { core.SetServices(core.Services{}) })

	s := &Server{}
	s.Reload()

	if calls != 0 {
		t.Fatalf("BeforeListenReload calls = %d, want 0 when the config is rejected", calls)
	}
}
