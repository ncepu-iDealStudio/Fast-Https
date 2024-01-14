package proxy

import (
	"crypto/tls"
	"fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/core"
	"fast-https/utils"
	"time"

	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"io"
	"net"
	"strconv"
	"strings"
)

type Proxy struct {
	ProxyAddr      string // TODO: upstream
	ProxyConn      net.Conn
	ProxyType      int
	ProxyNeedCache bool
}

func init() {
	core.RRHandlerRegister(config.PROXY_HTTP, ProxyFliterHandler, ProxyEvent)
	core.RRHandlerRegister(config.PROXY_HTTPS, ProxyFliterHandler, ProxyEvent)
}

func Newproxy(addr string, proxyType int, proxyNeedCache bool) *Proxy {
	return &Proxy{
		ProxyType:      proxyType,
		ProxyNeedCache: proxyNeedCache,
		ProxyAddr:      addr,
	}
}

// connect to the server
func (p *Proxy) ProxyInit() error {
	var err error
	p.ProxyConn, err = net.Dial("tcp", p.ProxyAddr)
	if err != nil {
		message.PrintWarn("Proxy event: Can't connect to " + err.Error())
		return err // no server
	}
	now := time.Now()
	p.ProxyConn.SetDeadline(now.Add(time.Second * 20)) // proxy server time out

	return nil
}

// TODO: when add upstream, this function need to do more
func (p *Proxy) proxyHandleAddr() {
	if !strings.Contains(p.ProxyAddr, ":") {
		p.ProxyAddr = p.ProxyAddr + ":80"
	}
}

func ChangeHeader(tmpByte []byte) ([]byte, string, string) {

	header := make([]byte, 1024*20)
	var header_str string
	var header_new string

	var i int
	var res []byte

	for i = 0; i < len(tmpByte)-4; i++ {
		if tmpByte[i] == byte(13) && tmpByte[i+1] == byte(10) &&
			tmpByte[i+2] == byte(13) && tmpByte[i+3] == byte(10) {
			break
		}
		header[i] = tmpByte[i]
	}

	body := tmpByte[i+4:]
	header_str = string(header[:i])
	lines := strings.Split(header_str, "\r\n")
	head_code := strings.Split(lines[0], " ")[1]
	header_new = lines[0] + "\r\n"
	for _, line := range lines[1:] {
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if strings.Compare(key, "Server") == 0 {
				header_new = header_new + "Server: Fast-Https\r\n"
			} else {
				header_new = header_new + key + ": " + value + "\r\n"
			}
		}
	}
	header_new = header_new + "\r\n"

	res = append(res, []byte(header_new)...)
	res = append(res, body...)
	// res = append(res, []byte("\r\n")...)

	return res, head_code, strconv.Itoa(len(body))
}

