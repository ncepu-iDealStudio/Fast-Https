package events

import (
	"crypto/tls"
	"fast-https/modules/cache"

	"fast-https/modules/core"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

func Change_header(tmpByte []byte) ([]byte, string, string) {

	header := make([]byte, 1024*20)
	var header_str string
	var header_new string

	var i int
	var res []byte

	for i = 0; i < len(tmpByte)-4; i++ {
		if tmpByte[i] == byte(13) && tmpByte[i+1] == byte(10) && tmpByte[i+2] == byte(13) && tmpByte[i+3] == byte(10) {
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
	res = append(res, []byte("\r\n")...)

	return res, head_code, strconv.Itoa(len(body))
}

// fast-https will send data to real server and get response from target
func get_data_from_server(ev *core.Event, proxyaddr string, data []byte) ([]byte, int) {
	if !strings.Contains(proxyaddr, ":") {
		proxyaddr = proxyaddr + ":80"
	}

	var err error
	if ev.RR.ProxyConn == nil {
		ev.RR.ProxyConn, err = net.Dial("tcp", proxyaddr)
		if err != nil {
			message.PrintWarn("[Proxy event]: Can't connect to "+proxyaddr, err.Error())
			return nil, 1 // no server
		}
		now := time.Now()
		ev.RR.ProxyConn.SetDeadline(now.Add(time.Second * 20)) // proxy server time out
	}

	_, err = ev.RR.ProxyConn.Write(data)
	if err != nil {
		ev.RR.ProxyConn.Close() // close proxy connection
		ev.Close()              // close event connection
		message.PrintWarn("[Proxy event]: Can't write to "+proxyaddr, err.Error())
		return nil, 2 // can't write
	}
	// fmt.Println(string(data))

	var resData []byte
	tmpByte := make([]byte, 512)
	for {
		len_once, err := ev.RR.ProxyConn.Read(tmpByte)
		if err != nil {
			if err == io.EOF { // read all
				break
			} else {
				ev.RR.ProxyConn.Close()
				ev.Close()
				message.PrintWarn("[Proxy event]: Can't read from "+proxyaddr, err.Error())
				return nil, 3 // can't read
			}
		}
		if len_once == 0 {
			break
		}
		resData = append(resData, tmpByte[:len_once]...)
	}

	finalData, head_code, b_len := Change_header(resData)

	log_append(ev, " "+head_code+" "+b_len)

	if !ev.RR.Req_.Is_keepalive() { // connection close
		ev.RR.ProxyConn.Close()
	}

	message.PrintAccess(ev.Conn.RemoteAddr().String(), "PROXY HTTP Event"+ev.Log, "\""+ev.RR.Req_.Headers["User-Agent"]+"\"")
	return finalData, 0 // no error
}

// fast-https will send data to real server and get response from target
func get_data_from_ssl_server(ev *core.Event, proxyaddr string, data []byte) ([]byte, int) {
	if !strings.Contains(proxyaddr, ":") {
		proxyaddr = proxyaddr + ":443"
	}

	var err error
	if ev.RR.ProxyConn == nil {
		ev.RR.ProxyConn, err = net.Dial("tcp", proxyaddr)
		if err != nil {
			message.PrintWarn("Can't connect to "+proxyaddr, err.Error())
			return nil, 1 // no server
		}
	}

	config := tls.Config{InsecureSkipVerify: true}
	tlsConn := tls.Client(ev.RR.ProxyConn, &config)

	_, err = tlsConn.Write(data)
	if err != nil {
		tlsConn.Close()
		message.PrintErr("Proxy Write error")
		return nil, 2 // cant' write
	}

	var resData []byte
	tmpByte := make([]byte, 512)
	for {
		len_once, err := tlsConn.Read(tmpByte)
		if err != nil {
			if err == io.EOF { // read all
				break
			} else {
				tlsConn.Close()
				message.PrintWarn("[Proxy event]: Can't read from "+proxyaddr, err.Error())
				return nil, 3 // can't read
			}
		}
		if len_once == 0 {
			break
		}
		resData = append(resData, tmpByte[:len_once]...)
	}

	finalData, head_code, b_len := Change_header(resData)

	log_append(ev, " "+head_code+" "+b_len)

	if !ev.RR.Req_.Is_keepalive() {
		tlsConn.Close()
	}

	message.PrintAccess(ev.Conn.RemoteAddr().String(), "PROXY HTTPS Event"+ev.Log, "\""+ev.RR.Req_.Headers["User-Agent"]+"\"")
	return finalData, 0 // no error
}

func proxyNeedCache(req_data []byte, cfg listener.ListenCfg, ev *core.Event) {
	var res []byte
	var err int

	flag := false
	uriStringMd5, _ := ProcessCacheConfig(ev, cfg, "")
	res, flag = cache.GCacheContainer.ReadCache(uriStringMd5)

	if !flag {

		if cfg.Proxy == 1 { // http proxy
			res, err = get_data_from_server(ev, cfg.Proxy_addr, req_data)
		} else {
			res, err = get_data_from_ssl_server(ev, cfg.Proxy_addr, req_data)
		}
		// Server error
		if err != 0 {
			write_bytes_close(ev, response.Default_server_error())
			return
		}
		CacheData(ev, cfg, "200", res, len(res))
	} else {
		message.PrintAccess(ev.Conn.RemoteAddr().String(), "PROXY Event(Cache)"+ev.Log, "\""+ev.RR.Req_.Headers["User-Agent"]+"\"")
	}

	// proxy server return valid data
	if ev.RR.Req_.Is_keepalive() {
		ev.Write_bytes(res)
		Handle_event(ev)
	} else {
		write_bytes_close(ev, res)
	}

}

func Proxy_event(cfg listener.ListenCfg, ev *core.Event) {
	req_data := ev.RR.Req_.Byte_row()

	configCache := true
	if cfg.ProxyCache.Key == "" {
		configCache = false
	}

	if configCache {
		proxyNeedCache(req_data, cfg, ev)
	} else {
		var res []byte
		var err int

		if cfg.Proxy == 1 { // http proxy
			res, err = get_data_from_server(ev, cfg.Proxy_addr, req_data)
		} else {
			res, err = get_data_from_ssl_server(ev, cfg.Proxy_addr, req_data)
		}
		if err != 0 {
			write_bytes_close(ev, response.Default_server_error())
			return
		}
		// proxy server return valid data
		if ev.RR.Req_.Is_keepalive() {
			ev.Write_bytes(res)
			Handle_event(ev)
		} else {
			write_bytes_close(ev, res)
		}

	}
}
