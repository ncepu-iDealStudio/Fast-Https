package events

import (
	"fast-https/modules/core"
	"fast-https/modules/core/filters"
	"fast-https/modules/core/h2"
	"fast-https/modules/core/h2/conn"
	frame "fast-https/modules/core/h2/frame"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/utils/logger"
	"fast-https/utils/message"
	"net"
	"net/http"
	"strings"

	"fast-https/modules/core/h2/hpack"
)

// TODO: http2 not support reload !!!
func H2HandleEvent(l *listener.Listener, conn1 net.Conn, shutdown *core.ServerControl, port_index int) {

	ev_conn := core.NewEvent(l, conn1)

	fif := filters.NewFilter() // Filter interface
	ev_conn.EventWrite = H2EventWrite

	Connh2 := conn.NewConn(ev_conn.Conn)

	err := Connh2.ReadMagic()
	if err != nil {
		message.PrintWarn(err)
		Connh2.Close()
		return
	}

	Connh2.CallBack = CallBack

	go Connh2.WriteLoop()
	settingsFrame := frame.NewSettingsFrame(frame.UNSET, 0, h2.DefaultSettings)
	Connh2.WriteChan <- settingsFrame

	Connh2.ReadLoop(ev_conn, fif)

	Connh2.Close()
}

func CallBack(stream *h2.Stream, ev_conn *core.Event, fif *filters.Filter) {

	stream_ev := core.Event{
		Conn:       ev_conn.Conn,
		LisInfo:    ev_conn.LisInfo,
		Timer:      nil,
		Reuse:      false,
		Log:        *core.NewLogger(),
		IsClose:    false, // not close
		ReadReady:  true,  // need read
		WriteReady: false, // needn't write
		RR: core.RRcircle{
			Ev:            nil, // include each other
			IsCircle:      true,
			CircleInit:    false,
			ProxyConnInit: false,
			CircleCommandVal: core.RRcircleCommandVal{
				Map: make(map[string]string), // init CircleCommandVal map
			},
		},
	}
	stream_ev.RR.Ev = &stream_ev
	// fmt.Printf("	init stream ev ptr: %p\n", &stream_ev)

	header := stream.Bucket.Headers
	body := stream.Bucket.Body
	stream_ev.RR.Req = request.RequestInit(true) // Create a request Object
	stream_ev.RR.Res = response.ResponseInit()   // Create a res Object
	stream_ev.RR.Req.Method = header.Get(":method")
	stream_ev.RR.Req.Path = header.Get(":path")
	stream_ev.RR.Req.Protocol = "HTTP/2"
	stream_ev.RR.Req.Headers["Host"] = header.Get(":authority")
	stream_ev.RR.Req.Body = body.Buffer

	stream_ev.EventWrite = H2EventWrite
	stream_ev.Stream = stream

	for k, v := range header {
		if !strings.Contains(k, ":") {
			stream_ev.RR.Req.Headers[k] = v[0]
		}
	}

	//fmt.Printf("\tev ptr: %p | stream_ev ptr: %p\n", ev, &stream_ev)
	//fmt.Printf("\tstream ptr: %p\n", stream)
	// Handle HTTP using handler
	EventHandler(&stream_ev, fif)

	// ev.WriteData([]byte("hello world"))
}

func H2EventWrite(ev *core.Event, data []byte) error {

	// fmt.Printf("	write body stream ev ptr: %p\n", ev)

	stream, flag := (ev.Stream).(*h2.Stream)
	if !flag {
		message.PrintErr("--events can not convert ev.Stream data to *h2.Stream")
	}

	// Send response body as DATA Frame
	// each DataFrame has data in window size

	maxFrameSize := stream.PeerSettings[frame.SETTINGS_MAX_FRAME_SIZE]
	//data := ev.RR.Res.GetBody()
	rest := int32(len(data))
	frameSize := rest

	// Consider the MaxFrameSize as the basis, and then reduce it to the size that can be sent
	for {
		logger.Debug("rest data size(%v), current peer(%v) window(%v)", rest, stream.ID, stream.Window)

		// If it's done sending, then finish
		if rest == 0 {
			break
		}

		frameSize = stream.Window.Consumable(rest)

		if frameSize <= 0 {
			continue
		}

		// If it's larger than MaxFrameSize, truncate it
		if frameSize > maxFrameSize {
			frameSize = maxFrameSize
		}

		logger.Debug("send %v/%v data", frameSize, rest)

		// Create and send a DATA Frame with the frameSize of data calculated up to this point
		dataToSend := make([]byte, frameSize)
		copy(dataToSend, data[:frameSize])
		dataFrame := frame.NewDataFrame(frame.UNSET, stream.ID, dataToSend, nil)
		stream.Write(dataFrame)

		// Subtract the sent portion
		rest -= frameSize
		copy(data, data[frameSize:])
		data = data[:rest]

		// Reduce the Peer’s Window Size
		stream.Window.ConsumePeer(frameSize)
	}

	// End Stream in empty DATA Frame
	endDataFrame := frame.NewDataFrame(frame.END_STREAM, stream.ID, nil, nil)
	stream.Write(endDataFrame)

	return nil
}

func WriteHeader(ev *core.Event, header http.Header) error {
	stream, flag := (ev.Stream).(*h2.Stream)
	if !flag {
		message.PrintErr("--events can not convert ev.Stream data to *h2.Stream")
	}

	// fmt.Printf("	write header stream ev ptr: %p\n", ev)

	// Send response headers as HEADERS Frame
	headerList := hpack.ToHeaderList(header)
	headerBlockFragment := stream.HpackContext.Encode(*headerList)
	logger.Debug("%v", headerList)

	headersFrame := frame.NewHeadersFrame(frame.END_HEADERS, stream.ID, nil, headerBlockFragment, nil)
	headersFrame.Headers = header

	stream.Write(headersFrame)

	return nil
}
