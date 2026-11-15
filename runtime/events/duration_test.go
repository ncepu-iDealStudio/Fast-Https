package events

import (
	"testing"
	"time"

	"fast-https/core"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
	"fast-https/runtime/metrics"
)

func durationLe(t *testing.T, h metrics.DurationHistogram, want any) uint64 {
	t.Helper()
	for _, b := range h.Buckets {
		switch w := want.(type) {
		case int:
			if v, ok := b.Le.(float64); ok && int(v) == w {
				return b.Count
			}
		case string:
			if v, ok := b.Le.(string); ok && v == w {
				return b.Count
			}
		}
	}
	t.Fatalf("bucket %v not found in %+v", want, h.Buckets)
	return 0
}

func TestDurationFastRequest(t *testing.T) {
	resetMetrics(t)
	registerStub(t, func(cfg *listener.ListenCfg, ev *core.Event) {
		ev.RR.Res.SetFirstLine(200, "OK")
		_ = ev.WriteResponse(nil)
	})
	EventHandler(newHandlerEvent(t, stubListener(), "GET", "/ok", "example.com"), filters.NewFilter())

	h := metrics.DurationSnapshot()
	if h.Count != 1 {
		t.Fatalf("count = %d, want 1", h.Count)
	}
	if durationLe(t, h, 1) != 1 {
		t.Fatalf("fast request did not land in the 1ms bucket: %+v", h.Buckets)
	}
	if durationLe(t, h, "+Inf") != 1 {
		t.Fatalf("+Inf = %d, want 1", durationLe(t, h, "+Inf"))
	}
}

func TestDurationSlowRequest(t *testing.T) {
	resetMetrics(t)
	registerStub(t, func(cfg *listener.ListenCfg, ev *core.Event) {
		time.Sleep(30 * time.Millisecond)
		ev.RR.Res.SetFirstLine(200, "OK")
		_ = ev.WriteResponse(nil)
	})
	EventHandler(newHandlerEvent(t, stubListener(), "GET", "/ok", "example.com"), filters.NewFilter())

	h := metrics.DurationSnapshot()
	if h.Count != 1 {
		t.Fatalf("count = %d, want 1", h.Count)
	}
	// 30ms 超过 25ms 上界：累积桶 le=1/5/10/25 不应增加。
	for _, le := range []int{1, 5, 10, 25} {
		if got := durationLe(t, h, le); got != 0 {
			t.Fatalf("le=%d count=%d, want 0 for a 30ms request", le, got)
		}
	}
	if durationLe(t, h, 50) != 1 {
		t.Fatalf("le=50 count=%d, want 1", durationLe(t, h, 50))
	}
	if durationLe(t, h, "+Inf") != 1 {
		t.Fatalf("+Inf = %d, want 1", durationLe(t, h, "+Inf"))
	}
}
