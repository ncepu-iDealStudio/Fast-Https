// Package proxy 把 HTTP/HTTPS 请求转发到配置的上游服务器。
package proxy

import (
	"crypto/tls"
	"errors"
	"fast-https/config"
	"fast-https/modules/appfirewall"
	"fast-https/core"
	"fmt"
	"strconv"
	"time"

	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/response"
	"fast-https/infra/logging"
	"net"
	"strings"

	"github.com/fatih/color"
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

// connect to the server
// init read write close handler
func (p *Proxy) ProxyInit(ev *core.Event) error {
	var err error

	p.ProxyConn, err = net.Dial("tcp", p.ProxyAddr)

	p.Read = p.read
	p.Write = p.write
	p.Close = p.close
	p.ev = ev

	if p.ProxyType == config.PROXY_HTTPS {
		config := tls.Config{InsecureSkipVerify: true}
		tlsConn := tls.Client(p.ProxyConn, &config)
		p.ProxyTlsConn = tlsConn

		p.Read = p.readSSL
		p.Write = p.writeSSL
		p.Close = p.closeSSL
	}
	p.ProxyNeedClose = false // keep-alive default

	if err != nil {
		logging.ConsoleLog.Debug().Msgf("proxy event: can't connect to upstream server %s", err.Error())
		return ProxyErrorDialUpstream // no server
	}
	now := time.Now()
	p.ProxyConn.SetDeadline(now.Add(time.Second * 20)) // proxy server time out

	return nil
}

// get a inited proxy
func getProxy(ev *core.Event, rr *core.RRcircle, cfg *listener.ListenCfg) (*Proxy, error) {
	var proxy *Proxy

	// init proxy tcp connection
	if !rr.ProxyConnInit {
		rr.ProxyConnInit = true

		configCache := true
		if cfg.ProxyCache.Key == "" {
			configCache = false
		}

		proxy = Newproxy(cfg.ProxyAddr, int(cfg.Type), configCache)
		proxy.proxyHandleAddr()
		err := proxy.ProxyInit(ev)
		if err != nil { // only one type error: `ProxyErrorDialUpstream`
			return nil, err
		}

		rr.CircleData = proxy
	} else {
		var flag bool
		proxy, flag = (rr.CircleData).(*Proxy)
		if !flag {
			logging.ConsoleLog.Fatal().Msg("proxy can not convert circle data to *Proxy")
		}
	}

	return proxy, nil
}

// TODO: when add upstream, this function need to do more
func (p *Proxy) proxyHandleAddr() {
	if !strings.Contains(p.ProxyAddr, ":") {
		if p.ProxyType == config.PROXY_HTTP {
			p.ProxyAddr = p.ProxyAddr + ":80"
		} else if p.ProxyType == config.PROXY_HTTPS {
			p.ProxyAddr = p.ProxyAddr + ":443"
		} else {
			logging.PrintErr("--proxy cant not set addr")
		}
	}
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
			logging.ConsoleLog.Debug().Msgf("%s%s", color.RedString("unhandled send to upstream server: "), err.Error())
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
		if err != ProxyErrorSendToUpstreamTimeout {
			logging.ConsoleLog.Debug().Msg("unhandled send to upstream server errror")
		}
		return 0, err
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
		// 命中缓存：直接回放缓存里的响应，不回源。
		logProxyAccess(ev, status, len(ev.RR.Res.Body))
		writeProxyResponse(ev)
		return
	}

	status, err := p.getDataFromServer(ev, req_data)
	if err != nil {
		ev.RR.Res = response.DefaultServerError()
		ev.WriteResponseClose(nil)
		return
	}
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
		fmt.Println("-------------------- websocket -------------")
		ev.WriteResponse(nil)
		// events.Handle_event(ev)
		ev.Reuse = true
	} else {
		ev.WriteResponseClose(nil)
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
			cfg.ProxyCache.Path, cfg.ProxyCache.MaxSize)
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
	if !appfirewall.HandleAppFireWall(cfg, ev.RR.Req) {
		ev.RR.Res = response.DefaultForbidden()
		ev.WriteResponseClose(nil)
		return false
	}
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
