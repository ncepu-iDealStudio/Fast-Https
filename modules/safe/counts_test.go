package safe

import (
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
)

func TestCountIPAllowEnforcesRateExactly(t *testing.T) {
	l := countIP{rate: 3, expire: int(time.Now().Unix()) + 60}
	for i := 0; i < 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("request %d must pass with rate=3", i+1)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("4th request must be rejected with rate=3")
	}
}

func TestCountIPResetsAfterWindow(t *testing.T) {
	l := countIP{rate: 1, expire: int(time.Now().Unix()) + 60}
	if !l.Allow("ip") || l.Allow("ip") {
		t.Fatal("rate=1 allows exactly one request")
	}
	// 窗口过期后再放行：Allow 内部按 expire 重置
	l.expire = int(time.Now().Unix()) - 1
	if !l.Allow("ip") {
		t.Fatal("counter must reset after the window expires")
	}
	if l.count != 1 {
		t.Fatalf("count after reset = %d, want 1", l.count)
	}
}

func TestCountIPSetRestartsWindow(t *testing.T) {
	l := countIP{rate: 1, count: 99, expire: 0}
	l.Set(5, time.Now())
	if l.rate != 5 || l.count != 0 {
		t.Fatalf("Set() = %+v", l)
	}
}

func TestCountLimitInsertSweepsAndCaps(t *testing.T) {
	t.Run("new ip counts first request", func(t *testing.T) {
		cl := NewCountLimit(0, 2, 1024)
		if !cl.Insert("a") || !cl.Insert("a") {
			t.Fatal("first two requests must pass")
		}
		if cl.Insert("a") {
			t.Fatal("third request must be rejected")
		}
	})

	t.Run("num over size always admits", func(t *testing.T) {
		cl := NewCountLimit(10, 0, 1) // num(10) > size(1)：超额直通
		for i := 0; i < 5; i++ {
			if !cl.Insert("same-ip") {
				t.Fatal("over-capacity limiter must not throttle")
			}
		}
	})

	t.Run("expired entries are swept", func(t *testing.T) {
		cl := NewCountLimit(0, 1, 1024)
		cl.Insert("gone")
		cl.mu.Lock()
		entry := cl.gMap["gone"]
		entry.expire = int(time.Now().Unix()) - 1
		cl.gMap["gone"] = entry
		// 清扫每秒至多一次：把上次清扫时间拨回上一秒，模拟跨秒到达
		cl.lastSweep = int(time.Now().Unix()) - 1
		cl.mu.Unlock()
		if !cl.Insert("fresh") {
			t.Fatal("sweep must not block new IPs")
		}
		cl.mu.RLock()
		_, still := cl.gMap["gone"]
		n := len(cl.gMap)
		cl.mu.RUnlock()
		if still || n != 1 {
			t.Fatalf("expired entry kept: still=%v map=%d (want only the fresh IP)", still, n)
		}
	})
}

func TestCountLimitForBounds(t *testing.T) {
	if CountLimitFor(0) != nil {
		t.Fatal("empty table must return nil")
	}
	table := []*CountLimit{nil, NewCountLimit(0, 1, 1)}
	gcl.Store(&table)
	defer gcl.Store(nil)
	if CountLimitFor(0) != nil {
		t.Fatal("slot 0 holds nil")
	}
	if CountLimitFor(1) == nil {
		t.Fatal("slot 1 must resolve")
	}
	for _, id := range []int{-1, 2, 99} {
		if CountLimitFor(id) != nil {
			t.Fatalf("id %d out of range must return nil", id)
		}
	}
}

func TestCountsInitBuildsTableFromListeners(t *testing.T) {
	orig := listener.GLisinfos
	listener.GLisinfos = []*listener.Listener{
		{
			Cfg: []listener.ListenCfg{
			{ID: 0, Limit: config.PathLimit{Rate: 5, Size: 1}},
			{ID: 2, Limit: config.PathLimit{Rate: 7, Size: 1}},
		}},
	}
	defer func() { listener.GLisinfos = orig; gcl.Store(nil) }()

	countsInit()
	for id, wantRate := range map[int]int{0: 5, 2: 7} {
		cl := CountLimitFor(id)
		if cl == nil {
			t.Fatalf("id %d missing from table", id)
		}
		if cl.rate != wantRate {
			t.Errorf("id %d rate = %d, want %d", id, cl.rate, wantRate)
		}
	}
	if CountLimitFor(1) != nil {
		t.Error("unfilled slot must stay nil")
	}
}

func TestCountHandlerWrites429(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.EventWrite = func(ev *core.Event, _ []byte) error {
		ev.Conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err := ev.Conn.Write(ev.ResponseBytes())
		return err
	}
	ev.RR.Req = request.RequestInit(false)

	done := make(chan string, 1)
	go func() {
		buf, _ := io.ReadAll(client)
		done <- string(buf)
	}()

	rr := core.RRcircle{Ev: ev}
	CountHandler(rr)

	select {
	case got := <-done:
		if !strings.Contains(got, "403") && !strings.Contains(got, "429") && !strings.Contains(got, "Too Many") {
			t.Fatalf("limit response: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("limit response never arrived")
	}
}
