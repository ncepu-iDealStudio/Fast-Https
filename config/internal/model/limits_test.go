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
