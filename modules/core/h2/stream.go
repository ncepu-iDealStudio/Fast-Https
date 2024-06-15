package h2

import (
	"bytes"
	"log"
	"net/http"

	"fast-https/modules/core"
	"fast-https/modules/core/filters"
	. "fast-https/modules/core/h2/frame"
	. "fast-https/utils/logger"

	"fast-https/modules/core/h2/hpack"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

type Body struct {
	bytes.Buffer
}

func (b *Body) Close() error {
	return nil
}

type Stream struct {
	ID           uint32
	State        State
	Window       *Window
	ReadChan     chan Frame
	WriteChan    chan Frame
	Settings     map[SettingsID]int32
	PeerSettings map[SettingsID]int32
	HpackContext *hpack.Context
	CallBack     CallBack
	Bucket       *Bucket
	Closed       bool
}

type Bucket struct {
	Headers http.Header
	Body    *Body
}

func NewBucket() *Bucket {
	return &Bucket{
		Headers: make(http.Header),
		Body:    new(Body),
	}
}

type CallBack func(stream *Stream, ev *core.Event, fif *filters.Filter)

func NewStream(id uint32, writeChan chan Frame, settings, peerSettings map[SettingsID]int32,
	hpackContext *hpack.Context, callback CallBack, ev *core.Event, fif *filters.Filter) *Stream {
	stream := &Stream{
		ID:    id,
		State: IDLE,
		Window: NewWindow(settings[SETTINGS_INITIAL_WINDOW_SIZE],
			peerSettings[SETTINGS_INITIAL_WINDOW_SIZE]),

		ReadChan:     make(chan Frame),
		WriteChan:    writeChan,
		Settings:     settings,
		PeerSettings: peerSettings,
		HpackContext: hpackContext,
		CallBack:     callback,
		Bucket:       NewBucket(),
		Closed:       false,
	}
	go stream.ReadLoop(ev, fif)
	return stream
}

func (stream *Stream) Read(f Frame, ev *core.Event, fif *filters.Filter) {
	Debug("stream (%d) recv (%v)", stream.ID, f.Header().Type)

	switch frame := f.(type) {
	case *HeadersFrame:
		// Decode Headers
		header := stream.DecodeHeader(frame.HeaderBlockFragment)
		frame.Headers = header

		for name, values := range header {
			for _, value := range values {
				stream.Bucket.Headers.Add(name, value)
			}
		}

		if frame.Header().Flags&END_STREAM == END_STREAM {
			go stream.CallBack(stream, ev, fif)
		}
	case *DataFrame:
		length := int32(frame.Header().Length)
		stream.WindowUpdate(length)

		_, err := stream.Bucket.Body.Write(frame.Data)
		if err != nil {
			Fatal("%v", err)
		}

		if frame.Header().Flags&END_STREAM == END_STREAM {
			stream.CallBack(stream, ev, fif)
		}
	case *RstStreamFrame:
		Debug("close stream by RST_STREAM")
		Error("RST_STREAM(%v)", frame.ErrorCode)
		stream.Close()
	case *PingFrame:
		Debug("response to PING")
		pong := NewPingFrame(ACK, stream.ID, frame.OpaqueData)
		stream.Write(pong)
	case *WindowUpdateFrame:
		Info("Window Update %d byte stream(%v)", frame.WindowSizeIncrement, stream.ID)
		stream.Window.UpdatePeer(int32(frame.WindowSizeIncrement))
	case *ContinuationFrame:
		// Decode Headers
		header := stream.DecodeHeader(frame.HeaderBlockFragment)
		frame.Headers = header

		for name, values := range header {
			for _, value := range values {
				stream.Bucket.Headers.Add(name, value)
			}
		}

		if frame.Header().Flags&END_STREAM == END_STREAM {
			go stream.CallBack(stream, ev, fif)
		}
	}
}

func (stream *Stream) ReadLoop(ev *core.Event, fif *filters.Filter) {
	Debug("start stream (%d) ReadLoop()", stream.ID)
	for f := range stream.ReadChan {
		stream.Read(f, ev, fif)
	}
	Debug("stop stream (%d) ReadLoop()", stream.ID)
}

func (stream *Stream) Write(frame Frame) {
	Trace("stream.Write (%v)", frame)
	if stream.Closed {
		return
	}
	stream.ChangeState(frame, SEND)
	stream.WriteChan <- frame
}

func (stream *Stream) WindowUpdate(length int32) {
	Debug("stream(%d) window update %d bytes", stream.ID, length)

	// If an update is necessary, it will return
	update := stream.Window.Consume(length)

	// If there's an update, send a WindowUpdate
	if update > 0 {
		stream.Write(NewWindowUpdateFrame(stream.ID, uint32(update)))
		stream.Window.Update(update)
	}
}

func (stream *Stream) Close() {
	Debug("stream(%d) Close()", stream.ID)
	// stream.WriteChan is conn.WriteChan, and it will be closed on the conn side,
	// so it will not be closed here
	stream.Closed = true
	Info("close stream(%v).ReadChan", stream.ID)
	close(stream.ReadChan)
}

// Encode Header using HPACK
func (stream *Stream) EncodeHeader(header http.Header) []byte {
	headerList := hpack.ToHeaderList(header)
	Trace("sending header list %s", headerList)
	return stream.HpackContext.Encode(*headerList)
}

// Decode Header using HPACK
func (stream *Stream) DecodeHeader(headerBlockFragment []byte) http.Header {
	stream.HpackContext.Decode(headerBlockFragment)
	return stream.HpackContext.ES.ToHeader()
}
