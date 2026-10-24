package frame

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestReadFrameIgnoresUnknownType(t *testing.T) {
	var raw bytes.Buffer
	writeUnknown(&raw, 0x0a, []byte{0x11, 0x22})
	// SETTINGS ACK follows, so ignoring the unknown frame must keep alignment.
	ack := NewSettingsFrame(ACK, 0, nil)
	if err := ack.Write(&raw); err != nil {
		t.Fatal(err)
	}

	settings := map[SettingsID]int32{
		SETTINGS_MAX_FRAME_SIZE: DEFAULT_MAX_FRAME_SIZE,
	}
	reader := bytes.NewReader(raw.Bytes())

	frame, err := ReadFrame(reader, settings)
	if err != nil {
		t.Fatal(err)
	}
	if frame != nil {
		t.Fatalf("unknown frame should be discarded, got %T", frame)
	}

	next, err := ReadFrame(reader, settings)
	if err != nil {
		t.Fatal(err)
	}
	settingsFrame, ok := next.(*SettingsFrame)
	if !ok {
		t.Fatalf("next frame = %T", next)
	}
	if settingsFrame.Flags != ACK || settingsFrame.Length != 0 {
		t.Fatalf("settings ack flags=%v length=%d", settingsFrame.Flags, settingsFrame.Length)
	}
	if reader.Len() != 0 {
		t.Fatalf("unread bytes %d", reader.Len())
	}
}

func writeUnknown(w io.Writer, frameType byte, payload []byte) {
	var first uint32 = uint32(len(payload))<<8 | uint32(frameType)
	_ = binary.Write(w, binary.BigEndian, first)
	_, _ = w.Write([]byte{0})
	_ = binary.Write(w, binary.BigEndian, uint32(0))
	_, _ = w.Write(payload)
}
