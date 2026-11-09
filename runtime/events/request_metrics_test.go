package events

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/modules/safe"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
	"fast-https/runtime/metrics"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

func resetMetrics(t testing.TB) {
	t.Helper()
	metrics.ResetForTest()
	t.Cleanup(metrics.ResetForTest)
}

func registerStub(t testing.TB, handle func(*listener.ListenCfg, *core.Event)) {
	t.Helper()
	core.RegisterHandler(config.DEVMOD,
		func(cfg *listener.ListenCfg, ev *core.Event) bool { return true },
		handle,
		func(*listener.ListenCfg, *core.Event) {})
	t.Cleanup(func() { core.RegisterHandler(config.DEVMOD, nil, nil, nil) })
}

func stubListener() *listener.Listener {
	re := regexp.MustCompile("^/(ok|app|ws|silent)")
	return &listener.Listener{Port: "18080", HostMap: map[string][]listener.ListenCfg{
		"example.com": {{ID: 1, Path: "/", PathRe: re, Type: config.DEVMOD}},
	}}
}

func newHandlerEvent(t *testing.T, lis *listener.Listener, method, path, host string) *core.Event {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	go func() {
		_, _ = io.Copy(io.Discard, client)
	}()
	ev := core.NewEvent(lis, server)
	ev.EventWrite = func(ev *core.Event, _ []byte) error { return nil }
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = method
	ev.RR.Req.Path = path
	ev.RR.Req.Headers["Host"] = host
	ev.RR.Req.Protocol = "HTTP/1.1"
	ev.RR.Res = response.ResponseInit()
	return ev
}

func TestKeepAliveTwoRequests(t *testing.T) {
	resetMetrics(t)
	registerStub(t, func(cfg *listener.ListenCfg, ev *core.Event) {
		ev.RR.Res.SetFirstLine(200, "OK")
		ev.RR.Res.SetHeader("Content-Length", "2")
		ev.RR.Res.Body = []byte("ok")
		ev.Reuse = true
		_ = ev.WriteResponse(nil)
	})

	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	metrics.ConnAccepted()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer metrics.ConnClosed()
		HandleEvent(stubListener(), server, ctx)
	}()

	br := bufio.NewReader(client)
	req := []byte("GET /ok HTTP/1.1\r\nHost: example.com\r\n\r\n")
	for i := 0; i < 2; i++ {
		if _, err := client.Write(req); err != nil {
			t.Fatal(err)
		}
		_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			t.Fatalf("response %d: %v", i, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("HandleEvent did not return")
	}

	if metrics.RequestsTotal() != 2 {
		t.Fatalf("requests_total = %d, want 2", metrics.RequestsTotal())
	}
	if metrics.RequestsActive() != 0 {
		t.Fatalf("requests.active = %d, want 0", metrics.RequestsActive())
	}
	if metrics.ConnAcceptedTotal() != 1 || metrics.ConnActive() != 0 {
		t.Fatalf("connections accepted=%d active=%d, want 1/0",
			metrics.ConnAcceptedTotal(), metrics.ConnActive())
	}
}

func TestStatusBuckets(t *testing.T) {
	resetMetrics(t)
	lis := stubListener()
	registerStub(t, func(cfg *listener.ListenCfg, ev *core.Event) {
		switch ev.RR.Req.Path {
		case "/ok":
			ev.RR.Res.SetFirstLine(200, "OK")
		default:
			ev.RR.Res.SetFirstLine(500, "SERVERERROR")
		}
		_ = ev.WriteResponse(nil)
	})

	EventHandler(newHandlerEvent(t, lis, "GET", "/ok", "example.com"), filters.NewFilter())
	EventHandler(newHandlerEvent(t, lis, "GET", "/nowhere", "example.com"), filters.NewFilter())
	EventHandler(newHandlerEvent(t, lis, "GET", "/app", "example.com"), filters.NewFilter())

	if metrics.Responses2xx() != 1 || metrics.Responses4xx() != 1 || metrics.Responses5xx() != 1 {
		t.Fatalf("2xx=%d 4xx=%d 5xx=%d, want 1/1/1",
			metrics.Responses2xx(), metrics.Responses4xx(), metrics.Responses5xx())
	}
	if metrics.Responses502() != 0 || metrics.Responses504() != 0 {
		t.Fatalf("502=%d 504=%d, want 0/0 for 200 and 500", metrics.Responses502(), metrics.Responses504())
	}
	if metrics.RequestsTotal() != 3 {
		t.Fatalf("requests_total = %d, want 3", metrics.RequestsTotal())
	}
}

