package server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/metrics"
)

func stubServeConnHandler(t *testing.T, fn func(*listener.Listener, net.Conn, context.Context)) {
	t.Helper()
	prev := serveConnHandler
	serveConnHandler = fn
	t.Cleanup(func() { serveConnHandler = prev })
}

func TestInitialStateIsStarting(t *testing.T) {
	metrics.ResetForTest()
	if got := metrics.State(); got != metrics.StateStarting {
		t.Fatalf("state = %s, want starting", got)
	}

	// 替身：不走真实 ListenWithCfg，只调用 ServerInit 在监听建立后的那几步。
	metrics.RecordProcessStart()
	publishListeners(nil)
	metrics.MarkRunning()

	if got := metrics.State(); got != metrics.StateRunning {
		t.Fatalf("after listeners ready, state = %s, want running", got)
	}
}

func TestReloadFailureKeepsRunning(t *testing.T) {
	metrics.ResetForTest()
	metrics.RecordProcessStart()
	metrics.MarkRunning()
	metrics.ConnAccepted()
	metrics.ConnClosed()
	metrics.NoteReloadSuccess()
	started := metrics.StartedAt()
	gen := metrics.Generation()
	failed := metrics.ReloadFailedTotal()
	accepted := metrics.ConnAcceptedTotal()

	previousReload := reloadConfig
	reloadConfig = func() error { return errors.New("reload rejected by test") }
	t.Cleanup(func() { reloadConfig = previousReload })

	s := &Server{}
	s.Reload()

	if got := metrics.State(); got != metrics.StateRunning {
		t.Fatalf("state = %s, want running", got)
	}
	if !metrics.StartedAt().Equal(started) {
		t.Fatal("started_at changed on failed reload")
	}
	if metrics.ConnAcceptedTotal() != accepted {
		t.Fatal("connection counters changed on failed reload")
	}
	if metrics.ReloadFailedTotal() != failed+1 {
		t.Fatalf("reload_failed_total = %d, want %d", metrics.ReloadFailedTotal(), failed+1)
	}
	if metrics.Generation() != gen {
		t.Fatalf("generation = %d, want %d", metrics.Generation(), gen)
	}
	if metrics.LastReloadOK() {
		t.Fatal("last_reload_ok = true, want false")
	}
}

func TestReloadSuccessBumpsGeneration(t *testing.T) {
	metrics.ResetForTest()
	metrics.RecordProcessStart()
	metrics.MarkRunning()
	metrics.ReplaceListeners([]metrics.ListenerInfo{{Port: "seed"}})

	previous := *config.Current()
	next := previous
	next.Servers = nil
	config.Store(&next, config.MIME())
	t.Cleanup(func() { config.Store(&previous, config.MIME()) })

	oldListener := &listener.Listener{Port: "18093", LisType: 0}
	previousListeners := listener.GLisinfos
	listener.GLisinfos = []*listener.Listener{oldListener}
	t.Cleanup(func() { listener.GLisinfos = previousListeners })

	core.SetServices(core.Services{})
	t.Cleanup(func() { core.SetServices(core.Services{}) })

	previousReload := reloadConfig
	reloadConfig = func() error { return nil }
	t.Cleanup(func() { reloadConfig = previousReload })

	before := time.Now().Add(-time.Second)
	s := &Server{Listens: []*listener.Listener{oldListener}}
	s.Reload()

	if metrics.Generation() != 1 {
		t.Fatalf("generation = %d, want 1", metrics.Generation())
	}
	if !metrics.LastReloadOK() {
		t.Fatal("last_reload_ok = false, want true")
	}
	if at := metrics.LastReloadAt(); at.Before(before) || at.IsZero() {
		t.Fatalf("last_reload_at = %v, want after %v", at, before)
	}
	if got := metrics.State(); got != metrics.StateRunning {
		t.Fatalf("state = %s, want running", got)
	}
	for _, lis := range metrics.Listeners() {
		if lis.Port == "seed" {
			t.Fatalf("listener copy was not replaced: %+v", metrics.Listeners())
		}
	}
}

