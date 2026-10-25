// Package events 将接受的连接分发给已注册的 HTTP/1.1 与 HTTP/2 处理器。
package events

import (
	"context"
	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"fast-https/infra/color"
	"fast-https/infra/logging"
	"net"
	"time"
)

func HandleEvent(l *listener.Listener, conn net.Conn, ctx context.Context) {
	ev := core.NewEvent(l, conn)
	defer ev.Close()
	stopWatch := watchClose(ctx, ev)
	defer stopWatch()

	fif := filters.NewFilter() // Filter interface

	if !fif.Fif.ConnFilter(ev) {
		return
	}

	ev.EventWrite = EventWrite

	for {
		select {
		case <-ctx.Done():
			logging.ConsoleLog.Debug().Msgf("Server on port %s is shutting down...\n", l.Port)
			return
		default:
			// websocket and tcp proxy through this
			if fif.Fif.ListenFilter(ev) {
				return
			}

			// parse failure and read timeout share this exit
			if parseRequest(ev, fif) != 1 {
				return
			}

			EventHandler(ev, fif)

			if !ev.EventReuse() {
				return
			}
		}
	}
}

// watchClose closes the connection when the listener context is cancelled,
// so a blocked read returns and the handler goroutine can exit.
func watchClose(ctx context.Context, ev *core.Event) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			ev.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

// distribute event
func EventHandler(ev *core.Event, fif *filters.Filter) {

	cfg, ok := fif.Fif.RequestFilter(ev)
	if !ok {
		// dynlog.Log(&ev.Log, ev, "")
		ev.RR.Res = response.DefaultNotFound()
		ev.WriteResponseClose(nil)
		return
	}
	// found specific "servername && url"

	if core.LocationRejected(cfg, ev) {
		return
	}

	if core.AuthRejected(cfg, ev) {
		return
	}
	// 应用防火墙：全项目唯一的检测检查点（location 匹配与认证之后，
	// handler 分发之前），每请求恰好一次；未配置防火墙时零开销放行。
	// 经 core.Services 钩子注入，runtime 不反向依赖功能包。
	if core.WAFRejected(cfg, ev) {
		return
	}
	ev.Type = cfg.Type
	// according to user's confgure and requets endporint handle events
	handler, ok := core.Handler(int(cfg.Type))
	if !ok || handler.RRHandler == nil {
		// 启动期 MissingHandlers 会拦下这种配置，这里只兜住运行期被撤销注册的极端情况。
		ev.RR.Res = response.DefaultServerError()
		ev.WriteResponseClose(nil)
		return
	}
	ev.RR.CircleHandler = handler
	if ev.RR.CircleHandler.ParseCommandHandler == nil {
		ev.RR.CircleHandler.ParseCommandHandler = core.DefaultParseCommandHandler
	}
	ev.RR.CircleHandler.ParseCommandHandler(cfg, ev)
	if !ev.RR.CircleHandler.FilterHandler(cfg, ev) {
		return
	}
	ev.RR.CircleHandler.RRHandler(cfg, ev)
}

