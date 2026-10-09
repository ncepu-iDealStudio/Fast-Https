package h2

import (
	"testing"

	. "fast-https/modules/core/h2/frame"
	"fast-https/modules/core/h2/hpack"
)

func TestHeadersIndexZeroClosesStream(t *testing.T) {
	write := make(chan Frame, 1)
	stream := &Stream{
		ID:           1,
		State:        IDLE,
		Window:       NewWindow(DefaultSettings[SETTINGS_INITIAL_WINDOW_SIZE], DefaultSettings[SETTINGS_INITIAL_WINDOW_SIZE]),
		ReadChan:     make(chan Frame),
		WriteChan:    write,
		Settings:     DefaultSettings,
		PeerSettings: DefaultSettings,
		HpackContext: hpack.NewContext(4096),
		Bucket:       NewBucket(),
	}

	headers := NewHeadersFrame(UNSET, 1, nil, []byte{0x80}, nil)
	stream.Read(headers, nil, nil)

	if !stream.Closed {
		t.Fatal("stream should close")
	}
	select {
	case frame := <-write:
		rst, ok := frame.(*RstStreamFrame)
		if !ok || rst.ErrorCode != COMPRESSION_ERROR {
			t.Fatalf("frame = %#v", frame)
		}
	default:
		t.Fatal("expected RST_STREAM")
	}
}
