package conn_test

import (
	bytespkg "bytes"
	"fast-https/modules/core/h2/conn"
	"io"
	"sync/atomic"
	"testing"
)

type mockRWCloser struct {
	readBuf *bytespkg.Buffer
	closed  int32
}

func newMockRWCloser() *mockRWCloser {
	return &mockRWCloser{readBuf: bytespkg.NewBuffer(nil)}
}

func (m *mockRWCloser) Read(p []byte) (int, error) {
	if atomic.LoadInt32(&m.closed) == 1 {
		return 0, io.EOF
	}
	return m.readBuf.Read(p)
}

func (m *mockRWCloser) Write(p []byte) (int, error) {
	if atomic.LoadInt32(&m.closed) == 1 {
		return 0, io.EOF
	}
	return len(p), nil
}

func (m *mockRWCloser) Close() error {
	atomic.StoreInt32(&m.closed, 1)
	return nil
}

func TestConnCloseIsIdempotent(t *testing.T) {
	rw := newMockRWCloser()
	c := conn.NewConn(rw)

	c.Close()
	c.Close()

	if atomic.LoadInt32(&rw.closed) != 1 {
		t.Fatal("underlying rw should be closed")
	}

	_, ok := <-c.WriteChan
	if ok {
		t.Fatal("write channel should be closed after conn.Close")
	}
}
