// Package proxy 把 HTTP/HTTPS 请求转发到配置的上游服务器。
package proxy

import (
	"crypto/tls"
	"errors"
	"fast-https/config"
	"fast-https/core"
	"strconv"
	"time"

	"fast-https/infra/logging"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/response"
	"net"
	"strings"
)

type Proxy struct {
	ProxyAddr      string // TODO: upstream
	ProxyConn      net.Conn
	ProxyTlsConn   *tls.Conn
	ProxyType      int
	ProxyNeedCache bool
	ProxyNeedClose bool
	Write          func([]byte) (int, error)
	Read           func([]byte) (int, error)
	Close          func() error
	ev             *core.Event
	// reused 标记连接来自空闲池：上游可能早已悄悄关闭，首次发送
	// 失败时换新连接重试一次
	reused bool
	// closed 在 Close 后置位，防止已关闭连接被归还进池
	closed bool
	// respEmptyBody 标记刚读完的响应在协议上必然没有 body（HEAD / 204 / 304）。
	// 只有这类响应可以在没读到连接关闭时就把连接归还进池。
	respEmptyBody bool
}

// Register installs the reverse-proxy handlers for HTTP and HTTPS upstreams.
func Register() {
	core.RegisterHandler(config.PROXY_HTTP, ProxyFilterHandler, ProxyEvent, nil)
	core.RegisterHandler(config.PROXY_HTTPS, ProxyFilterHandler, ProxyEvent, nil)
}

func Newproxy(addr string, proxyType int, proxyNeedCache bool) *Proxy {
	return &Proxy{
		ProxyType:      proxyType,
		ProxyNeedCache: proxyNeedCache,
		ProxyAddr:      addr,
	}
}

var (
	ProxyErrorDialUpstream            = &ProxyError{msg: "proxy dial upstream server"}
	ProxyErrorSendToUpstreamTimeout   = &ProxyError{msg: "proxy send to upstream server time out"}
	ProxyErrorReadFromUpstreamTimeout = &ProxyError{msg: "proxy read from upstream server time out"}
	// ProxyErrorUpstreamBodyTooLarge 表示上游既没给 Content-Length 也没给
	// chunked，读到 EOF 时超过 max_body_size。按 502 回给客户端。
	ProxyErrorUpstreamBodyTooLarge = &ProxyError{msg: "proxy upstream response body too large"}
)

type ProxyError struct {
	msg string
}

func (m *ProxyError) Error() string {
	return m.msg
}

// dialUpstream 建立到上游的新连接并完成读写闭包装配。
// addr 必须已带端口（normalizeUpstreamAddr 负责补默认端口）。
func dialUpstream(addr string, proxyType int) (*Proxy, error) {
	p := &Proxy{ProxyAddr: addr, ProxyType: proxyType}

	conn, err := net.Dial("tcp", p.ProxyAddr)
	p.Read = p.read
	p.Write = p.write
	p.Close = p.close
	p.ProxyNeedClose = false

	if err != nil {
		logging.ConsoleLog.Debug().Msgf("proxy event: can't connect to upstream server %s", err.Error())
		noteDialFail()
		return nil, ProxyErrorDialUpstream // no server
	}
	p.ProxyConn = conn

	if p.ProxyType == config.PROXY_HTTPS {
		tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
		p.ProxyTlsConn = tlsConn

		p.Read = p.readSSL
		p.Write = p.writeSSL
		p.Close = p.closeSSL
	}

	p.ProxyConn.SetDeadline(time.Now().Add(config.Current().Proxy.UpstreamTimeoutDuration())) // proxy server time out
	return p, nil
}

// normalizeUpstreamAddr 给不带端口的上游地址补默认端口，同时作为
// 连接池的分组键。
func normalizeUpstreamAddr(addr string, proxyType int) string {
	if strings.Contains(addr, ":") {
		return addr
	}
	if proxyType == int(config.PROXY_HTTPS) {
		return addr + ":443"
	}
	if proxyType == int(config.PROXY_HTTP) {
		return addr + ":80"
	}
	logging.PrintErr("--proxy cant not set addr")
	return addr
}

// connect to the server
// init read write close handler
func (p *Proxy) ProxyInit(ev *core.Event) error {
	fresh, err := dialUpstream(normalizeUpstreamAddr(p.ProxyAddr, p.ProxyType), p.ProxyType)
	if err != nil {
		return err
	}
	p.ProxyAddr = fresh.ProxyAddr
	p.ProxyConn = fresh.ProxyConn
	p.ProxyTlsConn = fresh.ProxyTlsConn
	p.Read, p.Write, p.Close = fresh.Read, fresh.Write, fresh.Close
	p.ProxyNeedClose = false
	p.ev = ev
	return nil
}

