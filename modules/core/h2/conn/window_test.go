package conn

import (
	"bytes"
	"strings"
	"testing"

	"fast-https/modules/core/h2"
	. "fast-https/modules/core/h2/frame"
)

type closeBuf struct {
	bytes.Buffer
	closed bool
}

func (c *closeBuf) Close() error {
	c.closed = true
	return nil
}

func TestWriteLoopStopsWhenPeerWindowExhausted(t *testing.T) {
	rw := &closeBuf{}
	c := NewConn(rw)
	c.Window = h2.NewWindow(65535, 0)
	c.WriteChan <- NewDataFrame(UNSET, 1, []byte("hi"), nil)

	err := c.WriteLoop()
	if err == nil || !strings.Contains(err.Error(), "peer window exhausted") {
		t.Fatalf("err = %v", err)
	}
	if rw.Len() != 0 {
		t.Fatalf("wrote %d bytes", rw.Len())
	}
	if !rw.closed {
		t.Fatal("connection should close")
	}
}

func TestWriteLoopConsumesPeerWindow(t *testing.T) {
	rw := &closeBuf{}
	c := NewConn(rw)
	c.Window = h2.NewWindow(65535, 8)
	c.WriteChan <- NewDataFrame(UNSET, 1, []byte("hi"), nil)
	close(c.WriteChan)

	if err := c.WriteLoop(); err != nil {
		t.Fatal(err)
	}
	if c.Window.PeerSize() != 6 {
		t.Fatalf("peer window = %d", c.Window.PeerSize())
	}
	if rw.Len() == 0 {
		t.Fatal("data frame should be written")
	}
}
