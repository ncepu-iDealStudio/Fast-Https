package conn

import (
	"fmt"
	"sync"

	"io"
	"log"
	"time"

	"fast-https/modules/core"
	"fast-https/modules/core/filters"
	"fast-https/modules/core/h2"
	. "fast-https/modules/core/h2/frame"
	"fast-https/modules/core/h2/hpack"
	. "fast-https/utils/color"
	. "fast-https/utils/logger"

	"fast-https/modules/core/h2/util"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

type Conn struct {
	RW           io.ReadWriter
	HpackContext *hpack.Context
	LastStreamID uint32
	Window       *h2.Window
	Settings     map[SettingsID]int32
	PeerSettings map[SettingsID]int32
	Streams      map[uint32]*h2.Stream
	StreamsLock  sync.RWMutex
	WriteChan    chan Frame
	CallBack     func(stream *h2.Stream, ev *core.Event, fif *filters.Filter)
}

func NewConn(rw io.ReadWriter) *Conn {
	conn := &Conn{
		RW:           rw,
		HpackContext: hpack.NewContext(uint32(DEFAULT_HEADER_TABLE_SIZE)),
		Settings:     h2.DefaultSettings,
		PeerSettings: h2.DefaultSettings,
		Window:       h2.NewWindowDefault(),
		Streams:      make(map[uint32]*h2.Stream),
		WriteChan:    make(chan Frame),
	}
	return conn
}

func (conn *Conn) NewStream(streamid uint32, ev *core.Event, fif *filters.Filter) *h2.Stream {
	stream := h2.NewStream(
		streamid,
		conn.WriteChan,
		conn.Settings,
		conn.PeerSettings,
		conn.HpackContext,
		conn.CallBack,
		ev,
		fif,
	)
	Debug("adding new stream (id=%d) total (%d)", stream.ID, len(conn.Streams))
	return stream
}

func (conn *Conn) HandleSettings(settingsFrame *SettingsFrame) {
	if settingsFrame.Flags == ACK {
		// receive ACK
		Trace("receive SETTINGS ACK")
		return
	}

	if settingsFrame.Flags != UNSET {
		Error("unknown flag of SETTINGS Frame %v", settingsFrame.Flags)
		return
	}

	// received SETTINGS Frame
	settings := settingsFrame.Settings

	defaultSettings := map[SettingsID]int32{
		SETTINGS_HEADER_TABLE_SIZE:      DEFAULT_HEADER_TABLE_SIZE,
		SETTINGS_ENABLE_PUSH:            DEFAULT_ENABLE_PUSH,
		SETTINGS_MAX_CONCURRENT_STREAMS: DEFAULT_MAX_CONCURRENT_STREAMS,
		SETTINGS_INITIAL_WINDOW_SIZE:    DEFAULT_INITIAL_WINDOW_SIZE,
		SETTINGS_MAX_FRAME_SIZE:         DEFAULT_MAX_FRAME_SIZE,
		SETTINGS_MAX_HEADER_LIST_SIZE:   DEFAULT_MAX_HEADER_LIST_SIZE,
	}

	// merge with default
	for k, v := range settings {
		defaultSettings[k] = v
	}

	Trace("merged settigns ============")
	for k, v := range defaultSettings {
		Trace("%v:%v", k, v)
	}
	Trace("merged settigns ============")

	// save settings to conn
	conn.Settings = defaultSettings

	// SETTINGS_INITIAL_WINDOW_SIZE
	initialWindowSize, ok := settings[SETTINGS_INITIAL_WINDOW_SIZE]
	if ok {

		if initialWindowSize > 2147483647 { // validate < 2^31-1
			Error("FLOW_CONTROL_ERROR (%s)", "SETTINGS_INITIAL_WINDOW_SIZE too large")
			return
		}

		conn.PeerSettings[SETTINGS_INITIAL_WINDOW_SIZE] = initialWindowSize

		for _, stream := range conn.Streams {
			log.Println("apply settings to stream", stream)
			stream.Window.UpdateInitialSize(initialWindowSize)
			stream.PeerSettings[SETTINGS_INITIAL_WINDOW_SIZE] = initialWindowSize
		}
	}

	// send ACK
	ack := NewSettingsFrame(ACK, 0, h2.NilSettings)
	conn.WriteChan <- ack
}

func (conn *Conn) ReadLoop(ev *core.Event, fif *filters.Filter) {
	Debug("start conn.ReadLoop()")

	for {
		// Read a frame from the connection
		frame, err := ReadFrame(conn.RW, conn.Settings)
		if err != nil {
			Error("%v", err)
			h2Error, ok := err.(*H2Error)
			if ok {
				conn.GoAway(0, h2Error)
			}
			break
		}
		if frame != nil {
			Notice("%v %v", Green("recv"), util.Indent(frame.String()))
		}

		streamID := frame.Header().StreamID
		types := frame.Header().Type

		// CONNECTION LEVEL
		if streamID == 0 {
			if types == DataFrameType ||
				types == HeadersFrameType ||
				types == PriorityFrameType ||
				types == RstStreamFrameType ||
				types == PushPromiseFrameType ||
				types == ContinuationFrameType {

				msg := fmt.Sprintf("%s FRAME for Stream ID 0", types)
				Error("%v", msg)
				conn.GoAway(0, &H2Error{ErrorCode: PROTOCOL_ERROR, AdditiolanDebugData: msg})
				break // TODO: check this flow is correct or not
			}

			// When a SETTINGS frame is received
			if types == SettingsFrameType {
				settingsFrame, ok := frame.(*SettingsFrame)
				if !ok {
					Error("invalid settings frame %v", frame)
					return
				}
				conn.HandleSettings(settingsFrame)
			}

			// Connection Level Window Update
			if types == WindowUpdateFrameType {
				windowUpdateFrame, ok := frame.(*WindowUpdateFrame)
				if !ok {
					Error("invalid window update frame %v", frame)
					return
				}
				Debug("connection window size increment(%v)", int32(windowUpdateFrame.WindowSizeIncrement))
				conn.Window.UpdatePeer(int32(windowUpdateFrame.WindowSizeIncrement))
			}

			// Respond to PING
			if types == PingFrameType {
				// ignore ack
				if frame.Header().Flags != ACK {
					conn.PingACK([]byte("pong    ")) // should be 8 bytes
				}
				continue
			}

			// Handle GOAWAY and close connection
			if types == GoAwayFrameType {
				Debug("stop conn.ReadLoop() by GOAWAY")
				break
			}
		}

		// STREAM LEVEL
		if streamID > 0 {
			if types == SettingsFrameType ||
				types == PingFrameType ||
				types == GoAwayFrameType {

				msg := fmt.Sprintf("%s FRAME for Stream ID not 0", types)
				Error("%v", msg)
				conn.GoAway(0, &H2Error{ErrorCode: PROTOCOL_ERROR, AdditiolanDebugData: msg})
				break // TODO: check this flow is correct or not
			}

			// Consume window if it's a DATA frame
			if types == DataFrameType {
				length := int32(frame.Header().Length)
				conn.WindowConsume(length)
			}

			// If it's a new stream ID, create the corresponding stream
			conn.StreamsLock.Lock()
			stream, ok := conn.Streams[streamID]
			conn.StreamsLock.Unlock()
			if !ok {
				// Create stream with streamID
				stream = conn.NewStream(streamID, ev, fif)
				conn.StreamsLock.Lock()
				conn.Streams[streamID] = stream
				conn.StreamsLock.Unlock()

				// Update last stream id
				if streamID > conn.LastStreamID {
					conn.LastStreamID = streamID
				}
			}

			// Change the state of the stream
			err = stream.ChangeState(frame, h2.RECV)
			if err != nil {
				Error("%v", err)
				h2Error, ok := err.(*H2Error)
				if ok {
					conn.GoAway(0, h2Error)
				}
				break
			}

			// If the stream is closed, remove it from the list
			if stream.State == h2.CLOSED {

				// However, wait 1 second to allow window updates
				// TODO: make this atomic
				go func(streamID uint32) {
					<-time.After(1 * time.Second)
					Info("remove stream(%d) from conn.Streams[]", streamID)
					conn.StreamsLock.Lock()
					conn.Streams[streamID] = nil
					conn.StreamsLock.Unlock()
				}(streamID)
			}

			// Pass the frame to the stream
			stream.ReadChan <- frame
		}
	}

	Debug("stop the readloop")
}

func (conn *Conn) WriteLoop() (err error) {
	Debug("start conn.WriteLoop()")
	for frame := range conn.WriteChan {
		Notice("%v %v", Red("send"), util.Indent(frame.String()))

		// TODO: Check the connection level WindowSize here
		err = frame.Write(conn.RW)
		if err != nil {
			Error("%v", err)
			return err
		}
	}
	return
}

func (conn *Conn) PingACK(opaqueData []byte) {
	Debug("Ping ACK with opaque(%v)", opaqueData)
	pingAck := NewPingFrame(ACK, 0, opaqueData)
	conn.WriteChan <- pingAck
}

func (conn *Conn) GoAway(streamId uint32, h2Error *H2Error) {
	Debug("connection close with GO_AWAY(%v)", h2Error)
	errorCode := h2Error.ErrorCode
	additionalDebugData := []byte(h2Error.AdditiolanDebugData)
	goaway := NewGoAwayFrame(streamId, conn.LastStreamID, errorCode, additionalDebugData)
	conn.WriteChan <- goaway
}

func (conn *Conn) WindowConsume(length int32) {
	Debug("connection window update %d bytes", length)

	// If an update is necessary, it will return
	update := conn.Window.Consume(length)

	// If there's an update, send a WindowUpdate
	if update > 0 {
		conn.WriteChan <- NewWindowUpdateFrame(0, uint32(update))
		conn.Window.Update(update)
	}
}

func (conn *Conn) WriteMagic() (err error) {
	_, err = conn.RW.Write([]byte(h2.CONNECTION_PREFACE))
	if err != nil {
		return err
	}
	Info("%v %q", Red("send"), h2.CONNECTION_PREFACE)
	return
}

func (conn *Conn) ReadMagic() (err error) {
	magic := make([]byte, len(h2.CONNECTION_PREFACE))
	_, err = conn.RW.Read(magic)
	if err != nil {
		return err
	}
	if string(magic) != h2.CONNECTION_PREFACE {
		Info("Invalid Magic String: %q", string(magic))
		return fmt.Errorf("invalid Magic String")
	}
	Info("%v %q", Green("recv"), string(magic))
	return
}

func (conn *Conn) Close() {
	Info("close all conn.Streams")
	for i, stream := range conn.Streams {
		if stream != nil {
			Debug("close stream(%d)", i)
			stream.Close()
		}
	}
	Info("close conn.WriteChan")
	close(conn.WriteChan)
}
