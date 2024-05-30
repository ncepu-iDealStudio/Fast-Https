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
	"fast-https/utils/message"
	"io"
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

func (m *Proxy) Error() string {
	return "proxy error"
}

// connect to the server
// init read write close handler
func (p *Proxy) ProxyInit() error {
	var err error

	p.ProxyConn, err = net.Dial("tcp", p.ProxyAddr)

	p.Read = p.read
	p.Write = p.write
	p.Close = p.close

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
		message.PrintWarn("Proxy event: Can't connect to " + err.Error())
		return err // no server
	}
	now := time.Now()
	p.ProxyConn.SetDeadline(now.Add(time.Second * 20)) // proxy server time out

	return nil
}

// get a inited proxy
func getProxy(rr *core.RRcircle, cfg *listener.ListenCfg) (*Proxy, error) {
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
		err := proxy.ProxyInit()
		if err != nil {
			message.PrintWarn("--proxy can not init circle" + err.Error())

			return nil, errors.New("proxy init error")
		}

		rr.CircleData = proxy
	} else {
		var flag bool
		proxy, flag = (rr.CircleData).(*Proxy)
		if !flag {
			message.PrintErr("--proxy can not convert circle data to *Proxy")
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

func (p *Proxy) ChangeHeader(tmpByte []byte, rr *core.RRcircle) ([]byte, string) {

	var res []byte

	temp_res := response.ResponseInit()
	temp_res.HttpResParse(string(tmpByte))

	head_code := strings.Split(temp_res.FirstLine, " ")[1]

	if temp_res.Headers["connection"] == "" {
		p.ProxyNeedClose = true
	}
	if temp_res.Headers["connection"] == "close" {
		p.ProxyNeedClose = true
	}

	temp_res.DelHeader("server")
	temp_res.SetHeader("server", "Fast-Https")

	temp_body := rr.Res.Body
	rr.Res = temp_res
	rr.Res.Body = temp_body

	res = temp_res.GenerateHeaderBytes()

	return res, head_code
}

func (p *Proxy) proxyReadAll(ev *core.Event) ([]byte, error) {

	var resData []byte
	tmpByte := make([]byte, 1024)
	for {
		len_once, err := p.Read(tmpByte)
		if err != nil {
			if err == io.EOF { // read all
				break
			} else {
				p.Close()
				ev.Close()
				return nil, err // can't read
			}
		}
		resData = append(resData, tmpByte[:len_once]...)
	}

	return resData, nil
}

// fast-https will send data to real server and get response from target
func (p *Proxy) getDataFromServer(ev *core.Event,
	req_data []byte) ([]byte, error) {

	var err error

	_, err = p.Write(req_data)
	if err != nil {
		p.Close()  // close proxy connection
		ev.Close() // close event connection
		message.PrintWarn("Proxy event: Can't write to " + err.Error())
		return nil, err // can't write
	}

	var resData []byte
	if !ev.RR.Req.IsKeepalive() {
		if ev.Upgrade == "websocket" {
			web := make([]byte, 1024)
			n, _ := p.Read(web)
			resData = web[:n]
		} else {
			resData, err = p.proxyReadAll(ev)
			// fmt.Println("-----This is proxyReadAll")
			if err != nil {
				message.PrintWarn("Proxy event: Can't read all ", err.Error())
			}
		}
	} else {
		ro := ReadOnce{
			TryNum:       0,
			ProxyConn:    p.ProxyConn,
			ProxyTlsConn: p.ProxyTlsConn,
			Type:         p.ProxyType,
		}
		err = ro.proxyReadOnce(ev)
		// fmt.Println("-----This is proxyReadOnce")
		if err != nil {
			message.PrintWarn("Proxy event: Can't read once ", err.Error())
		}
		resData = ro.finalStr
		// fmt.Println(ev.RR.Res)
		// ev.RR.Res.FirstLine = ro.res.FirstLine
		// ev.RR.Res.Headers = ro.res.Headers
		ev.RR.Res.Body = ro.body
	}

	if !ev.RR.Req.IsKeepalive() && ev.Upgrade == "" { // connection close
		p.Close()
	}

	if len(resData) < 4 {
		message.PrintWarn(ev.Conn.RemoteAddr().String(), " proxy return null")
		p.ProxyConn.Close()
		return nil, errors.New("proxy return null")
	}

	b_len := len(ev.RR.Res.Body)
	headerData, head_code := p.ChangeHeader(resData, &ev.RR)
	fmt.Println("headerData", string(headerData))

	core.LogOther(&ev.Log, "status", head_code)
	core.LogOther(&ev.Log, "size", strconv.Itoa(b_len))
	core.Log(&ev.Log, ev, "")

	core.LogClear(&ev.Log)
	return headerData, nil // no error
}

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

		res, err = p.getDataFromServer(ev, req_data)

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

	_, err := p.getDataFromServer(ev, req_data)

	if err != nil {
		ev.RR.Res = response.DefaultServerError()
		ev.WriteResponseClose(nil)
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

	proxy, err := getProxy(&ev.RR, cfg)
	if err != nil {
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
		// if item.HeaderKey == "Host" {
		// 	if item.HeaderValue == "$host" {
		// 		ev.RR.Req_.SetHeader("Host", cfg.Proxy_addr, cfg)
		// 	}
		// }
		// if !strings.Contains(item.HeaderValue, "$") {
		ev.RR.Req.SetHeader(item.HeaderKey,
			ev.GetCommandParsedStr(item.HeaderValue), cfg)
		// }
		// fmt.Println(item.HeaderKey, ev.GetCommandParsedStr(item.HeaderValue))
	}
	// ev.RR.Req_.SetHeader("Host", cfg.Proxy_addr, cfg)
	// ev.RR.Req_.SetHeader("Connection", "close", cfg)
	// ev.RR.Req.SetHeader("Accept-Encoding", "", cfg)
	ev.RR.Req.Flush()
}