// getProxy 借一条到上游的连接。
//
// websocket 升级后连接被当作双向管道整条占用，必须在一个会话里
// 保持同一上游连接，因此由 CircleData 持有而不进池；普通 HTTP
// 请求每请求从池里借、响应后归还，跨客户端连接复用。
func getProxy(ev *core.Event, rr *core.RRcircle, cfg *listener.ListenCfg) (*Proxy, error) {
	if ev.Upgrade == "websocket" {
		if rr.ProxyConnInit {
			proxy, ok := rr.CircleData.(*Proxy)
			if !ok || proxy == nil {
				logging.ConsoleLog.Error().Msg("proxy can not convert circle data to *Proxy")
				return nil, ProxyErrorDialUpstream
			}
			return proxy, nil
		}

		proxy := Newproxy(cfg.ProxyAddr, int(cfg.Type), cfg.ProxyCache != "")
		if err := proxy.ProxyInit(ev); err != nil { // only one type error: `ProxyErrorDialUpstream`
			return nil, err
		}
		rr.ProxyConnInit = true
		rr.CircleData = proxy
		return proxy, nil
	}

	addr := normalizeUpstreamAddr(cfg.ProxyAddr, int(cfg.Type))
	p, err := upstreamPool().get(addr, func() (*Proxy, error) {
		fresh, derr := dialUpstream(addr, int(cfg.Type))
		if derr != nil {
			return nil, derr
		}
		fresh.ev = ev
		fresh.ProxyNeedCache = cfg.ProxyCache != ""
		return fresh, nil
	})
	if err != nil {
		return nil, err
	}
	if p.reused {
		// 新的请求周期，刷新上游整体截止时间
		p.ProxyConn.SetDeadline(time.Now().Add(config.Current().Proxy.UpstreamTimeoutDuration()))
		p.ev = ev
		p.respEmptyBody = false
	}
	return p, nil
}

// releaseProxy 在一个请求周期结束后回收上游连接：
// websocket 会话由 CircleData 持有不进池；上游要求关闭或已出错
// 关闭的连接直接丢弃；健康连接清除整体 deadline 后归还。
func releaseProxy(p *Proxy) {
	if p == nil || p.isClosed() {
		return
	}
	if p.ev != nil && p.ev.Upgrade == "websocket" {
		return
	}
	if p.ProxyNeedClose {
		p.Close()
		return
	}
	p.ProxyConn.SetDeadline(time.Time{})
	upstreamPool().put(p.ProxyAddr, p)
}

func (p *Proxy) ChangeHeader(isKeepalive bool, tmpByte []byte, rr *core.RRcircle) string {

	temp_res := response.ResponseInit()
	temp_res.HttpResParse(string(tmpByte))

	var head_code string

	firstLineDec := strings.Split(temp_res.FirstLine, " ")
	if len(firstLineDec) < 2 {
		logging.ConsoleLog.Error().Msgf("%s", string(tmpByte))
		rr.Res = response.DefaultServerHeaderError()
		return "509"
	} else {
		head_code = firstLineDec[1]
	}

	if temp_res.Headers["Connection"] == "" {
		p.ProxyNeedClose = true
	}
	if temp_res.Headers["Connection"] == "close" {
		p.ProxyNeedClose = true
	}
	// 空 body 响应（HEAD / 204 / 304）的响应边界已经到齐，没有未读 body
	// 需要等上游关闭。HTTP/1.1 上游未显式 close 时按持久连接复用；
	// HTTP/1.0 则要求显式 keep-alive。
	if p.respEmptyBody && !upstreamCloseRequested(firstLineDec, temp_res.GetHeader("Connection")) {
		p.ProxyNeedClose = false
	}

	temp_res.DelHeader("Server")
	temp_res.SetHeader("Server", "Fast-Https")

	temp_body := rr.Res.Body
	rr.Res = temp_res

	if isKeepalive {
		rr.Res.Body = temp_body
	}

	return head_code
}

// upstreamCloseRequested 报告上游是否会在本次响应后关闭连接。
// HTTP/1.0 默认不持久，只有显式 keep-alive 才认为可复用。
func upstreamCloseRequested(firstLine []string, connection string) bool {
	if strings.EqualFold(connection, "close") {
		return true
	}
	if len(firstLine) > 0 && firstLine[0] == "HTTP/1.0" && !strings.EqualFold(connection, "keep-alive") {
		return true
	}
	return false
}