func TestGatewayStatusBuckets(t *testing.T) {
	resetMetrics(t)
	re := regexp.MustCompile("^/")
	lis := &listener.Listener{Port: "18080", HostMap: map[string][]listener.ListenCfg{
		"example.com": {{ID: 1, Path: "/", PathRe: re, Type: config.DEVMOD}},
	}}
	registerStub(t, func(cfg *listener.ListenCfg, ev *core.Event) {
		switch ev.RR.Req.Path {
		case "/ok":
			ev.RR.Res.SetFirstLine(200, "OK")
		case "/gw":
			ev.RR.Res.SetFirstLine(502, "BADGATEWAY")
		case "/timeout":
			ev.RR.Res.SetFirstLine(504, "GATEWAYTIMEOUT")
		default:
			ev.RR.Res.SetFirstLine(500, "SERVERERROR")
		}
		_ = ev.WriteResponse(nil)
	})

	EventHandler(newHandlerEvent(t, lis, "GET", "/ok", "example.com"), filters.NewFilter())
	EventHandler(newHandlerEvent(t, lis, "GET", "/gw", "example.com"), filters.NewFilter())
	EventHandler(newHandlerEvent(t, lis, "GET", "/timeout", "example.com"), filters.NewFilter())
	EventHandler(newHandlerEvent(t, lis, "GET", "/other", "example.com"), filters.NewFilter())

	if metrics.Responses2xx() != 1 || metrics.Responses502() != 1 || metrics.Responses504() != 1 || metrics.Responses5xx() != 3 {
		t.Fatalf("2xx=%d 502=%d 504=%d 5xx=%d, want 1/1/1/3",
			metrics.Responses2xx(), metrics.Responses502(), metrics.Responses504(), metrics.Responses5xx())
	}
}

func TestMalformedHeaderIs400(t *testing.T) {
	resetMetrics(t)
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	go func() {
		_, _ = client.Write([]byte("this is not http\r\n\r\n"))
		buf := make([]byte, 512)
		_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _ = client.Read(buf)
	}()
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.EventWrite = EventWrite
	if parseRequest(ev, filters.NewFilter()) != 0 {
		t.Fatal("malformed header should return 0")
	}
	if metrics.ParseErrorsTotal() != 1 {
		t.Fatalf("parse_errors_total = %d, want 1", metrics.ParseErrorsTotal())
	}
	if metrics.RequestsTotal() != 0 {
		t.Fatalf("requests_total = %d, want 0", metrics.RequestsTotal())
	}
}

func TestClientAbortMidRequest(t *testing.T) {
	resetMetrics(t)
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })

	metrics.ConnAccepted()
	done := make(chan int, 1)
	go func() {
		ev := core.NewEvent(&listener.Listener{}, server)
		ev.EventWrite = EventWrite
		done <- parseRequest(ev, filters.NewFilter())
		metrics.ConnClosed()
	}()
	if _, err := client.Write([]byte("GET / HTTP/1.1\r\nHost: ")); err != nil {
		t.Fatal(err)
	}
	_ = client.Close()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("parseRequest = %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parseRequest did not return after client abort")
	}
	if metrics.ParseErrorsTotal() != 0 {
		t.Fatalf("parse_errors_total = %d, want 0", metrics.ParseErrorsTotal())
	}
	if metrics.AbortedTotal() != 1 {
		t.Fatalf("aborted_total = %d, want 1", metrics.AbortedTotal())
	}
	if metrics.ConnActive() != 0 {
		t.Fatalf("connections.active = %d, want 0", metrics.ConnActive())
	}
}

