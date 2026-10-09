package server

import (
	"context"
	"net"
	"testing"
	"time"

	"fast-https/modules/core/listener"
)

func TestServeListenerExitsWhenContextCanceled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{Listens: []listener.Listener{{
		Lfd:    ln,
		Port:   "1",
		Ctx:    ctx,
		Cancel: cancel,
	}}}

	done := make(chan struct{})
	go func() {
		s.serveListener(0, 1)
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
