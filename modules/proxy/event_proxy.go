// Package proxy 把 HTTP/HTTPS 请求转发到配置的上游服务器。
package proxy

import (
	"crypto/tls"
	"errors"
	"fast-https/config"
	"fast-https/core"
	"strconv"
	"time"

	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/response"
	"fast-https/infra/color"
	"fast-https/infra/logging"
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

	p.ProxyConn.SetDeadline(time.Now().Add(time.Second * 20)) // proxy server time out
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

		proxy := Newproxy(cfg.ProxyAddr, int(cfg.Type), cfg.ProxyCache.Key != "")
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
		fresh.ProxyNeedCache = cfg.ProxyCache.Key != ""
		return fresh, nil
	})
	if err != nil {
		return nil, err
	}
	if p.reused {
		// 新的请求周期，刷新整体 20s 截止时间
		p.ProxyConn.SetDeadline(time.Now().Add(time.Second * 20))
		p.ev = ev
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

	temp_res.DelHeader("Server")
	temp_res.SetHeader("Server", "Fast-Https")

	temp_body := rr.Res.Body
	rr.Res = temp_res

	if isKeepalive {
		rr.Res.Body = temp_body
	}

	return head_code
}

func (p *Proxy) readFromUpstreamServer(ev *core.Event) (resData []byte, err error) {
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
		} else if ok {
			logging.ConsoleLog.Debug().Msgf("%s%s", color.Red("unhandled send to upstream server: "), err.Error())
		} else {
			logging.ConsoleLog.Fatal().Msg("convent net error")
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
	logProxyAccess(ev, status, len(ev.RR.Res.Body))

	return status, nil
}

/*  ====  这两个函数可以用 event ===== */
func (p *Proxy) proxyNeedCache(pc *ProxyCache, req_data []byte, ev *core.Event) {
	if status, ok := pc.Serve(ev); ok {
		// 命中缓存：借来的上游连接没被使用，直接归还
		releaseProxy(p)
		logProxyAccess(ev, status, len(ev.RR.Res.Body))
		writeProxyResponse(ev)
		return
	}

	status, err := p.getDataFromServer(ev, req_data)
	if err != nil {
		// 上游连接状态未知，一律关闭不回收
		p.Close()
		ev.RR.Res = response.DefaultServerError()
		ev.WriteResponseClose(nil)
		return
	}
	releaseProxy(p)
	pc.Store(ev, status)
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

func (p *Proxy) proxyNoCache(req_data []byte, ev *core.Event) {

	_, err := p.getDataFromServer(ev, req_data)

	if err != nil {
		// 上游连接状态未知，一律关闭不回收
		p.Close()
		if err == ProxyErrorSendToUpstreamTimeout || err == ProxyErrorReadFromUpstreamTimeout {
			ev.Close()
			ev.Reuse = false
		} else {
			ev.RR.Res = response.DefaultServerError()
			ev.WriteResponseClose(nil)
		}
		return
	}
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
		ev.RR.Res = DialFailureResponse(cfg)
		ev.WriteResponseClose(nil)
		return
	}

	if proxy.ProxyNeedCache {
		proxyCache := NewProxyCache(cfg.ProxyCache.Key, cfg.ProxyCache.Valid,
			cfg.ProxyCache.Path, cfg.ProxyCache.Methods, cfg.ProxyCache.MaxSize)
		proxy.proxyNeedCache(proxyCache, req_data, ev)
	} else {
		proxy.proxyNoCache(req_data, ev)
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
	for _, item := range cfg.ProxySetHeader {
		ev.RR.Req.SetHeader(item.HeaderKey,
			ev.GetCommandParsedStr(item.HeaderValue), cfg)
	}
	// ev.RR.Req_.SetHeader("Host", cfg.Proxy_addr, cfg)
	// ev.RR.Req_.SetHeader("Connection", "close", cfg)
	// ev.RR.Req.SetHeader("Accept-Encoding", "", cfg)
}
