package events

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"

	"fast-https/core"
	"fast-https/infra/logging"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
	"fast-https/runtime/metrics"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

// H2GracefulShutdownPeriod is the maximum time to wait for in-flight HTTP/2
// streams to complete during a reload before force-closing the connection.
const H2GracefulShutdownPeriod = 5 * time.Second

const (
	h2MaxConcurrentStreams = 250
	h2ReadHeaderTimeout    = 30 * time.Second
)

// H2HandleEvent 接管一条已完成 TCP 接入的 HTTP/2 连接。
//
// 协议栈（分帧、HPACK、流状态机、流量控制、CONTINUATION、PING/GOAWAY）
// 全部由 golang.org/x/net/http2 负责；本函数只保留两件事：
//  1. 用一个只吐一条连接的 net.Listener 把连接喂给 http.Server，
//     这样 TLS 握手与 ALPN 协商由标准库完成，Shutdown 也能发出 GOAWAY；
//  2. 把请求转交给既有的 core.Event / filters / 模块管线。
func H2HandleEvent(l *listener.Listener, conn net.Conn, ctx context.Context) {
	connEv := core.NewEvent(l, conn)
	h := &h2Handler{
		lis:  l,
		conn: conn,
		fif:  filters.NewFilter(), // Filter interface
	}
	if !h.fif.Fif.ConnFilter(connEv) {
		metrics.NoteRejected()
		_ = conn.Close()
		return
	}

	ln := newSingleConnListener(conn)
	headerLimit := maxHeaderSize()
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: h2ReadHeaderTimeout,
		MaxHeaderBytes:    headerLimit,
		ErrorLog:          log.New(h2LogWriter{}, "", 0),
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ConnState: func(_ net.Conn, state http.ConnState) {
			// 单连接监听器在连接结束后必须放行 Accept，
			// 否则 Serve 会一直阻塞在 Accept 上导致协程泄漏。
			if state == http.StateClosed || state == http.StateHijacked {
				_ = ln.Close()
			}
		},
	}
	http2.ConfigureServer(srv, &http2.Server{
		MaxConcurrentStreams: h2MaxConcurrentStreams,
	})

	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			logging.ConsoleLog.Debug().Msg("H2 connection shutting down (reload/stop), starting graceful close")
			sctx, cancel := context.WithTimeout(context.Background(), H2GracefulShutdownPeriod)
			defer cancel()
			_ = srv.Shutdown(sctx)
		case <-stopped:
		}
	}()

	err := srv.Serve(ln)
	close(stopped)
	if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		logging.ConsoleLog.Debug().Msgf("HTTP/2 connection closed: %v", err)
	}
	_ = srv.Close()
}

// h2Handler 把 http.Request 翻译成既有的 core.Event，并把模块写出的
// response.Response 序列化回 http.ResponseWriter。
type h2Handler struct {
	lis  *listener.Listener
	conn net.Conn
	fif  *filters.Filter
}

// ServeHTTP 把一条 HTTP/2 流转成 Event 交给 EventHandler。
// 流级错误（RST_STREAM 等）目前还分不清，不单独计数。
func (h *h2Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ev := core.NewEvent(h.lis, h.conn)
	ev.RR.Req = request.RequestInit(true) // h2 = true
	ev.RR.Res = response.ResponseInit()
	ev.RR.ReqBuf = make([]byte, core.READ_BODY_BUF_LEN)

	// 头上限：协议栈按 MaxHeaderBytes 设置的列表上限只是 DoS 兜底，
	// 真正的 431 在这里按同一份配置判定，保证 h2 与 h1 的状态码一致。
	if h2HeaderSize(r) > maxHeaderSize() {
		w.WriteHeader(http.StatusRequestHeaderFieldsTooLarge)
		metrics.NoteParseError()
		return
	}
	if !h2ExpectationOK(w, r) {
		return
	}
	// 声明的长度已超限：不读正文，直接 413（与 h1 的 Expect 前置检查一致）。
	if limit := maxBodySize(); limit > 0 && r.ContentLength > int64(limit) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		metrics.NoteAborted()
		return
	}

	switch fillRequest(ev.RR.Req, r) {
	case 0:
	case http.StatusRequestEntityTooLarge:
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		metrics.NoteAborted()
		return
	default:
		w.WriteHeader(http.StatusBadRequest)
		metrics.NoteParseError()
		return
	}
	// 期望已在本机满足（协议栈在首次读 body 时自动发 100），
	// 转发给上游前去掉，避免上游再回一个 100。
	ev.RR.Req.DelHeader("Expect")

	out := &h2ResponseWriter{w: w, req: r}
	ev.EventWrite = func(ev *core.Event, _ []byte) error { return out.write(ev) }

	EventHandler(ev, h.fif)

	// 有模块（如 websocket 升级）可能不写响应，h2 下不能挂住客户端。
	if !out.written {
		if err := out.write(ev); err != nil {
			logging.ConsoleLog.Debug().Msgf("h2 write response: %v", err)
		}
	}
}

