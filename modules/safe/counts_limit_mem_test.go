package safe

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// 复现：配置里只写 rate 不写 mem（size=0）时，per-location 计数限流
// 被静默旁路——旧实现从第二个请求起 `num > size` 恒真，任何 IP 都
// 不再计数。修复后未配置 mem 应全量跟踪，限流照常生效。
func TestCountLimitWithoutMemStillLimits(t *testing.T) {
	cl := NewCountLimit(0, 2, 0) // rate=2，未配置 mem

	if !cl.Insert("10.0.0.1") {
		t.Fatalf("first request of ip1 should pass")
	}
	if !cl.Insert("10.0.0.1") {
		t.Fatalf("second request of ip1 should pass (rate=2)")
	}
	if cl.Insert("10.0.0.1") {
		t.Fatalf("third request of ip1 must be rejected (rate=2)")
	}

	// 其他 IP 同样受限制，而不是绕过
	if !cl.Insert("10.0.0.2") {
		t.Fatalf("first request of ip2 should pass")
	}
	if !cl.Insert("10.0.0.2") {
		t.Fatalf("second request of ip2 should pass (rate=2)")
	}
	if cl.Insert("10.0.0.2") {
		t.Fatalf("third request of ip2 must be rejected (rate=2)")
	}
}

// rate 未配置（0）的 location 必须完全放行：每个 location 都会建一个
// CountLimit 条目，不能因为 count >= rate(0) 恒真而回 429。
func TestCountLimitWithoutRateAllowsEverything(t *testing.T) {
	cl := NewCountLimit(0, 0, 0)
	for i := 0; i < 50; i++ {
		if !cl.Insert("10.0.0.1") {
			t.Fatalf("unconfigured location must not reject (request %d)", i+1)
		}
	}
}

// 显式配置了 mem 时保留原有降级语义：超过容量的新 IP 放行不计数。
func TestCountLimitWithMemCapsTrackedIPs(t *testing.T) {
	cl := NewCountLimit(0, 1, 1) // rate=1，mem 折算后 size=1

	if !cl.Insert("10.0.0.1") {
		t.Fatalf("ip1 should be tracked (under cap)")
	}
	if cl.Insert("10.0.0.1") {
		t.Fatalf("ip1 second request must be rejected (rate=1)")
	}
	// num=1 已达到 size=1：新 IP 走降级路径放行，但不进入计数
	if !cl.Insert("10.0.0.2") {
		t.Fatalf("ip2 should pass via the over-cap fallback")
	}
	if !cl.Insert("10.0.0.2") {
		t.Fatalf("ip2 must NOT be counted after the over-cap fallback")
	}
}

// servers_limit 的 rate 换算：rps>1000 时毫秒整数除法得 0ms →
// rate.Every(0)=Inf，"每秒 N 个请求"静默变成完全不限流。
func TestEveryFromRateHandlesHighRPS(t *testing.T) {
	if everyFromRate(2000) == rate.Inf {
		t.Fatalf("rate=2000 must not degrade to unlimited")
	}
	if got := everyFromRate(2000); got != rate.Every(time.Second/2000) {
		t.Fatalf("rate=2000 should be a 500us token interval, got %v", got)
	}
	if got := everyFromRate(1); got != rate.Every(time.Second) {
		t.Fatalf("rate=1 should be a 1s token interval, got %v", got)
	}
	if everyFromRate(0) != rate.Inf {
		t.Fatalf("rate<=0 keeps the documented unlimited behaviour")
	}
}
