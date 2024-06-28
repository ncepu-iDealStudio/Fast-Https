package proxy

import (
	"crypto/tls"
	"errors"
	"fast-https/config"
	"fast-https/modules/appfirewall"
	"fast-https/modules/cache"
	"fast-https/modules/core"
	"fmt"
	"strconv"
	"time"

	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"fast-https/utils/logger"
	"fast-https/utils/message"
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

func init() {
	core.RRHandlerRegister(config.PROXY_HTTP, ProxyFilterHandler, ProxyEvent, nil)
	core.RRHandlerRegister(config.PROXY_HTTPS, ProxyFilterHandler, ProxyEvent, nil)
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
		logger.Debug("proxy event: can't connect to upstream server" + err.Error())
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
			logger.Fatal("proxy can not convert circle data to *Proxy")
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
			message.PrintErr("--proxy cant not set addr")
		}
	}
}

func (p *Proxy) ChangeHeader(isKeepalive bool, tmpByte []byte, rr *core.RRcircle) string {

	temp_res := response.ResponseInit()
	temp_res.HttpResParse(string(tmpByte))

	var head_code string

	firstLineDec := strings.Split(temp_res.FirstLine, " ")
	if len(firstLineDec) < 2 {
		logger.Error(string(tmpByte))
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
			logger.Debug("-----This is Read websocket")
			web := make([]byte, 1024)
			var n int
			n, err = p.Read(web)
			if err != nil {
				logger.Debug("proxy event: can't read websocket %v", err.Error())
				err = errors.New("proxy read websocket")
				return
			}
			resData = web[:n]
		} else {
			rcc := ReadConnectionClose{
				p: p,
			}
			logger.Debug("-----This is ReadConnectionClose")
			resData, err = rcc.proxyReadAll()
			if err != nil {
				if err != ProxyErrorReadFromUpstreamTimeout {
					logger.Debug("can't read connection close %v", err.Error())
				}
				return
			}

		}
	} else {
		rka := ReadKeepAlive{
			TryNum: 0,
			p:      p,
		}
		logger.Debug("-----This is ReadKeepAlive")
		err = rka.proxyKeepAlive()
		if err != nil {
			if err != ProxyErrorReadFromUpstreamTimeout {
				logger.Debug("can't read keep alive %v", err.Error())
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
			logger.Debug("send to upstream server time out")
			err = ProxyErrorSendToUpstreamTimeout
		} else if ok {
			logger.Debug(color.RedString("unhandled send to upstream server: ") + err.Error())
		} else {
			logger.Fatal("convent net error")
		}

		p.Close()  // close proxy connection
		return err // can't write
	}
	if len(req_data) != n {
		logger.Debug("send to upstream server part")
	}

	return nil
}

// fast-https will send data to real server and get response from target
func (p *Proxy) getDataFromServer(ev *core.Event, req_data []byte) error {

	//logger.Debug("\n\n" + string(req_data) + "\n\n")
	if err := p.sendToUpstreamServer(req_data); err != nil {
		if err != ProxyErrorSendToUpstreamTimeout {
			logger.Debug("unhandled send to upstream server errror")
		}
		return err
	}

	resData, err := p.readFromUpstreamServer(ev)
	if err != nil {
		if err != ProxyErrorReadFromUpstreamTimeout {
			logger.Debug("unhandled read form upstream server error")
		}
		// logger.Debug("	send\n%s\nto server%s", string(ev.DEBUG_BUFFER), ev.Conn.RemoteAddr().String())
		return err
	}

	if !ev.RR.Req.IsKeepalive() && ev.Upgrade == "" { // connection close
		p.Close()
	}

	head_code := p.ChangeHeader(ev.RR.Req.IsKeepalive(), resData, &ev.RR)
	b_len := len(ev.RR.Res.Body)

	core.LogOther(&ev.Log, "status", head_code)
	core.LogOther(&ev.Log, "size", strconv.Itoa(b_len))
	core.Log(&ev.Log, ev, "")
	core.LogClear(&ev.Log)

	return nil // no error
}

/*  ====  这两个函数可以用 event ===== */
func (p *Proxy) proxyNeedCache(pc *ProxyCache, req_data []byte, ev *core.Event) {
	var res []byte
	var err error

	flag := false
	uriStringMd5, _ := pc.ProcessCacheConfig(ev, "")
	_, flag = cache.GCacheContainer.ReadCache(uriStringMd5)

	if ev.RR.Req.Headers["Cache-Control"] == "no-cache" {
		flag = false
	}

	if !flag {

		err = p.getDataFromServer(ev, req_data)

		// Server error
		if err != nil {
			ev.RR.Res = response.DefaultServerError()
			ev.WriteResponseClose(nil)
			return
		}
		pc.CacheData(ev, "200", res, len(res))

	} else {
		core.Log(&ev.Log, ev, "")
		core.LogClear(&ev.Log)
	}

	// proxy server return valid data
	if ev.RR.Req.IsKeepalive() {
		ev.WriteResponse(nil)
		// events.Handle_event(ev)
		ev.Reuse = true
	} else {
		ev.WriteResponseClose(nil)
	}
}

func (p *Proxy) proxyNoCache(req_data []byte, ev *core.Event) {

	err := p.getDataFromServer(ev, req_data)

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
		ev.RR.Res = response.DefaultServerError()
		ev.WriteResponseClose(nil)
		return
	}

	if proxy.ProxyNeedCache {
		proxyCache := NewProxyCache(cfg.ProxyCache.Key, cfg.ProxyCache.Valid, cfg.ProxyCache.Path)
		proxy.proxyNeedCache(proxyCache, req_data, ev)
	} else {
		proxy.proxyNoCache(req_data, ev)
	}
}

func ProxyFilterHandler(cfg *listener.ListenCfg, ev *core.Event) bool {
	ChangeHead(cfg, ev)
	appfirewall.HandleAppFireWall(cfg, ev.RR.Req)
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