func (p *Proxy) proxyReadAll(ev *core.Event) ([]byte, error) {
	var resData []byte
	tmpByte := make([]byte, 1024)
	for {
		len_once, err := p.ProxyConn.Read(tmpByte)
		if err != nil {
			if err == io.EOF { // read all
				break
			} else {
				p.ProxyConn.Close()
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

	_, err = p.ProxyConn.Write(req_data)
	if err != nil {
		p.ProxyConn.Close() // close proxy connection
		ev.Close()          // close event connection
		message.PrintWarn("Proxy event: Can't write to " + err.Error())
		return nil, err // can't write
	}

	var resData []byte
	if !ev.RR.Req_.IsKeepalive() {
		resData, err = p.proxyReadAll(ev)
		// fmt.Println("-----This is proxyReadAll")
		if err != nil {
			message.PrintWarn("Proxy event: Can't read all", err.Error())
		}
	} else {
		ro := ReadOnce{
			TryNum:    0,
			ProxyConn: p.ProxyConn,
		}
		err = ro.proxyReadOnce(ev)
		// fmt.Println("-----This is proxyReadOnce")
		if err != nil {
			message.PrintWarn("Proxy event: Can't read once", err.Error())
		}
		resData = ro.finalStr
	}

	finalData, head_code, b_len := ChangeHeader(resData)

	ev.Log_append(" " + head_code + " " + b_len)

	if !ev.RR.Req_.IsKeepalive() { // connection close
		p.ProxyConn.Close()
	}

	message.PrintAccess(ev.Conn.RemoteAddr().String(), "PROXY HTTP Event"+
		ev.Log, "\""+ev.RR.Req_.Headers["User-Agent"]+"\"")

	ev.Log_clear()
	return finalData, nil // no error
}

// fast-https will send data to real server and get response from target
func (p *Proxy) getDataFromSslServer(ev *core.Event,
	data []byte) ([]byte, error) {

	var err error

	config := tls.Config{InsecureSkipVerify: true}
	tlsConn := tls.Client(p.ProxyConn, &config)

	_, err = tlsConn.Write(data)
	if err != nil {
		tlsConn.Close()
		message.PrintErr("Proxy Write error")
		return nil, err // cant' write
	}

	var resData []byte
	if !ev.RR.Req_.IsKeepalive() {
		resData, err = p.proxyReadAll(ev)
		// fmt.Println("-----This is proxyReadAll")
		if err != nil {
			message.PrintWarn("Proxy event: Can't read all", err.Error())
		}
	} else {
		ro := ReadOnce{
			TryNum:    0,
			ProxyConn: p.ProxyConn,
		}
		err = ro.proxyReadOnce(ev)
		// fmt.Println("-----This is proxyReadOnce")
		if err != nil {
			message.PrintWarn("Proxy event: Can't read once", err.Error())
		}
		resData = ro.finalStr
	}

	finalData, head_code, b_len := ChangeHeader(resData)

	ev.Log_append(" " + head_code + " " + b_len)

	if !ev.RR.Req_.IsKeepalive() {
		tlsConn.Close()
	}

	message.PrintAccess(ev.Conn.RemoteAddr().String(), "PROXY HTTPS Event"+
		ev.Log, "\""+ev.RR.Req_.Headers["User-Agent"]+"\"")
	ev.Log_clear()
	return finalData, nil // no error
}

type ProxyCache struct {
	ProxyCachePath  string
	ProxyCacheKey   string
	ProxyCacheValid []string
}

func NewProxyCache(key string, valid []string, path string) *ProxyCache {
	return &ProxyCache{
		ProxyCacheKey:   key,
		ProxyCacheValid: valid,
		ProxyCachePath:  path,
	}
}

// to do: improve this function
func (pc *ProxyCache) ProcessCacheConfig(ev *core.Event,
	resCode string) (md5 string, expire int) {

	cacheKeyRule := pc.ProxyCacheKey
	keys := strings.Split(cacheKeyRule, "$")
	rule := map[string]string{ // 配置缓存key字段的生成规则
		"request_method": ev.RR.Req_.Method,
		"request_uri":    ev.RR.Req_.Path,
		"host":           ev.RR.Req_.GetHeader("Host"),
	}

	ruleString := ""
	for _, item := range keys {
		str, ok := rule[item]
		if !ok { // 未配置相应字段的生成规则，跳过即可
			continue
		}
		ruleString += str
	}
	// fmt.Println("-------------------", ev.RR.Req_.Path)
	// fmt.Println("generate cache key value=", ruleString)
	md5 = cache.GetMd5(ruleString)

	// convert ["200:1h", "304:1h", "any:30m"]
	valid := pc.ProxyCacheValid
	for _, c := range valid {
		split := strings.Split(c, ":")
		if split[0] != resCode || split[0] == "any" {
			expire = utils.ParseTime(split[1])
			// fmt.Println("generate cache expire time=", expire)
			return
		}
	}
	return
}

func (pc *ProxyCache) CacheData(ev *core.Event, resCode string,
	data []byte, size int) {

	// according to usr's config, create a key
	uriStringMd5, expireTime := pc.ProcessCacheConfig(ev, resCode)
	cache.GCacheContainer.WriteCache(uriStringMd5, expireTime,
		pc.ProxyCachePath, data, size)
	// fmt.Println(cfg.ProxyCache.Key, cfg.ProxyCache.Path,
	// cfg.ProxyCache.MaxSize, cfg.ProxyCache.Valid)
}

func (p *Proxy) proxyNeedCache(pc *ProxyCache, req_data []byte, ev *core.Event) {
	var res []byte
	var err error

	flag := false
	uriStringMd5, _ := pc.ProcessCacheConfig(ev, "")
	res, flag = cache.GCacheContainer.ReadCache(uriStringMd5)

	if ev.RR.Req_.Headers["Cache-Control"] == "no-cache" {
		flag = false
	}

	if !flag {

		if p.ProxyType == config.PROXY_HTTP { // http proxy
			res, err = p.getDataFromServer(ev, req_data)
		} else if p.ProxyType == config.PROXY_HTTPS { // https proxy
			res, err = p.getDataFromSslServer(ev, req_data)
		} else {
			message.PrintErr("invalid path type")
		}
		// Server error
		if err != nil {
			ev.WriteDataClose(response.DefaultServerError())
			return
		}
		pc.CacheData(ev, "200", res, len(res))

	} else {
		message.PrintAccess(ev.Conn.RemoteAddr().String(),
			"PROXY Event(Cache)"+ev.Log,
			"\""+ev.RR.Req_.Headers["User-Agent"]+"\"")
	}

	// proxy server return valid data
	if ev.RR.Req_.IsKeepalive() {
		ev.WriteData(res)
		// events.Handle_event(ev)
		ev.Reuse = true
	} else {
		ev.WriteDataClose(res)
	}
}

func (p *Proxy) proxyNoCache(req_data []byte, ev *core.Event) {
	var res []byte
	var err error

	if p.ProxyType == config.PROXY_HTTP { // http proxy
		res, err = p.getDataFromServer(ev, req_data)
	} else if p.ProxyType == config.PROXY_HTTPS { // https proxy
		res, err = p.getDataFromSslServer(ev, req_data)
	} else {
		message.PrintErr("invalid path type")
	}
	if err != nil {
		ev.WriteDataClose(response.DefaultServerError())
		return
	}
	// proxy server return valid data
	if ev.RR.Req_.IsKeepalive() {
		ev.WriteData(res)
		// events.Handle_event(ev)
		ev.Reuse = true
	} else {
		ev.WriteDataClose(res)
	}
}

/*
 *************************************
 ****** Interfaces are as follows ****
 *************************************
 */
func ProxyEvent(cfg listener.ListenCfg, ev *core.Event) {
	req_data := ev.RR.Req_.ByteRow()

	configCache := true
	if cfg.ProxyCache.Key == "" {
		configCache = false
	}

	var proxy *Proxy

	// init proxy tcp connection
	if ev.RR.ProxyConnInit == false {
		ev.RR.ProxyConnInit = true
		proxy = Newproxy(cfg.Proxy_addr, int(cfg.Type), configCache)
		proxy.ProxyInit()
		proxy.proxyHandleAddr()
		ev.RR.CircleData = proxy
	} else {
		var flag bool
		proxy, flag = (ev.RR.CircleData).(*Proxy)
		if !flag {
			message.PrintErr("--proxy can not convert circle data to *Proxy")
		}
	}

	if configCache {
		proxyCache := NewProxyCache(cfg.ProxyCache.Key, cfg.ProxyCache.Valid, cfg.ProxyCache.Path)
		proxy.proxyNeedCache(proxyCache, req_data, ev)
	} else {
		proxy.proxyNoCache(req_data, ev)
	}
}

func ProxyFliterHandler(cfg listener.ListenCfg, ev *core.Event) bool {
	ChangeHead(cfg, ev)
	return true
}

func ChangeHead(cfg listener.ListenCfg, ev *core.Event) {
	for _, item := range cfg.ProxySetHeader {
		if item.HeaderKey == "Host" {
			if item.HeaderValue == "$host" {
				ev.RR.Req_.SetHeader("Host", cfg.Proxy_addr, cfg)
			}
		}
		if !strings.Contains(item.HeaderValue, "$") {
			ev.RR.Req_.SetHeader(item.HeaderKey, item.HeaderValue, cfg)
		}
	}
	// ev.RR.Req_.SetHeader("Host", cfg.Proxy_addr, cfg)
	// ev.RR.Req_.SetHeader("Connection", "close", cfg)
	ev.RR.Req_.Flush()
}
