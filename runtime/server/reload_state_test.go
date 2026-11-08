package server

import (
	"context"
	"errors"
	"net"
	"runtime"
	"sort"
	"strconv"
	"syscall"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/metrics"
)

func TestReloadSuccessState(t *testing.T) {
	metrics.ResetForTest()
	metrics.RecordProcessStart()
	metrics.MarkRunning()
	metrics.ReplaceListeners([]metrics.ListenerInfo{{Port: "seed"}})

	previous := *config.Current()
	next := previous
	next.Servers = nil
	config.Store(&next, config.MIME())
	t.Cleanup(func() { config.Store(&previous, config.MIME()) })

	oldListener := &listener.Listener{Port: "18094", LisType: 0}
	previousListeners := listener.GLisinfos
	listener.GLisinfos = []*listener.Listener{oldListener}
	t.Cleanup(func() { listener.GLisinfos = previousListeners })

	core.SetServices(core.Services{})
	t.Cleanup(func() { core.SetServices(core.Services{}) })

	block := make(chan struct{})
	stubServeConnHandler(t, func(*listener.Listener, net.Conn, context.Context) {
		<-block
	})
	s := &Server{Listens: []*listener.Listener{oldListener}}
	a, b := net.Pipe()
	t.Cleanup(func() {
		close(block)
		_ = a.Close()
		_ = b.Close()
		s.connWg.Wait()
	})
	s.serveConn(&listener.Listener{}, b, context.Background())

	started := metrics.StartedAt()
	pid := metrics.PID()
	accepted := metrics.ConnAcceptedTotal()
	closed := metrics.ConnClosedTotal()
	failed := metrics.ReloadFailedTotal()
	gen := metrics.Generation()
	if metrics.ConnActive() != 1 {
		t.Fatalf("pre-reload connections.active = %d, want 1", metrics.ConnActive())
	}

	var sawReloading bool
	previousReload := reloadConfig
	reloadConfig = func() error {
		sawReloading = metrics.State() == metrics.StateReloading
		if metrics.ConnActive() != 1 {
			t.Errorf("connections.active = %d during reload, want 1", metrics.ConnActive())
		}
		return nil
	}
	t.Cleanup(func() { reloadConfig = previousReload })

	before := time.Now().Add(-time.Second)
	s.Reload()

	if !sawReloading {
		t.Fatal("successful reload never entered reloading")
	}
	if got := metrics.State(); got != metrics.StateRunning {
		t.Fatalf("state = %s, want running", got)
	}
	if !metrics.StartedAt().Equal(started) {
		t.Fatal("started_at changed on successful reload")
	}
	if metrics.PID() != pid {
		t.Fatal("pid changed on successful reload")
	}
	if metrics.ConnAcceptedTotal() != accepted || metrics.ConnClosedTotal() != closed {
		t.Fatal("connection counters changed on successful reload")
	}
	if metrics.ConnActive() != 1 {
		t.Fatalf("connections.active = %d after reload, want 1", metrics.ConnActive())
	}
	if metrics.ReloadFailedTotal() != failed {
		t.Fatal("reload_failed_total changed on success")
	}
	if metrics.Generation() != gen+1 {
		t.Fatalf("generation = %d, want %d", metrics.Generation(), gen+1)
	}
	if !metrics.LastReloadOK() {
		t.Fatal("last_reload_ok = false, want true")
	}
	if at := metrics.LastReloadAt(); at.Before(before) || at.IsZero() {
		t.Fatalf("last_reload_at = %v, want after %v", at, before)
	}
}

