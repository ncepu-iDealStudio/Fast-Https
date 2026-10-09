package events

import (
	"context"
	"net"
	"testing"
	"time"

	"fast-https/modules/core/listener"
)

func TestHandleEventExitsWhenContextCanceled(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		HandleEvent(&listener.Listener{Port: "1"}, server, ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler still running after cancel")
	}
}
