package conn_test

import (
	"fast-https/modules/core/h2"
	"fast-https/modules/core/h2/conn"
	"fast-https/modules/core/h2/frame"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

// TestGracefulCloseSendsGoAway verifies that GracefulClose sends a GOAWAY
// frame with NO_ERROR errorCode through WriteChan before closing.
func TestGracefulCloseSendsGoAway(t *testing.T) {
	rw := newMockRWCloser()
	c := conn.NewConn(rw)

	// Drain WriteChan in background, collecting frames
	var frames []frame.Frame
	drainDone := make(chan struct{})
	go func() {
		for f := range c.WriteChan {
			frames = append(frames, f)
		}
		close(drainDone)
	}()

	c.GracefulClose(100 * time.Millisecond)
	<-drainDone

	// Verify at least one GOAWAY frame was sent
	foundGoAway := false
	for _, f := range frames {
		if goaway, ok := f.(*frame.GoAwayFrame); ok {
			foundGoAway = true
			if goaway.ErrorCode != frame.NO_ERROR {
				t.Errorf("GOAWAY ErrorCode = %v, want NO_ERROR", goaway.ErrorCode)
			}
		}
	}
	if !foundGoAway {
		t.Fatal("expected GOAWAY frame to be sent during GracefulClose, got none")
	}

	if atomic.LoadInt32(&rw.closed) != 1 {
		t.Fatal("underlying rw should be closed after GracefulClose")
	}
}

// TestGracefulCloseIsIdempotent verifies that calling GracefulClose and Close
// multiple times is safe (protected by sync.Once).
func TestGracefulCloseIsIdempotent(t *testing.T) {
	rw := newMockRWCloser()
	c := conn.NewConn(rw)

	go func() {
		for range c.WriteChan {
		}
	}()

	c.GracefulClose(50 * time.Millisecond)
	c.GracefulClose(50 * time.Millisecond) // should be no-op
	c.Close()                               // should be no-op
	c.Close()                               // should be no-op

	if atomic.LoadInt32(&rw.closed) != 1 {
		t.Fatal("underlying rw should be closed exactly once")
	}
}

// TestGracefulCloseWaitsForActiveStreams verifies that GracefulClose waits
// for active (non-closed) streams to finish before closing, when they
// complete within the grace period.
func TestGracefulCloseWaitsForActiveStreams(t *testing.T) {
	rw := newMockRWCloser()
	c := conn.NewConn(rw)

	go func() {
		for range c.WriteChan {
		}
	}()

	// Simulate an active stream (ReadChan must be initialized for Close())
	activeStream := &h2.Stream{ID: 1, Closed: false, ReadChan: make(chan frame.Frame)}
	c.StreamsLock.Lock()
	c.Streams[1] = activeStream
	c.StreamsLock.Unlock()

	// Start GracefulClose in background with 1s grace period
	done := make(chan struct{})
	start := time.Now()
	go func() {
		c.GracefulClose(1 * time.Second)
		close(done)
	}()

	// Mark stream as closed after 200ms (simulating request completion)
	time.AfterFunc(200*time.Millisecond, func() {
		c.StreamsLock.Lock()
		c.Streams[1].Closed = true
		c.StreamsLock.Unlock()
	})

	<-done
	elapsed := time.Since(start)

	// Should have waited ~200ms for the stream to complete, not the full 1s
	if elapsed > 800*time.Millisecond {
		t.Errorf("GracefulClose waited too long (%v), should have returned shortly after stream closed", elapsed)
	}
	if elapsed < 150*time.Millisecond {
		t.Errorf("GracefulClose returned too quickly (%v), should have waited for active stream", elapsed)
	}

	if atomic.LoadInt32(&rw.closed) != 1 {
		t.Fatal("underlying rw should be closed")
	}
}

// TestGracefulCloseForceClosesAfterTimeout verifies that GracefulClose
// force-closes remaining active streams after the grace period expires.
func TestGracefulCloseForceClosesAfterTimeout(t *testing.T) {
	rw := newMockRWCloser()
	c := conn.NewConn(rw)

	go func() {
		for range c.WriteChan {
		}
	}()

	// Simulate a stream that never closes (ReadChan initialized for Close() safety)
	stuckStream := &h2.Stream{ID: 1, Closed: false, ReadChan: make(chan frame.Frame)}
	c.StreamsLock.Lock()
	c.Streams[1] = stuckStream
	c.StreamsLock.Unlock()

	// Use short grace period
	start := time.Now()
	c.GracefulClose(300 * time.Millisecond)
	elapsed := time.Since(start)

	// Should have waited ~300ms (the grace period) then force-closed
	if elapsed < 250*time.Millisecond {
		t.Errorf("GracefulClose returned too early (%v), should wait for grace period", elapsed)
	}
	if elapsed > 1*time.Second {
		t.Errorf("GracefulClose took too long (%v), should not exceed grace period significantly", elapsed)
	}

	if atomic.LoadInt32(&rw.closed) != 1 {
		t.Fatal("underlying rw should be force-closed after timeout")
	}
}

// TestGracefulCloseNoActiveStreamsReturnsQuickly verifies that GracefulClose
// returns quickly when there are no active streams.
func TestGracefulCloseNoActiveStreamsReturnsQuickly(t *testing.T) {
	rw := newMockRWCloser()
	c := conn.NewConn(rw)

	go func() {
		for range c.WriteChan {
		}
	}()

	// No streams at all
	start := time.Now()
	c.GracefulClose(5 * time.Second)
	elapsed := time.Since(start)

	// Should return almost immediately since there are no active streams
	if elapsed > 500*time.Millisecond {
		t.Errorf("GracefulClose took %v with no active streams, should be near-instant", elapsed)
	}

	if atomic.LoadInt32(&rw.closed) != 1 {
		t.Fatal("underlying rw should be closed")
	}
}

// TestGracefulCloseClosesUnderlyingRW verifies that GracefulClose closes
// the underlying read/write connection.
func TestGracefulCloseClosesUnderlyingRW(t *testing.T) {
	rw := newMockRWCloser()
	c := conn.NewConn(rw)

	go func() {
		for range c.WriteChan {
		}
	}()

	c.GracefulClose(50 * time.Millisecond)

	// After close, writes to rw should return EOF (closed flag set)
	_, err := rw.Write([]byte("test"))
	if err == nil {
		t.Error("write after close should return error")
	}
}

// Ensure mockRWCloser satisfies io.ReadWriteCloser
var _ io.ReadWriteCloser = (*mockRWCloser)(nil)
