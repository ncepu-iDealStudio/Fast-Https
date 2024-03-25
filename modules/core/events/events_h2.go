package events

import (
	"fast-https/modules/core"
	"fast-https/modules/core/filters"
	"fast-https/modules/core/h2"
	frame "fast-https/modules/core/h2/frame"
	"fast-https/utils/logger"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Jxck/hpack"
)

func CallBack(stream *h2.Stream, ev *core.Event, fif *filters.Filter) {
	header := stream.Bucket.Headers
	body := stream.Bucket.Body
	method := header.Get(":method")

	for k, v := range header {
		fmt.Println(k, v)
	}
	fmt.Println(body)
	fmt.Println(method)

	// Handle HTTP using handler
	EventHandler(ev, fif)
}

func h2Response(stream *h2.Stream, ev *core.Event) {
	responseHeader := http.Header{}
	responseHeader.Add(":status", strconv.Itoa(200))

	// Send response headers as HEADERS Frame
	headerList := hpack.ToHeaderList(responseHeader)
	headerBlockFragment := stream.HpackContext.Encode(*headerList)
	logger.Debug("%v", headerList)

	headersFrame := frame.NewHeadersFrame(frame.END_HEADERS, stream.ID, nil, headerBlockFragment, nil)
	headersFrame.Headers = responseHeader

	stream.Write(headersFrame)

	// Send response body as DATA Frame
	// each DataFrame has data in window size
	data := []byte("hello world")
	maxFrameSize := stream.PeerSettings[frame.SETTINGS_MAX_FRAME_SIZE]
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
}