func TestConnCounters(t *testing.T) {
	metrics.ResetForTest()
	stubServeConnHandler(t, func(*listener.Listener, net.Conn, context.Context) {})

	s := &Server{}
	ctx := context.Background()
	lis := &listener.Listener{}
	for i := 0; i < 2; i++ {
		a, b := net.Pipe()
		t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
		s.serveConn(lis, b, ctx)
	}
	s.connWg.Wait()

	if metrics.ConnAcceptedTotal() != 2 || metrics.ConnClosedTotal() != 2 || metrics.ConnActive() != 0 {
		t.Fatalf("accepted=%d closed=%d active=%d, want 2/2/0",
			metrics.ConnAcceptedTotal(), metrics.ConnClosedTotal(), metrics.ConnActive())
	}
}

func TestConnCounterPanic(t *testing.T) {
	metrics.ResetForTest()
	stubServeConnHandler(t, func(*listener.Listener, net.Conn, context.Context) {
		panic("handler boom")
	})

	s := &Server{}
	a, b := net.Pipe()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	s.serveConn(&listener.Listener{}, b, context.Background())
	s.connWg.Wait()

	if metrics.ConnAcceptedTotal() != 1 {
		t.Fatalf("accepted = %d, want 1", metrics.ConnAcceptedTotal())
	}
	if metrics.ConnClosedTotal() != 1 {
		t.Fatalf("closed = %d, want 1", metrics.ConnClosedTotal())
	}
	if metrics.ConnActive() != 0 {
		t.Fatalf("active = %d, want 0 after panic", metrics.ConnActive())
	}
}

func TestConnCounterNotCountedWhenStopping(t *testing.T) {
	metrics.ResetForTest()
	called := false
	stubServeConnHandler(t, func(*listener.Listener, net.Conn, context.Context) {
		called = true
	})

	s := &Server{stopping: true}
	a, b := net.Pipe()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	_ = a.SetWriteDeadline(time.Now().Add(time.Second))
	s.serveConn(&listener.Listener{}, b, context.Background())

	if called {
		t.Fatal("discarded connection must not enter the handler")
	}
	if metrics.ConnAcceptedTotal() != 0 || metrics.ConnClosedTotal() != 0 || metrics.ConnActive() != 0 {
		t.Fatalf("stopping discard counted: accepted=%d closed=%d active=%d",
			metrics.ConnAcceptedTotal(), metrics.ConnClosedTotal(), metrics.ConnActive())
	}
	if _, err := a.Write([]byte("x")); err == nil {
		t.Fatal("discarded connection should be closed")
	}
}

func TestStopTimeoutCounter(t *testing.T) {
	// Windows 生产 stop 走 Kill，进不了 drainConnections；超时分支本身
	// 与信号无关。用不返回的处理函数加很短超时覆盖，不等默认的 5 秒。
	metrics.ResetForTest()
	block := make(chan struct{})
	stubServeConnHandler(t, func(*listener.Listener, net.Conn, context.Context) {
		<-block
	})
	s := &Server{}
	a, b := net.Pipe()
	t.Cleanup(func() {
		close(block)
		_ = a.Close()
		_ = b.Close()
		s.connWg.Wait()
	})
	s.serveConn(&listener.Listener{}, b, context.Background())

	before := metrics.StopTimeoutTotal()
	s.drainConnections(20 * time.Millisecond)
	if metrics.StopTimeoutTotal() != before+1 {
		t.Fatalf("stop_timeout_total = %d, want %d", metrics.StopTimeoutTotal(), before+1)
	}
}

func TestListenerSnapshotMapsTypes(t *testing.T) {
	metrics.ResetForTest()
	publishListeners([]*listener.Listener{
		{Port: "8080", LisType: 0},
		{Port: "443", LisType: 10},
		{Port: "8443", LisType: 1},
		{Port: "9000", LisType: 2},
		nil,
	})
	got := metrics.Listeners()
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4 (nil skipped)", len(got))
	}
	want := []metrics.ListenerInfo{
		{Port: "8080"},
		{Port: "443", SSL: true, H2: true},
		{Port: "8443", SSL: true},
		{Port: "9000"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("listeners[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
