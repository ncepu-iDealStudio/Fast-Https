package safe

import (
	"testing"
	"time"

	"fast-https/config"
	"fast-https/runtime/listener"

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

func keepCounts(t *testing.T) {
	t.Helper()
	old := listener.GLisinfos
	oldTable := gcl.Load()
	t.Cleanup(func() {
		listener.GLisinfos = old
		gcl.Store(oldTable)
	})
}

func TestCountsInitReplacesSlice(t *testing.T) {
	keepCounts(t)

	listener.GLisinfos = []listener.Listener{{
		Cfg: []listener.ListenCfg{{}},
	}}
	countsInit()
	if n := len(*gcl.Load()); n != 1 {
		t.Fatalf("first init length %d, want 1", n)
	}
	countsInit()
	if n := len(*gcl.Load()); n != 1 {
		t.Fatalf("second init length %d, want 1", n)
	}
}

func TestCountLimitForUsesLocationID(t *testing.T) {
	keepCounts(t)

	// IDs follow server order, so one port may hold IDs 0 and 2.
	listener.GLisinfos = []listener.Listener{
		{Cfg: []listener.ListenCfg{
			{ID: 0, Limit: config.PathLimit{Rate: 10}},
			{ID: 2, Limit: config.PathLimit{Rate: 30}},
		}},
		{Cfg: []listener.ListenCfg{
			{ID: 1, Limit: config.PathLimit{Rate: 20}},
		}},
	}
	countsInit()
	for id, want := range []int{10, 20, 30} {
		cl := CountLimitFor(id)
		if cl == nil {
			t.Fatalf("CountLimitFor(%d) = nil", id)
		}
		if cl.rate != want {
			t.Fatalf("CountLimitFor(%d).rate = %d, want %d", id, cl.rate, want)
		}
	}
}

func TestCountLimitForOutOfRange(t *testing.T) {
	keepCounts(t)

	listener.GLisinfos = []listener.Listener{{
		Cfg: []listener.ListenCfg{{ID: 0}},
	}}
	countsInit()
	for _, id := range []int{-1, 1, 99} {
		if cl := CountLimitFor(id); cl != nil {
			t.Fatalf("CountLimitFor(%d) = %v, want nil", id, cl)
		}
	}
}

func TestCountLimitForDuringRebuild(t *testing.T) {
	keepCounts(t)

	large := []listener.Listener{{Cfg: []listener.ListenCfg{{ID: 0}, {ID: 1}, {ID: 2}}}}
	small := []listener.Listener{{Cfg: []listener.ListenCfg{{ID: 0}}}}
	listener.GLisinfos = large
	countsInit()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			for id := 0; id < 3; id++ {
				if cl := CountLimitFor(id); cl != nil {
					cl.Insert("127.0.0.1")
				}
			}
		}
	}()
	for i := 0; i < 200; i++ {
		if i%2 == 0 {
			listener.GLisinfos = small
		} else {
			listener.GLisinfos = large
		}
		countsInit()
	}
	close(stop)
	<-done
}