func TestKeepAliveNoStaleStatus(t *testing.T) {
	resetMetrics(t)
	lis := stubListener()
	core.RegisterHandler(config.DEVMOD,
		func(cfg *listener.ListenCfg, ev *core.Event) bool {
			return ev.RR.Req.Path != "/silent"
		},
		func(cfg *listener.ListenCfg, ev *core.Event) {
			ev.RR.Res.SetFirstLine(200, "OK")
			_ = ev.WriteResponse(nil)
		},
		func(*listener.ListenCfg, *core.Event) {})
	t.Cleanup(func() { core.RegisterHandler(config.DEVMOD, nil, nil, nil) })

	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	go func() { _, _ = io.Copy(io.Discard, client) }()
	ev := core.NewEvent(lis, server)
	ev.EventWrite = func(*core.Event, []byte) error { return nil }
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "GET"
	ev.RR.Req.Headers["Host"] = "example.com"
	ev.RR.Req.Protocol = "HTTP/1.1"
	ev.RR.Res = response.ResponseInit()

	ev.RR.Req.Path = "/ok"
	EventHandler(ev, filters.NewFilter())
	ev.RR.Req.Path = "/silent"
	EventHandler(ev, filters.NewFilter())
	ev.RR.Req.Path = "/nowhere"
	EventHandler(ev, filters.NewFilter())
	ev.RR.Req.Path = "/ok"
	EventHandler(ev, filters.NewFilter())

	if metrics.Responses2xx() != 2 || metrics.Responses4xx() != 1 {
		t.Fatalf("2xx=%d 4xx=%d, want 2/1 (stale status leaked)",
			metrics.Responses2xx(), metrics.Responses4xx())
	}
	if metrics.ResponsesUnknown() != 1 {
		t.Fatalf("unknown = %d, want 1 for the request that did not set FirstLine", metrics.ResponsesUnknown())
	}
}

