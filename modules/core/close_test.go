package core

import (
	"net"
	"testing"

	"fast-https/modules/core/listener"
)

type closeCounter struct {
	net.Conn
	n   int
	err error
}

func (c *closeCounter) Close() error {
	c.n++
	if c.err != nil {
		return c.err
	}
	return c.Conn.Close()
}

func TestCloseIsSilentWhenRepeated(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	counter := &closeCounter{Conn: server, err: net.ErrClosed}
	ev := NewEvent(&listener.Listener{}, counter)

	ev.Close()
	ev.Close()

	if counter.n != 1 {
		t.Fatalf("Close called %d times, want 1", counter.n)
	}
	if !ev.IsClose {
		t.Fatal("connection not marked closed")
	}
}