func (p *Proxy) readFromUpstreamServer(ev *core.Event) (resData []byte, err error) {
	p.respEmptyBody = false
	if !ev.RR.Req.IsKeepalive() {
		if ev.Upgrade == "websocket" {
			logging.ConsoleLog.Debug().Msg("-----This is Read websocket")
			web := make([]byte, 1024)
			var n int
			n, err = p.Read(web)
			if err != nil {
				logging.ConsoleLog.Debug().Msgf("proxy event: can't read websocket %v", err.Error())
				err = errors.New("proxy read websocket")
				return
			}
			resData = web[:n]
		} else {
			rcc := ReadConnectionClose{
				p: p,
			}
			logging.ConsoleLog.Debug().Msg("-----This is ReadConnectionClose")
			resData, err = rcc.proxyReadAll()
			if err != nil {
				if err != ProxyErrorReadFromUpstreamTimeout {
					logging.ConsoleLog.Debug().Msgf("can't read connection close %v", err.Error())
				}
				return
			}

		}
	} else {
		rka := ReadKeepAlive{
			TryNum: 0,
			p:      p,
		}
		logging.ConsoleLog.Debug().Msg("-----This is ReadKeepAlive")
		err = rka.proxyKeepAlive()
		if err != nil {
			if err != ProxyErrorReadFromUpstreamTimeout {
				logging.ConsoleLog.Debug().Msgf("can't read keep alive %v", err.Error())
			}
			return
		}
		resData = rka.finalStr
		// if len(rka.finalStr) < 8192 {
		// 	ev.DEBUG_BUFFER = append(ev.DEBUG_BUFFER, rka.finalStr...)
		// } else {
		// 	ev.DEBUG_BUFFER = append(ev.DEBUG_BUFFER, rka.finalStr[:8192]...)
		// }

		ev.RR.Res.Body = rka.body
		// 空 body 响应后还有多余字节：上游违约（例如 204 带了正文）。
		// 响应本身可以交付，但这条连接不能再复用，否则残留字节会污染
		// 下一条借到它的请求。
		if rka.trailingBytes {
			p.ProxyNeedClose = true
		}
		p.respEmptyBody = rka.emptyBody && !rka.trailingBytes
		err = nil
	}
	return
}

func (p *Proxy) sendToUpstreamServer(req_data []byte) error {
	// ev.DEBUG_BUFFER = append(ev.DEBUG_BUFFER, req_data...)
	n, err := p.Write(req_data)
	if err != nil {
		neterr, ok := err.(net.Error)
		if ok && neterr.Timeout() {
			logging.ConsoleLog.Debug().Msg("send to upstream server time out")
			err = ProxyErrorSendToUpstreamTimeout
			noteSendTimeout()
		} else {
			// 非 net.Error 的写错误（io.ErrClosedPipe、TLS 包装错误等）
			// 也必须按普通错误处理：此前这里走 Fatal（os.Exit），一条
			// 上游连接的异常写就能打死整个进程。
			logging.ConsoleLog.Debug().Msgf("send to upstream server error: %s", err.Error())
		}

		p.Close()  // close proxy connection
		return err // can't write
	}
	if len(req_data) != n {
		logging.ConsoleLog.Debug().Msg("send to upstream server part")
	}

	return nil
}

// fast-https will send data to real server and get response from target.
// 返回上游状态码，调用方据此决定是否缓存。
func (p *Proxy) getDataFromServer(ev *core.Event, req_data []byte) (int, error) {

	//logging.ConsoleLog.Debug().Msgf("\n\n" + string(req_data) + "\n\n")
	if err := p.sendToUpstreamServer(req_data); err != nil {
		// 复用的空闲连接可能已被上游悄悄关闭：换新连接重试一次
		if !p.reused || err == ProxyErrorSendToUpstreamTimeout {
			return 0, err
		}
		p.Close()
		fresh, derr := dialUpstream(p.ProxyAddr, p.ProxyType)
		if derr != nil {
			return 0, derr
		}
		fresh.ev = p.ev
		fresh.ProxyNeedCache = p.ProxyNeedCache
		*p = *fresh
		if err := p.sendToUpstreamServer(req_data); err != nil {
			return 0, err
		}
	}

	resData, err := p.readFromUpstreamServer(ev)
	if err != nil {
		if err != ProxyErrorReadFromUpstreamTimeout {
			logging.ConsoleLog.Debug().Msg("unhandled read form upstream server error")
		}
		// logging.ConsoleLog.Debug().Msgf("	send\n%s\nto server%s", string(ev.DEBUG_BUFFER), ev.Conn.RemoteAddr().String())
		return 0, err
	}

	if !ev.RR.Req.IsKeepalive() && ev.Upgrade == "" { // connection close
		p.Close()
	}

	headCode := p.ChangeHeader(ev.RR.Req.IsKeepalive(), resData, &ev.RR)
	status, _ := strconv.Atoi(headCode)
	return status, nil
}

