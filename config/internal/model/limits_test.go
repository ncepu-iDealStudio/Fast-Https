package model

import (
	"testing"
	"time"
)

// 超时配置的语义：0 用默认值，正数按秒换算成 Duration。
func TestServerLimitTimeoutDuration(t *testing.T) {
	var zero ServerLimit
	if got := zero.ReadTimeoutDuration(); got != 30*time.Second {
		t.Fatalf("read timeout = %v, want 30s", got)
	}
	if got := zero.WriteTimeoutDuration(); got != 30*time.Second {
		t.Fatalf("write timeout = %v, want 30s", got)
	}
	if got := zero.BodyTimeoutDuration(); got != time.Second {
		t.Fatalf("body timeout = %v, want 1s", got)
	}

	cfg := ServerLimit{ReadTimeout: 5, WriteTimeout: 7, BodyTimeout: 2}
	if got := cfg.ReadTimeoutDuration(); got != 5*time.Second {
		t.Fatalf("read timeout = %v, want 5s", got)
	}
	if got := cfg.WriteTimeoutDuration(); got != 7*time.Second {
		t.Fatalf("write timeout = %v, want 7s", got)
	}
	if got := cfg.BodyTimeoutDuration(); got != 2*time.Second {
		t.Fatalf("body timeout = %v, want 2s", got)
	}
}

func TestProxyUpstreamTimeoutDuration(t *testing.T) {
	var zero Proxy
	if got := zero.UpstreamTimeoutDuration(); got != 20*time.Second {
		t.Fatalf("upstream timeout = %v, want 20s", got)
	}
	if got := (Proxy{UpstreamTimeout: 5}).UpstreamTimeoutDuration(); got != 5*time.Second {
		t.Fatalf("upstream timeout = %v, want 5s", got)
	}
}

// 隧道空闲断开：0 表示不启用，正数按秒换算。
func TestProxyTunnelIdleTimeoutDuration(t *testing.T) {
	if got := (Proxy{}).TunnelIdleTimeoutDuration(); got != 0 {
		t.Fatalf("tunnel idle timeout = %v, want 0 (disabled)", got)
	}
	if got := (Proxy{TunnelIdleTimeout: 60}).TunnelIdleTimeoutDuration(); got != time.Minute {
		t.Fatalf("tunnel idle timeout = %v, want 1m", got)
	}
}
