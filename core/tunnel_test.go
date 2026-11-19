package core

import (
	"testing"
	"time"
)

// timeout=0 表示不启用：不设 deadline，也永远不会判空闲。
func TestTunnelIdleDisabled(t *testing.T) {
	idle := &TunnelIdle{}
	if idle.Enabled() {
		t.Fatal("zero timeout should be disabled")
	}
	if !idle.Deadline().IsZero() {
		t.Fatal("disabled tunnel should clear the deadline")
	}
	if idle.Expired() {
		t.Fatal("disabled tunnel should never expire")
	}
}

// 启用后按最后活动时间判断；Touch 会重置计时。
func TestTunnelIdleExpiresAndResets(t *testing.T) {
	idle := &TunnelIdle{timeout: 50 * time.Millisecond}
	idle.Touch()
	if !idle.Enabled() || idle.Deadline().IsZero() {
		t.Fatal("enabled tunnel should set a deadline")
	}
	if idle.Expired() {
		t.Fatal("just touched tunnel must not be expired")
	}
	time.Sleep(60 * time.Millisecond)
	if !idle.Expired() {
		t.Fatal("tunnel should expire after the idle timeout")
	}
	idle.Touch()
	if idle.Expired() {
		t.Fatal("Touch should reset the idle timer")
	}
}