/*  ====  这两个函数可以用 event ===== */
func (p *Proxy) proxyNeedCache(cfg *listener.ListenCfg, pc *ProxyCache, req_data []byte, ev *core.Event) {
	if status, ok := pc.Serve(ev); ok {
		// 命中缓存：借来的上游连接没被使用，直接归还
		releaseProxy(p)
		compressResponse(cfg, ev)
		logProxyAccess(ev, status, len(ev.RR.Res.Body))
		writeProxyResponse(ev)
		return
	}

	status, err := p.getDataFromServer(ev, req_data)
	if err != nil {
		// 上游连接状态未知，一律关闭不回收
		p.Close()
		ev.RR.Res = proxyErrorResponse(err)
		ev.WriteResponseClose(nil)
		return
	}
	releaseProxy(p)
	// 缓存先存未压缩正文：后续命中要按各自请求的 Accept-Encoding 重新协商。
	pc.Store(ev, status)
	compressResponse(cfg, ev)
	logProxyAccess(ev, status, len(ev.RR.Res.Body))
	writeProxyResponse(ev)
}

// logProxyAccess 按响应状态与长度写一条访问日志。
func logProxyAccess(ev *core.Event, status, size int) {
	log := dynlog.DynLogger{}
	log.Status(status)
	log.Size(size)
	dynlog.Log(&log, ev)
}

// writeProxyResponse 写出响应，并按连接可复用性决定是否关闭。
func writeProxyResponse(ev *core.Event) {
	if ev.RR.Req.IsKeepalive() {
		ev.WriteResponse(nil)
		ev.Reuse = true
		return
	}
	ev.WriteResponseClose(nil)
}

// proxyErrorResponse 把上游读取错误映射成对客户端的响应。
// 上游响应超出限额属于网关侧问题，用 502；其余沿用 500。
func proxyErrorResponse(err error) *response.Response {
	if err == ProxyErrorUpstreamBodyTooLarge {
		return response.DefaultBadGateway()
	}
	return response.DefaultServerError()
}

func (p *Proxy) proxyNoCache(cfg *listener.ListenCfg, req_data []byte, ev *core.Event) {

	status, err := p.getDataFromServer(ev, req_data)

	if err != nil {
		// 上游连接状态未知，一律关闭不回收
		p.Close()
		if err == ProxyErrorSendToUpstreamTimeout || err == ProxyErrorReadFromUpstreamTimeout {
			ev.Close()
			ev.Reuse = false
		} else {
			ev.RR.Res = proxyErrorResponse(err)
			ev.WriteResponseClose(nil)
		}
		return
	}
	compressResponse(cfg, ev)
	logProxyAccess(ev, status, len(ev.RR.Res.Body))
	// proxy server return valid data
	if ev.RR.Req.IsKeepalive() && !p.ProxyNeedClose {
		ev.WriteResponse(nil)
		// events.Handle_event(ev)
		ev.Reuse = true
	} else if ev.Upgrade == "websocket" {
		ev.WriteResponse(nil)
		// events.Handle_event(ev)
		ev.Reuse = true
	} else {
		ev.WriteResponseClose(nil)
	}
	// websocket 会话由 CircleData 持有这条连接，其余归还进池
	if ev.Upgrade != "websocket" {
		releaseProxy(p)
	}
}

/*
 *************************************
 ****** Interfaces are as follows ****
 *************************************
 */
