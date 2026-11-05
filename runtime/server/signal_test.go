package server

import (
	"syscall"
	"testing"

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

// SIGINT 在所有运行方式下都只重载：前台（容器、systemd）也一样，
// 停止走 SIGTERM / SIGQUIT 或 stop 命令。
func TestSigHandlerSigintReloads(t *testing.T) {
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
		t.Error("SIGINT should reload")
	}
}