func TestReloadFailureState(t *testing.T) {
	metrics.ResetForTest()
	metrics.RecordProcessStart()
	metrics.MarkRunning()
	metrics.ConnAccepted()
	metrics.NoteReloadSuccess()
	started := metrics.StartedAt()
	gen := metrics.Generation()
	failed := metrics.ReloadFailedTotal()
	accepted := metrics.ConnAcceptedTotal()

	var sawReloading bool
	previousReload := reloadConfig
	reloadConfig = func() error {
		sawReloading = metrics.State() == metrics.StateReloading
		return errors.New("reload rejected by test")
	}
	t.Cleanup(func() { reloadConfig = previousReload })

	s := &Server{}
	s.Reload()

	if !sawReloading {
		t.Fatal("failed reload never entered reloading")
	}
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

func TestListenersAfterReload(t *testing.T) {
	metrics.ResetForTest()
	metrics.RecordProcessStart()
	metrics.MarkRunning()

	portA := freeTCPPort(t)
	portB := freeTCPPort(t)

	stubServeConnHandler(t, func(*listener.Listener, net.Conn, context.Context) {})

	previous := *config.Current()
	config.Store(&previous, config.MIME())
	t.Cleanup(func() { config.Store(&previous, config.MIME()) })

	var s *Server
	prevLis := listener.GLisinfos
	t.Cleanup(func() {
		for _, lis := range listener.GLisinfos {
			closeListener(lis)
		}
		if s != nil {
			s.connWg.Wait()
		}
		listener.GLisinfos = prevLis
	})

	core.SetServices(core.Services{})
	t.Cleanup(func() { core.SetServices(core.Services{}) })

	storeHTTPServers(t, httpTestServer(portA))
	listens := listener.ListenWithCfg()
	s = &Server{Listens: listens}
	publishListeners(s.Listens)
	assertListenSet(t, s, portA)
	assertPortState(t, portA, true)
	assertPortState(t, portB, false)

	previousReload := reloadConfig
	reloadConfig = func() error { return nil }
	t.Cleanup(func() { reloadConfig = previousReload })

	storeHTTPServers(t, httpTestServer(portA), httpTestServer(portB))
	s.Reload()
	assertListenSet(t, s, portA, portB)
	assertPortState(t, portA, true)
	assertPortState(t, portB, true)

	storeHTTPServers(t, httpTestServer(portB))
	s.Reload()
	assertListenSet(t, s, portB)
	assertPortState(t, portA, false)
	assertPortState(t, portB, true)
}

func TestActiveGaugesReturnToZero(t *testing.T) {
	metrics.ResetForTest()
	stubServeConnHandler(t, func(*listener.Listener, net.Conn, context.Context) {})

	s := &Server{}
	const n = 32
	for i := 0; i < n; i++ {
		a, b := net.Pipe()
		t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
		s.serveConn(&listener.Listener{}, b, context.Background())
	}
	s.connWg.Wait()

	if metrics.ConnActive() != 0 {
		t.Fatalf("connections.active = %d, want 0", metrics.ConnActive())
	}
	if metrics.ConnAcceptedTotal() != n || metrics.ConnClosedTotal() != n {
		t.Fatalf("accepted=%d closed=%d, want %d/%d",
			metrics.ConnAcceptedTotal(), metrics.ConnClosedTotal(), n, n)
	}
}

func TestGracefulStopMarksStopping(t *testing.T) {
	metrics.ResetForTest()
	metrics.MarkRunning()
	s := &Server{}
	s.Wg.Add(1)
	s.gracefulStop()
	if got := metrics.State(); got != metrics.StateStopping {
		t.Fatalf("state = %s, want stopping", got)
	}
}

func TestSigtermMarksStopping(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows stop uses Kill, not SIGTERM; stopping is not observable from outside")
	}
	metrics.ResetForTest()
	metrics.MarkRunning()
	s := &Server{}
	s.Wg.Add(1)
	s.SigHandler(syscall.SIGTERM)
	if got := metrics.State(); got != metrics.StateStopping {
		t.Fatalf("state = %s, want stopping", got)
	}
}

func freeTCPPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func httpTestServer(port string) config.Server {
	return config.Server{
		Listen:     port,
		ServerName: "localhost",
		Path: []config.Path{{
			PathName: "/",
			PathType: config.LOCAL,
			Root:     ".",
			Index:    []string{"index.html"},
		}},
	}
}

func storeHTTPServers(t *testing.T, servers ...config.Server) {
	t.Helper()
	cur := *config.Current()
	next := cur
	next.Servers = append([]config.Server(nil), servers...)
	config.Store(&next, config.MIME())
}

func closeListener(lis *listener.Listener) {
	if lis == nil {
		return
	}
	if lis.Cancel != nil {
		lis.Cancel()
	}
	if lis.Lfd != nil {
		_ = lis.Lfd.Close()
	}
}

func assertListenSet(t *testing.T, s *Server, want ...string) {
	t.Helper()
	gotLive := make([]string, 0, len(s.Listens))
	for _, lis := range s.Listens {
		if lis != nil {
			gotLive = append(gotLive, lis.Port)
		}
	}
	gotSnap := make([]string, 0, len(metrics.Listeners()))
	for _, lis := range metrics.Listeners() {
		gotSnap = append(gotSnap, lis.Port)
	}
	sort.Strings(gotLive)
	sort.Strings(gotSnap)
	sort.Strings(want)
	if !stringSlicesEqual(gotLive, want) {
		t.Fatalf("s.Listens ports = %v, want %v", gotLive, want)
	}
	if !stringSlicesEqual(gotSnap, want) {
		t.Fatalf("metrics listeners = %v, want %v", gotSnap, want)
	}
}

func assertPortState(t *testing.T, port string, wantOpen bool) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 400*time.Millisecond)
	open := err == nil
	if conn != nil {
		_ = conn.Close()
	}
	if open != wantOpen {
		t.Fatalf("port %s open=%v, want %v (err=%v)", port, open, wantOpen, err)
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
