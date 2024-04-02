package events

import (
	"errors"
	"fast-https/modules/core"
	"fast-https/modules/core/filters"
	"fast-https/modules/core/h2"
	"fast-https/modules/core/h2/conn"
	frame "fast-https/modules/core/h2/frame"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/utils/logger"
	"fast-https/utils/message"
	"net/http"
	"strings"

	"github.com/Jxck/hpack"
)

func H2HandleEvent(ev *core.Event, fif *filters.Filter, shutdown *core.ServerControl) {
	ev.EventWrite = H2EventWrite

	Connh2 := conn.NewConn(ev.Conn)

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

	Connh2.ReadLoop(ev, fif)

	Connh2.Close()
}

func CallBack(stream *h2.Stream, ev *core.Event, fif *filters.Filter) {
	header := stream.Bucket.Headers
	// body := stream.Bucket.Body

	// save request information to ev.RR.Req_
	if !ev.RR.CircleInit {
		ev.RR.Req_ = request.ReqInit(true)   // Create a request Object
		ev.RR.Res_ = response.ResponseInit() // Create a res Object
		ev.RR.CircleInit = true
	}

	ev.RR.Req_.Method = header.Get(":method")
	ev.RR.Req_.Path = header.Get(":path")
	ev.RR.Req_.Protocol = "HTTP/2"
	ev.RR.Req_.Headers["Host"] = header.Get(":authority")

	for k, v := range header {
		if !strings.Contains(k, ":") {
			ev.RR.Req_.Headers[k] = v[0]
		}
	}

	ev.Stream = stream

	// Handle HTTP using handler

	EventHandler(ev, fif)

	// ev.WriteData([]byte("hello world"))
}

func H2EventWrite(ev *core.Event, _data []byte) error {
	stream, flag := (ev.Stream).(*h2.Stream)
	if !flag {
		message.PrintErr("--events can not convert ev.Stream data to *h2.Stream")
	}
	responseHeader := http.Header{}
	firstLine := strings.Split(ev.RR.Res_.FirstLine, " ")
	if len(firstLine) != 3 {
		return errors.New("h2 event write invalid response first line")
	}
	responseHeader.Add(":status", firstLine[1])

	for header, content := range ev.RR.Res_.Headers {
		responseHeader.Add(header, content)
	}

	// Send response headers as HEADERS Frame
	headerList := hpack.ToHeaderList(responseHeader)
	headerBlockFragment := stream.HpackContext.Encode(*headerList)
	logger.Debug("%v", headerList)

	headersFrame := frame.NewHeadersFrame(frame.END_HEADERS, stream.ID, nil, headerBlockFragment, nil)
	headersFrame.Headers = responseHeader

	stream.Write(headersFrame)

	// Send response body as DATA Frame
	// each DataFrame has data in window size

	maxFrameSize := stream.PeerSettings[frame.SETTINGS_MAX_FRAME_SIZE]
	data := ev.RR.Res_.GetBody()
	rest := int32(len(data))
	frameSize := rest

	// MaxFrameSize を基準に考え、そこから送れるサイズまで減らして行く
	for {
		logger.Debug("rest data size(%v), current peer(%v) window(%v)", rest, stream.ID, stream.Window)

		// 送り終わってれば終わり
		if rest == 0 {
			break
		}

		frameSize = stream.Window.Consumable(rest)

		if frameSize <= 0 {
			continue
		}

		// MaxFrameSize より大きいなら切り詰める
		if frameSize > maxFrameSize {
			frameSize = maxFrameSize
		}

		logger.Debug("send %v/%v data", frameSize, rest)

		// ここまでに算出した frameSize 分のデータを DATA Frame を作って送る
		dataToSend := make([]byte, frameSize)
		copy(dataToSend, data[:frameSize])
		dataFrame := frame.NewDataFrame(frame.UNSET, stream.ID, dataToSend, nil)
		stream.Write(dataFrame)

		// 送った分を削る
		rest -= frameSize
		copy(data, data[frameSize:])
		data = data[:rest]

		// Peer の Window Size を減らす
		stream.Window.ConsumePeer(frameSize)
	}

	// End Stream in empty DATA Frame
	endDataFrame := frame.NewDataFrame(frame.END_STREAM, stream.ID, nil, nil)
	stream.Write(endDataFrame)

	return nil
}
