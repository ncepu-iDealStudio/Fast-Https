package safe

import (
	"testing"
	"time"
)

// per-IP 令牌桶：一个客户端打满自己的桶不影响别人
func TestIPLimitersAreIndependent(t *testing.T) {
	il := &ipLimiters{
		limiters: make(map[string]*ipLimiter),
		rate:     1,
		burst:    1,
	}
	if !il.allow("1.1.1.1") {
		t.Fatal("first request should pass")
	}
	if il.allow("1.1.1.1") {
		t.Error("burst exhausted for 1.1.1.1, should reject")
	}
	if !il.allow("2.2.2.2") {
		t.Error("another IP should not be affected by 1.1.1.1's exhausted bucket")
	}
}

// 长时间不活跃的 IP 桶会被清扫回收
func TestIPLimitersSweepIdle(t *testing.T) {
	il := &ipLimiters{
		limiters: make(map[string]*ipLimiter),
		rate:     1,
		burst:    1,
	}
	il.allow("1.1.1.1")
	il.limiters["1.1.1.1"].lastSeen = il.limiters["1.1.1.1"].lastSeen.Add(-limiterIdleTTL - time.Minute)

	il.sweepLocked(time.Now())
	if _, ok := il.limiters["1.1.1.1"]; ok {
		t.Error("idle limiter should be swept")
	}
}
