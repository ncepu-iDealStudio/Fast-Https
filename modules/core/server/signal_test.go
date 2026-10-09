package server

import (
	"syscall"
	"testing"
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
