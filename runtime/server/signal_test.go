package server

import (
	"syscall"
	"testing"

	"fast-https/core"
	"fast-https/runtime/listener"
)

func TestSignalEffectIsTheSameOnEveryPlatform(t *testing.T) {
	if signalEffectOf(syscall.SIGINT) != signalReload {
		t.Fatal("SIGINT should reload")
	}
	if signalEffectOf(syscall.SIGTERM) != signalStop {
		t.Fatal("SIGTERM should stop")
	}
	if signalEffectOf(syscall.SIGQUIT) != signalStop {
		t.Fatal("SIGQUIT should stop")
	}
}

// dev 前台模式：SIGINT 走优雅停机，而不是重载
func TestSigHandlerDevForegroundStops(t *testing.T) {
	SetForegroundDev(true)
	t.Cleanup(func() { SetForegroundDev(false) })

	prev := reloadConfig
	reloadConfig = func() error {
		t.Error("SIGINT in dev foreground mode must not reload")
		return nil
	}
	t.Cleanup(func() { reloadConfig = prev })

	stopped := false
	core.SetServices(core.Services{
		BeforeStop: func() { stopped = true },
	})
	t.Cleanup(func() { core.SetServices(core.Services{}) })

	s := &Server{Listens: []*listener.Listener{}}
	s.Wg.Add(1)
	s.SigHandler(syscall.SIGINT)

	if !stopped {
		t.Error("SIGINT in dev mode should run the graceful stop hooks")
	}
}

// 守护进程模式（默认）：SIGINT 仍然重载
func TestSigHandlerDaemonSigintReloads(t *testing.T) {
	SetForegroundDev(false)

	reloaded := false
	prev := reloadConfig
	reloadConfig = func() error {
		reloaded = true
		return nil
	}
	t.Cleanup(func() { reloadConfig = prev })

	s := &Server{Listens: []*listener.Listener{}}
	s.SigHandler(syscall.SIGINT)

	if !reloaded {
		t.Error("SIGINT in daemon mode should reload")
	}
}
