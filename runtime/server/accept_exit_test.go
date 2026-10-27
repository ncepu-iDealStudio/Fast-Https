package server

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"fast-https/runtime/listener"
)

func TestServeListenerExitsWhenContextCanceled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{Listens: []*listener.Listener{{
		Lfd:    ln,
		Port:   "1",
		Ctx:    ctx,
		Cancel: cancel,
	}}}

	done := make(chan struct{})
	go func() {
		s.serveListener(s.Listens[0])
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("accept loop still running after cancel")
	}
}

func TestDrainConnectionsWaitsForTrackedHandlers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{Listens: []*listener.Listener{{
		Lfd:    ln,
		Port:   "1",
		Ctx:    ctx,
		Cancel: cancel,
	}}}

	release := make(chan struct{})
	s.connWg.Add(1)
	go func() {
		<-release
		s.connWg.Done()
	}()

	acceptDone := make(chan struct{})
	go func() {
		s.serveListener(s.Listens[0])
		close(acceptDone)
	}()
	time.Sleep(50 * time.Millisecond)

	var finished sync.WaitGroup
	finished.Add(1)
	go func() {
		defer finished.Done()
		s.drainConnections(2 * time.Second)
	}()

	select {
	case <-acceptDone:
	case <-time.After(time.Second):
		t.Fatal("accept loop still running after cancel")
	}

	select {
	case <-time.After(200 * time.Millisecond):
	case <-func() chan struct{} {
		ch := make(chan struct{})
		go func() {
			finished.Wait()
			close(ch)
		}()
		return ch
	}():
		t.Fatal("drain returned before tracked handlers finished")
	}

	close(release)
	finished.Wait()
}
