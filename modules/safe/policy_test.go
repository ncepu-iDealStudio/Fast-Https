package safe

import (
	"testing"
	"time"

	"fast-https/modules/core/listener"

	"golang.org/x/time/rate"
)

func TestBlacklistHitAndMiss(t *testing.T) {
	list := NewBlacklist()
	if err := list.Add("127.0.0.2"); err != nil {
		t.Fatalf("add ip: %v", err)
	}
	if err := list.Add("192.168.1.10-192.168.1.12"); err != nil {
		t.Fatalf("add range: %v", err)
	}

	if !list.isInBlacklist("127.0.0.2") {
		t.Fatal("listed ip should be rejected")
	}
	if list.isInBlacklist("127.0.0.1") {
		t.Fatal("unlisted ip should be allowed")
	}
	if !list.isInBlacklist("192.168.1.11") {
		t.Fatal("ip inside the range should be rejected")
	}
	if list.isInBlacklist("192.168.1.13") {
		t.Fatal("ip outside the range should be allowed")
	}

	if err := list.Remove("127.0.0.2"); err != nil {
		t.Fatalf("remove ip: %v", err)
	}
	if list.isInBlacklist("127.0.0.2") {
		t.Fatal("removed ip should be allowed")
	}
}

func TestRateLimitRejectsAfterBurst(t *testing.T) {
	lim := rate.NewLimiter(rate.Every(time.Hour), 1)
	if !allow(lim) {
		t.Fatal("first event inside the burst should pass")
	}
	if allow(lim) {
		t.Fatal("event past the burst should be rejected")
	}
	if !allow(nil) {
		t.Fatal("a nil limiter should allow the event")
	}
}

func TestCountsInitReplacesSlice(t *testing.T) {
	old := listener.GLisinfos
	oldGcl := Gcl
	t.Cleanup(func() {
		listener.GLisinfos = old
		Gcl = oldGcl
	})

	listener.GLisinfos = []listener.Listener{{
		Cfg: []listener.ListenCfg{{}},
	}}
	countsInit()
	if len(Gcl) != 1 {
		t.Fatalf("first init length %d, want 1", len(Gcl))
	}
	countsInit()
	if len(Gcl) != 1 {
		t.Fatalf("second init length %d, want 1", len(Gcl))
	}
}
