// Package events 将接受的连接分发给已注册的 HTTP/1.1 与 HTTP/2 处理器。
package events

import (
	"bytes"
	"context"
	"errors"
	"fast-https/config"
	"fast-https/core"
	"fast-https/infra/color"
	"fast-https/infra/logging"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
	"fast-https/runtime/metrics"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"net"
	"strconv"
	"strings"
	"time"
)

func HandleEvent(l *listener.Listener, conn net.Conn, ctx context.Context) {
	ev := core.NewEvent(l, conn)
	defer ev.Close()
	stopWatch := watchClose(ctx, ev)
	defer stopWatch()

	fif := filters.NewFilter() // Filter interface

	// 连接级钩子（黑名单、全局限流）在 HTTP 解析前就会写拒绝响应，
	// 因此 EventWrite 必须在 ConnFilter 之前装配，否则写响应时空指针。
	ev.EventWrite = EventWrite

	if !fif.Fif.ConnFilter(ev) {
		metrics.NoteRejected()
		return
	}

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
	// 在途计数覆盖整条处理链路（含 panic 展开），reload 用它标注受影响请求数。
	// 只统计 HTTP 请求；websocket/tcp 隧道由 ListenFilter 直接接管，不计入。
	if ev != nil && ev.LisInfo != nil {
		ev.LisInfo.BeginRequest()
		defer ev.LisInfo.EndRequest()
	}
	metrics.RequestBegin()
	started := time.Now()
	defer func() {
		metrics.ObserveDuration(time.Since(started))
		line := ""
		if ev != nil && ev.RR.Res != nil {
			line = ev.RR.Res.FirstLine
		}
		metrics.RequestEnd(line)
	}()
	// keep-alive 复用同一个 Response：进入本请求前清掉上一条的状态行，
	// 否则漏设 FirstLine 的路径会把上一请求的码算进本请求。
	if ev.RR.Res != nil {
		ev.RR.Res.FirstLine = ""
	}

	// 保留路径（如 ACME http-01 挑战）在 location 匹配之前应答：
	// 挑战响应不能依赖站点根目录里是否恰好有那个文件。
	if core.RequestHandled(ev) {
		return
	}

	// Host 校验先于路由：enforce 模式下未命中任何 server_name 直接 421，
	// 不落到默认 server（默认 off，保持既有回落行为）。
	if filters.HostCheckRejects(ev) {
		ev.RR.Res = response.DefaultMisdirected()
		ev.WriteResponseClose(nil)
		return
	}

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

	// 方法限制在认证/防火墙之前：允许的方法列表是路由契约，
	// 不该因为缺凭据而先返回 401。
	if !filters.MethodAllowed(cfg, ev.RR.Req.Method) {
		ev.RR.Res = response.DefaultMethodNotAllowed(cfg.AllowMethods)
		ev.WriteResponseClose(nil)
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
		// 这条 500 之前完全静默，配置改错时看不出是哪个路由缺处理器。
		host, path := "", ""
		if ev.RR.Req != nil {
			host, path = ev.RR.Req.GetHost(), ev.RR.Req.Path
		}
		logging.ConsoleLog.Error().Msgf("events: no handler for type %d (host=%s path=%s), returning 500",
			cfg.Type, host, path)
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
		// 响应对象同样按请求重建：静态层直接往 rr.Res.Headers 写头部，
		// 不清会把上一请求的 Content-Encoding / Content-Range / Set-Cookie
		// 原样带给下一个响应（keep-alive 下表现为「正文与编码对不上」）。
		rr.Res = response.ResponseInit()
		rr.OriginPath = ""
		rr.PathLocation = nil
		rr.ProxyConnInit = false
		rr.CircleData = nil
		ev.Upgrade = ""
	}

	// 读缓冲从池里取：byte_row 可能别名这段缓冲，但 parseRequest
	// 返回前所有需要保留的内容都已拷贝进 Req/Body，归还后无人再读它
	readBuf := core.GetReadBuf()
	defer core.PutReadBuf(readBuf)

	// read data (bytes and str) from socket
	byte_row := ev.ReadRequestInto(readBuf)

	if byte_row == nil { // 首次读到 EOF 或超时：只算连接结束，不计数
		return 0
	}

	// 头可能分多次到达：读完整或判非法为止。
	for {
		parse := rr.Req.ParseHeader(byte_row)
		if parse == request.RequestOk {
			// 头部完整也要检查大小：整段头一次到齐时不会走下面的
			// NeedReadMore 分支，漏掉检查就等于 max_header_size 失效。
			if headerLen(byte_row) > maxHeaderSize() {
				rejectHeaderTooLarge(ev)
				return 0
			}
			break
		}
		if parse != request.RequestNeedReadMore {
			// 畸形请求拒绝处理，不能带着空/半截头走进后续管线
			ev.RR.Res = response.DefaultBadRequest()
			ev.WriteResponseClose(nil)
			metrics.NoteParseError()
			return 0
		}
		if len(byte_row) > maxHeaderSize() {
			rejectHeaderTooLarge(ev)
			return 0
		}
		ev.Conn.SetReadDeadline(time.Now().Add(config.Current().Limit.ReadTimeoutDuration()))
		n, err := ev.Conn.Read(rr.ReqBuf)
		if err != nil || n == 0 {
			metrics.NoteAborted()
			return 0
		}
		byte_row = append(byte_row, rr.ReqBuf[:n]...)
	}

	if !fif.Fif.HttpParseFilter(rr) {
		return -300
	}

	// parse host
	rr.Req.ParseHost(ev.LisInfo)

	// Expect: 100-continue：客户端在收到 100 之前不会发正文，必须在读 body
	// 前先回中间响应。声明的 Content-Length 已超限时直接 413、不发 100，
	// 让客户端在上传前就知道会被拒绝。
	if !handleExpectation(ev) {
		return 0
	}

	if err := rr.Req.ParseBody(byte_row); err != nil {
		logging.ConsoleLog.Debug().Msgf("%s", color.Red(err.Error()))
	}

	if request.HasToken(rr.Req.HeaderFold("Transfer-Encoding"), "chunked") {
		body, err := request.ReadChunked(rr.Req.Body.Bytes(), func() ([]byte, error) {
			ev.Conn.SetReadDeadline(time.Now().Add(config.Current().Limit.BodyTimeoutDuration()))
			n, err := ev.Conn.Read(rr.ReqBuf)
			if err != nil {
				return nil, err
			}
			return append([]byte(nil), rr.ReqBuf[:n]...), nil
		}, maxBodySize())
		if err != nil {
			if errors.Is(err, request.ErrBodyTooLarge) {
				rejectTooLargeBody(ev)
				return 0
			}
			logging.ConsoleLog.Debug().Msgf("%s", color.Red(err.Error()))
			metrics.NoteAborted()
			return 0
		}
		rr.Req.Body.Reset()
		rr.Req.Body.Write(body)
		// 解码后的 body 与客户端原始帧描述（TE: chunked / 旧 Content-Length）
		// 已不一致。转发前按实际长度重写，避免 TE+旧 CL 的 CL.TE 请求走私，
		// 以及 TE+裸 body 被严格上游判为 400。
		rr.Req.FixRequestFraming()
		return 1
	}

	// 请求体可能随首个数据包完整到达（ParseBody 一次就读完了），此时下面的
	// 增量读取循环不会执行——所以超限检查不能只放在循环里，否则「一次性发完
	// 的超大 body」会完全绕过 max_body_size。
	tooLarge := func() bool {
		limit := maxBodySize()
		return limit > 0 && rr.Req.Body.Len() > limit
	}
	// 超过 max_body_size 必须回 413 再关闭，不能静默断连：客户端只看到连接
	// 被重置时，分不清「服务端拒绝」和「网络故障」。
	if tooLarge() {
		rejectTooLargeBody(ev)
		return 0
	}

	// headerOtherData := make([]byte, core.READ_BODY_BUF_LEN)
	for {
		if rr.Req.RequestBodyValid() {
			break
		} else {
			ev.Conn.SetReadDeadline(time.Now().Add(config.Current().Limit.BodyTimeoutDuration()))
			datasize, err := ev.Conn.Read(rr.ReqBuf)
			if err != nil { // read error, like time out
				logging.ConsoleLog.Debug().Msgf("%s", color.Red("read body error"))
				metrics.NoteAborted()
				return 0
			}
			byte_row = append(byte_row, rr.ReqBuf[:datasize]...)
			rr.Req.TryFixBody(rr.ReqBuf[:datasize])
			if tooLarge() {
				rejectTooLargeBody(ev)
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

// headerLen 返回头部段长度（含终止空行）；头还没读完时返回已读字节数。
func headerLen(b []byte) int {
	if i := bytes.Index(b, []byte("\r\n\r\n")); i >= 0 {
		return i + 4
	}
	return len(b)
}

// rejectHeaderTooLarge 回 431：请求头超过 max_header_size。
func rejectHeaderTooLarge(ev *core.Event) {
	logging.ConsoleLog.Debug().Msgf("%s", color.Red("request header too large"))
	logReject("header_too_large", "max_header_size", ev.Conn)
	ev.RR.Res = response.DefaultRequestHeaderFieldsTooLarge()
	ev.WriteResponseClose(nil)
	metrics.NoteParseError()
}

// rejectTooLargeBody 回 413 并关闭连接：请求体超过 max_body_size。
func rejectTooLargeBody(ev *core.Event) {
	logging.ConsoleLog.Debug().Msgf("%s", color.Red("read body too big"))
	logReject("body_too_large", "max_body_size", ev.Conn)
	ev.RR.Res = response.DefaultPayloadTooLarge()
	ev.WriteResponseClose(nil)
	metrics.NoteAborted()
}

// logReject 写一条统一字段的拒绝日志（R-10）。只记客户端 IP，
// 不记原始请求路径，避免把可能含敏感参数的编码带进日志。
func logReject(reason, rule string, conn net.Conn) {
	logging.RejectWarn(reason, rule, logging.RemoteIP(conn)).Msg("request rejected")
}

// handleExpectation 处理 Expect 头。返回 false 表示已经写出拒绝响应并
// 应当结束连接。
//
// HTTP/1.1 支持 100-continue：先回中间响应再读正文。其它期望按
// RFC 7231 §5.1.1 回 417；HTTP/1.0 则按规范忽略整个期望。
func handleExpectation(ev *core.Event) bool {
	r := ev.RR.Req
	values := expectTokens(r)
	if len(values) == 0 {
		return true
	}
	if r.Protocol != "HTTP/1.1" {
		r.DelHeader("Expect")
		return true
	}

	continueOK := false
	for _, v := range values {
		if strings.EqualFold(v, "100-continue") {
			continueOK = true
			break
		}
	}
	if !continueOK {
		ev.RR.Res = response.DefaultExpectationFailed()
		ev.WriteResponseClose(nil)
		metrics.NoteParseError()
		return false
	}

	// 期望已在本机满足：转发给上游时去掉，避免上游再回一个 100，
	// 被代理当成最终响应透传给客户端。
	r.DelHeader("Expect")
	if n := declaredBodyLen(r); n > 0 && maxBodySize() > 0 && n > maxBodySize() {
		rejectTooLargeBody(ev)
		return false
	}
	if !writeContinue(ev) {
		metrics.NoteAborted()
		return false
	}
	return true
}

// expectTokens 把 Expect 头拆成 token 列表；头不存在时返回 nil。
func expectTokens(r *request.Request) []string {
	v := strings.TrimSpace(r.HeaderFold("Expect"))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// declaredBodyLen 返回 Content-Length 声明的长度；chunked 或无长度时返回 -1。
func declaredBodyLen(r *request.Request) int {
	v := strings.TrimSpace(r.GetHeader("Content-Length"))
	if v == "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}

// writeContinue 直接写 1xx 中间响应。它不是最终响应，不经过 Res，也不写访问日志。
func writeContinue(ev *core.Event) bool {
	ev.SetWriteTimeout()
	_, err := ev.Conn.Write([]byte("HTTP/1.1 100 Continue\r\n\r\n"))
	return err == nil
}

func EventWrite(ev *core.Event, _data []byte) error {
	//fmt.Printf("%p", ev)
	ev.SetWriteTimeout()
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