func TestBodyTimeoutAndTooLarge(t *testing.T) {
	resetMetrics(t)
	previous := *config.Current()
	previousMIME := config.MIME()
	t.Cleanup(func() { config.Store(&previous, previousMIME) })
	next := previous
	next.Limit.MaxBodySize = 16
	config.Store(&next, previousMIME)

	t.Run("timeout", func(t *testing.T) {
		metrics.ResetForTest()
		server, client := net.Pipe()
		t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
		go func() {
			_, _ = client.Write([]byte("POST /ok HTTP/1.1\r\nHost: example.com\r\nContent-Length: 100\r\n\r\n"))
		}()
		ev := core.NewEvent(&listener.Listener{}, server)
		ev.EventWrite = EventWrite
		if parseRequest(ev, filters.NewFilter()) != 0 {
			t.Fatal("body timeout should return 0")
		}
		if metrics.AbortedTotal() != 1 || metrics.ParseErrorsTotal() != 0 || metrics.RequestsTotal() != 0 {
			t.Fatalf("aborted=%d parse_errors=%d requests=%d",
				metrics.AbortedTotal(), metrics.ParseErrorsTotal(), metrics.RequestsTotal())
		}
	})

	t.Run("too large", func(t *testing.T) {
		metrics.ResetForTest()
		server, client := net.Pipe()
		t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
		ev := core.NewEvent(&listener.Listener{}, server)
		ev.EventWrite = EventWrite
		done := make(chan int, 1)
		go func() { done <- parseRequest(ev, filters.NewFilter()) }()
		time.Sleep(20 * time.Millisecond)
		if _, err := client.Write([]byte("POST /ok HTTP/1.1\r\nHost: example.com\r\nContent-Length: 100\r\n\r\n")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
		if _, err := client.Write([]byte("0123456789abcdefxxxx")); err != nil {
			t.Fatal(err)
		}
		select {
		case code := <-done:
			if code != 0 {
				t.Fatal("oversize body should return 0")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("parseRequest timed out")
		}
		if metrics.AbortedTotal() != 1 || metrics.ParseErrorsTotal() != 0 || metrics.RequestsTotal() != 0 {
			t.Fatalf("aborted=%d parse_errors=%d requests=%d",
				metrics.AbortedTotal(), metrics.ParseErrorsTotal(), metrics.RequestsTotal())
		}
	})
}

func TestChunkedDecodeFailure(t *testing.T) {
	resetMetrics(t)
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	go func() {
		_, _ = client.Write([]byte("POST /ok HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\nxyz\r\n"))
	}()
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.EventWrite = EventWrite
	if parseRequest(ev, filters.NewFilter()) != 0 {
		t.Fatal("bad chunked body should return 0")
	}
	if metrics.AbortedTotal() != 1 {
		t.Fatalf("aborted_total = %d, want 1", metrics.AbortedTotal())
	}
	if metrics.ParseErrorsTotal() != 0 || metrics.RequestsTotal() != 0 {
		t.Fatalf("parse_errors=%d requests=%d, want 0/0",
			metrics.ParseErrorsTotal(), metrics.RequestsTotal())
	}
}

func TestConnFilterRejected(t *testing.T) {
	resetMetrics(t)
	safe.Init()
	before := safe.Snapshot()
	core.SetServices(core.Services{
		RejectConn: func(*core.Event) bool { return true },
	})
	t.Cleanup(func() { core.SetServices(core.Services{}) })

	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	HandleEvent(&listener.Listener{Port: "1"}, server, context.Background())

	if metrics.RejectedTotal() != 1 {
		t.Fatalf("rejected_total = %d, want 1", metrics.RejectedTotal())
	}
	if metrics.RequestsTotal() != 0 {
		t.Fatalf("requests_total = %d, want 0", metrics.RequestsTotal())
	}
	after := safe.Snapshot()
	if after.TotalChecked != before.TotalChecked {
		t.Fatalf("safe.totalChecked changed %d -> %d; rejected_total is independent",
			before.TotalChecked, after.TotalChecked)
	}
}

func TestWebSocketUpgrade(t *testing.T) {
	resetMetrics(t)
	registerStub(t, func(cfg *listener.ListenCfg, ev *core.Event) {
		ev.RR.Res.SetFirstLine(101, "Switching Protocols")
		_ = ev.WriteResponse(nil)
	})
	frames := 0
	core.SetServices(core.Services{
		HandleListen: func(ev *core.Event) bool {
			if ev.Upgrade != "websocket" {
				return false
			}
			buf := make([]byte, 64)
			for {
				n, err := ev.Conn.Read(buf)
				frames += n
				if err != nil {
					return true
				}
			}
		},
	})
	t.Cleanup(func() { core.SetServices(core.Services{}) })

	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	metrics.ConnAccepted()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer metrics.ConnClosed()
		HandleEvent(stubListener(), server, ctx)
	}()

	br := bufio.NewReader(client)
	upgrade := "GET /ws HTTP/1.1\r\nHost: example.com\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n"
	if _, err := client.Write([]byte(upgrade)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if metrics.RequestsTotal() != 1 {
		t.Fatalf("after upgrade requests_total = %d, want 1", metrics.RequestsTotal())
	}
	if metrics.ConnActive() != 1 {
		t.Fatalf("during session connections.active = %d, want 1", metrics.ConnActive())
	}

	if _, err := client.Write([]byte("frame-one")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write([]byte("frame-two")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if metrics.RequestsTotal() != 1 {
		t.Fatalf("websocket frames incremented requests_total to %d", metrics.RequestsTotal())
	}
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("HandleEvent did not return after websocket close")
	}
	if frames == 0 {
		t.Fatal("websocket session read no frames")
	}
	if metrics.ConnActive() != 0 {
		t.Fatalf("connections.active = %d after session, want 0", metrics.ConnActive())
	}
}

func TestHTTP2ConcurrentStreams(t *testing.T) {
	resetMetrics(t)

	const n = 4
	var started sync.WaitGroup
	started.Add(n)
	release := make(chan struct{})
	core.RegisterHandler(config.DEVMOD,
		func(cfg *listener.ListenCfg, ev *core.Event) bool { return true },
		func(cfg *listener.ListenCfg, ev *core.Event) {
			started.Done()
			<-release
			ev.RR.Res.SetFirstLine(200, "OK")
			_ = ev.WriteResponse(nil)
		},
		func(*listener.ListenCfg, *core.Event) {})
	t.Cleanup(func() { core.RegisterHandler(config.DEVMOD, nil, nil, nil) })

	h := &h2Handler{lis: stubListener(), fif: filters.NewFilter()}
	metrics.ConnAccepted()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("GET", "http://example.com/ok", nil)
			h.ServeHTTP(httptest.NewRecorder(), r)
		}()
	}

	doneStart := make(chan struct{})
	go func() {
		started.Wait()
		close(doneStart)
	}()
	select {
	case <-doneStart:
	case <-time.After(5 * time.Second):
		t.Fatal("streams did not all enter the handler")
	}
	if metrics.RequestsActive() < int64(n) {
		t.Fatalf("requests.active = %d, want >= %d", metrics.RequestsActive(), n)
	}
	if metrics.ConnActive() != 1 {
		t.Fatalf("connections.active = %d, want 1", metrics.ConnActive())
	}
	if metrics.RequestsActive() <= metrics.ConnActive() {
		t.Fatalf("requests.active = %d should exceed connections.active = %d",
			metrics.RequestsActive(), metrics.ConnActive())
	}
	close(release)
	wg.Wait()
	metrics.ConnClosed()

	if metrics.RequestsActive() != 0 || metrics.ConnActive() != 0 {
		t.Fatalf("after streams: requests.active=%d connections.active=%d",
			metrics.RequestsActive(), metrics.ConnActive())
	}
	if metrics.RequestsTotal() != uint64(n) {
		t.Fatalf("requests_total = %d, want %d", metrics.RequestsTotal(), n)
	}
}

func TestH2FillRequestFailure(t *testing.T) {
	resetMetrics(t)
	r := httptest.NewRequest("GET", "http://example.com/", nil)
	long := "/" + strings.Repeat("a", request.MaxPathLength)
	r.URL.Path = long
	r.URL.RawPath = long
	if fillRequest(request.RequestInit(true), r) {
		t.Fatal("fillRequest should reject an oversized path")
	}
	rec := httptest.NewRecorder()
	h := &h2Handler{lis: stubListener(), fif: filters.NewFilter()}
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if metrics.ParseErrorsTotal() != 1 {
		t.Fatalf("parse_errors_total = %d, want 1", metrics.ParseErrorsTotal())
	}
	if metrics.RequestsTotal() != 0 {
		t.Fatalf("requests_total = %d, want 0", metrics.RequestsTotal())
	}
}

func TestPanicInHandler(t *testing.T) {
	resetMetrics(t)
	registerStub(t, func(cfg *listener.ListenCfg, ev *core.Event) {
		panic("handler boom")
	})
	ev := newHandlerEvent(t, stubListener(), "GET", "/ok", "example.com")
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		EventHandler(ev, filters.NewFilter())
	}()
	if metrics.RequestsActive() != 0 {
		t.Fatalf("requests.active = %d after panic, want 0", metrics.RequestsActive())
	}
	if metrics.RequestsTotal() != 1 {
		t.Fatalf("requests_total = %d, want 1", metrics.RequestsTotal())
	}
}