func ProxyEvent(cfg *listener.ListenCfg, ev *core.Event) {
	req_data := ev.RR.Req.ByteRow()

	// if len(req_data) > 8192 {
	// 	fmt.Println(string(req_data[:8192]))

	// } else {
	// 	fmt.Println(string(req_data[:]))
	// }
	proxy, err := getProxy(ev, &ev.RR, cfg)
	if err != nil { // one type error `ProxyErrorDialUpstream`
		// 这条 500 之前完全静默：上游没起来、DNS 不通、连接池耗尽时只能靠猜。
		upstream, path := "", ""
		if cfg != nil {
			upstream = cfg.ProxyAddr
		}
		if ev.RR.Req != nil {
			path = ev.RR.Req.Path
		}
		logging.ConsoleLog.Error().Msgf("proxy: cannot dial upstream %q (path=%s): %v",
			upstream, path, err)
		ev.RR.Res = DialFailureResponse(cfg)
		ev.WriteResponseClose(nil)
		return
	}

	if proxy.ProxyNeedCache {
		// 站点只存缓存区名，目录与有效期来自主配置；配置校验保证名字存在，
		// 这里取不到时用零值区，CacheKey 返回空串即退化为不缓存。
		zone, _ := config.Current().CacheZoneByName(cfg.ProxyCache)
		proxyCache := NewProxyCache(zone)
		proxy.proxyNeedCache(cfg, proxyCache, req_data, ev)
	} else {
		proxy.proxyNoCache(cfg, req_data, ev)
	}
}

func DialFailureResponse(cfg *listener.ListenCfg) *response.Response {
	res := response.DefaultServerError()
	if cfg != nil {
		response.UseFile(res, cfg.ErrorProxy)
	}
	return res
}

func ProxyFilterHandler(cfg *listener.ListenCfg, ev *core.Event) bool {
	ChangeHead(cfg, ev)
	// 应用防火墙检查已收敛到 events.EventHandler 统一执行
	return true
}

func ChangeHead(cfg *listener.ListenCfg, ev *core.Event) {
	// 先按全局 proxy 策略写入 X-Forwarded-For；下面 proxy_set_header 里的
	// 显式配置仍然可以覆盖它（与 nginx 的「后写生效」一致）。
	if ev != nil && ev.RR.Req != nil {
		// 先记下客户端连接是否复用：hop-by-hop 头剥掉后就读不到 Connection 了。
		keepalive := ev.RR.Req.IsKeepalive()
		if xff := core.ForwardedForChain(cfg, ev); xff != "" {
			ev.RR.Req.SetHeader("X-Forwarded-For", xff, cfg)
		}
		// preserve_host 默认 true 原样透传；显式关闭时用上游地址，
		// 让按 Host 分虚拟主机的上游也能命中正确站点。
		if cfg != nil && !cfg.PreserveHost && strings.TrimSpace(cfg.ProxyAddr) != "" {
			ev.RR.Req.SetHeader("Host", upstreamHost(cfg.ProxyAddr), cfg)
		}
		stripHopByHop(ev.RR.Req, ev.Upgrade, cfg)
		// 剥掉 Connection 后要按客户端语义重建上游这一跳的连接头：
		// 客户端要求关闭时不补会让上游保持 keep-alive，代理按「读到连接
		// 结束」读取响应会等到超时；HTTP/1.0 的 keep-alive 则相反，默认
		// 是短连接，必须显式补回。websocket 已由 stripHopByHop 处理。
		if ev.Upgrade != "websocket" {
			switch {
			case !keepalive:
				ev.RR.Req.SetHeader("Connection", "close", cfg)
			case ev.RR.Req.Protocol == "HTTP/1.0":
				// HTTP/1.0 默认短连接，剥离后必须把 keep-alive 显式补回去。
				ev.RR.Req.SetHeader("Connection", "keep-alive", cfg)
			}
		}
	}
	for _, item := range cfg.ProxySetHeader {
		ev.RR.Req.SetHeader(item.HeaderKey,
			ev.GetCommandParsedStr(item.HeaderValue), cfg)
	}
	// ev.RR.Req_.SetHeader("Host", cfg.Proxy_addr, cfg)
	// ev.RR.Req_.SetHeader("Connection", "close", cfg)
	// ev.RR.Req.SetHeader("Accept-Encoding", "", cfg)
}

// upstreamHost 从 proxy_pass 地址取 host:port，用于 preserve_host=false。
// 接受带不带 scheme、带路径或 userinfo 的写法。
func upstreamHost(addr string) string {
	addr = strings.TrimSpace(addr)
	if i := strings.Index(addr, "://"); i >= 0 {
		addr = addr[i+3:]
	}
	if i := strings.IndexAny(addr, "/?#"); i >= 0 {
		addr = addr[:i]
	}
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		addr = addr[i+1:]
	}
	return addr
}