func parseRequest(ev *core.Event, fif *filters.Filter) int {
	rr := &ev.RR
	// save request information to ev.RR.Req
	if !rr.CircleInit {
		rr.Req = request.RequestInit(false) // Create a request Object
		rr.Res = response.ResponseInit()    // Create a res Object
		rr.CircleInit = true
		rr.ReqBuf = make([]byte, core.READ_BODY_BUF_LEN)
	} else {
		rr.Req.Flush()
	}

	// 读缓冲从池里取：byte_row 可能别名这段缓冲，但 parseRequest
	// 返回前所有需要保留的内容都已拷贝进 Req/Body，归还后无人再读它
	readBuf := core.GetReadBuf()
	defer core.PutReadBuf(readBuf)

	// read data (bytes and str) from socket
	byte_row := ev.ReadRequestInto(readBuf)

	if byte_row == nil { // client closed
		return 0
	}

	// 头可能分多次到达：读完整或判非法为止，超过头长度上限按 400 拒绝
	for {
		parse := rr.Req.ParseHeader(byte_row)
		if parse == request.RequestOk {
			break
		}
		if parse != request.RequestNeedReadMore || len(byte_row) > maxHeaderSize() {
			// 畸形请求拒绝处理，不能带着空/半截头走进后续管线
			ev.RR.Res = response.DefaultBadRequest()
			ev.WriteResponseClose(nil)
			return 0
		}
		ev.Conn.SetReadDeadline(time.Now().Add(time.Second * 30))
		n, err := ev.Conn.Read(rr.ReqBuf)
		if err != nil || n == 0 {
			return 0
		}
		byte_row = append(byte_row, rr.ReqBuf[:n]...)
	}

	if !fif.Fif.HttpParseFilter(rr) {
		return -300
	}

	// parse host
	rr.Req.ParseHost(ev.LisInfo)

	if err := rr.Req.ParseBody(byte_row); err != nil {
		logging.ConsoleLog.Debug().Msgf("%s", color.Red(err.Error()))
	}

	if request.HasToken(rr.Req.HeaderFold("Transfer-Encoding"), "chunked") {
		body, err := request.ReadChunked(rr.Req.Body.Bytes(), func() ([]byte, error) {
			ev.Conn.SetReadDeadline(time.Now().Add(time.Second))
			n, err := ev.Conn.Read(rr.ReqBuf)
			if err != nil {
				return nil, err
			}
			return append([]byte(nil), rr.ReqBuf[:n]...), nil
		}, maxBodySize())
		if err != nil {
			logging.ConsoleLog.Debug().Msgf("%s", color.Red(err.Error()))
			return 0
		}
		rr.Req.Body.Reset()
		rr.Req.Body.Write(body)
		return 1
	}

	// headerOtherData := make([]byte, core.READ_BODY_BUF_LEN)
	for {
		if rr.Req.RequestBodyValid() {
			break
		} else {
			ev.Conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			datasize, err := ev.Conn.Read(rr.ReqBuf)
			if err != nil { // read error, like time out
				logging.ConsoleLog.Debug().Msgf("%s", color.Red("read body error"))
				return 0
			}
			byte_row = append(byte_row, rr.ReqBuf[:datasize]...)
			rr.Req.TryFixBody(rr.ReqBuf[:datasize])
			if rr.Req.Body.Len() > config.Current().Limit.MaxBodySize {
				// body bytes beyond config
				logging.ConsoleLog.Debug().Msgf("%s", color.Red("read body too big"))
				return 0
			}
		}
	}

	return 1
}

func maxBodySize() int {
	if config.Current().Limit.MaxBodySize > 0 {
		return config.Current().Limit.MaxBodySize
	}
	return config.DEFAULT_MAX_BODY_SIZE
}

func maxHeaderSize() int {
	if config.Current().Limit.MaxHeaderSize > 0 {
		return config.Current().Limit.MaxHeaderSize
	}
	return config.DEFAULT_MAX_HEADER_SIZE
}

func EventWrite(ev *core.Event, _data []byte) error {
	//fmt.Printf("%p", ev)
	ev.Conn.SetWriteDeadline(time.Now().Add(time.Second * 30))
	data := ev.ResponseBytes()
	// data := []byte("HTTP/1.1 200 OK\r\nContent-Length: 11\r\n\r\nhello world")
	for len(data) > 0 {
		n, err := ev.Conn.Write(data)
		if err != nil {
			ev.Reuse = false
			if ev.CheckIfTimeOut(err) {
				logging.PrintWarn("Warn  --core " + ev.Conn.RemoteAddr().String() + " write timeout")
				return err
			} else { // other error can not handle temporarily
				logging.PrintWarn("Error --core "+ev.Conn.RemoteAddr().String()+" writing to client ", err.Error())
				return err
			}
		}
		data = data[n:]
	}
	return nil
}