// h2ExpectationOK 处理 HTTP/2 的 Expect 头：100-continue 由协议栈在
// 首次读 body 时自动回 100，这里只负责拒绝其它无法满足的期望。
func h2ExpectationOK(w http.ResponseWriter, r *http.Request) bool {
	expect := strings.TrimSpace(r.Header.Get("Expect"))
	if expect == "" {
		return true
	}
	for _, v := range strings.Split(expect, ",") {
		if strings.EqualFold(strings.TrimSpace(v), "100-continue") {
			return true
		}
	}
	w.WriteHeader(http.StatusExpectationFailed)
	return false
}

// h2HeaderSize 按 RFC 7540 §6.5.2 估算头部列表大小：每个字段额外计
// 32 字节开销，伪首部按方法、Host、路径与固定开销计入。
func h2HeaderSize(r *http.Request) int {
	size := 32*5 + len(r.Method) + len(r.Host) + len(r.URL.RequestURI()) + len("https")
	for k, vs := range r.Header {
		for _, v := range vs {
			size += len(k) + len(v) + 32
		}
	}
	return size
}

// fillRequest 将 net/http 的请求映射为 runtime/request.Request。
// 返回 0 表示成功；400 表示请求目标不合法；413 表示正文超过 max_body_size。
func fillRequest(req *request.Request, r *http.Request) int {
	path, ok := request.NormalizePath(r.URL.RequestURI())
	if !ok {
		return http.StatusBadRequest
	}
	req.Method = r.Method
	req.Path = path
	req.Protocol = "HTTP/2"
	req.Headers["Host"] = r.Host

	for k, vs := range r.Header {
		if len(vs) > 0 {
			req.Headers[k] = vs[0]
		}
	}
	for k, vs := range r.URL.Query() {
		if len(vs) > 0 {
			req.Query[k] = vs[0]
		}
	}

	if r.Body == nil {
		return 0
	}
	limit := maxBodySize()
	var reader io.Reader = r.Body
	if limit > 0 {
		reader = io.LimitReader(r.Body, int64(limit)+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		logging.ConsoleLog.Debug().Msgf("h2 read body: %v", err)
	}
	_ = r.Body.Close()
	if limit > 0 && len(body) > limit {
		return http.StatusRequestEntityTooLarge
	}
	req.Body.Write(body)
	req.BodyLen = len(body)
	return 0
}

// h2ResponseWriter 把 response.Response 写出为 HTTP/2 响应。
type h2ResponseWriter struct {
	w       http.ResponseWriter
	req     *http.Request
	written bool
}

func (s *h2ResponseWriter) write(ev *core.Event) error {
	if s.written {
		return nil
	}
	s.written = true

	res := ev.RR.Res
	if res == nil {
		return nil
	}

	for k, v := range res.Headers {
		if isHopByHopHeader(k) {
			continue
		}
		// HEAD 保留模块给出的 Content-Length；其余交给标准库按实际写出的字节数计算。
		if strings.EqualFold(k, "Content-Length") && s.req.Method != http.MethodHead {
			continue
		}
		s.w.Header().Set(k, v)
	}

	// HTTP/2 流级错误（RST_STREAM 等）目前还分不清，不单独计数，留给后续步骤。
	s.w.WriteHeader(statusFromFirstLine(res.FirstLine))

	if s.req.Method == http.MethodHead || len(res.Body) == 0 {
		return nil
	}
	_, err := s.w.Write(res.Body)
	return err
}

// statusFromFirstLine 从 "HTTP/1.1 200 OK" 中取出状态码。
func statusFromFirstLine(firstLine string) int {
	fields := strings.Fields(firstLine)
	if len(fields) < 2 {
		return http.StatusOK
	}
	code, err := strconv.Atoi(fields[1])
	if err != nil || code < 100 || code > 999 {
		return http.StatusOK
	}
	return code
}

// isHopByHopHeader 列出 HTTP/2 禁止出现的逐跳首部。
func isHopByHopHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Connection", "Keep-Alive", "Proxy-Connection", "Transfer-Encoding", "Upgrade", "Te", "Trailer":
		return true
	}
	return false
}

// singleConnListener 把一条已经 accept 的连接交给 http.Server。
type singleConnListener struct {
	addr     net.Addr
	conn     net.Conn
	accepted bool
	once     sync.Once
	done     chan struct{}
}

func newSingleConnListener(conn net.Conn) *singleConnListener {
	return &singleConnListener{addr: conn.LocalAddr(), conn: conn, done: make(chan struct{})}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if !l.accepted {
		l.accepted = true
		return l.conn, nil
	}
	<-l.done
	return nil, net.ErrClosed
}

func (l *singleConnListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *singleConnListener) Addr() net.Addr { return l.addr }

// h2LogWriter 把标准库内部的日志（握手失败、handler panic 等）转到项目日志。
type h2LogWriter struct{}

func (h2LogWriter) Write(p []byte) (int, error) {
	logging.ConsoleLog.Debug().Msgf("h2: %s", strings.TrimSpace(string(p)))
	return len(p), nil
}
