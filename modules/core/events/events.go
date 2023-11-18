package events

import (
	"fast-https/modules/cache"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/core/timer"
	"fast-https/utils"
	"fast-https/utils/message"
	"fmt"
	"io"
	"net"
	"reflect"
	"regexp"
	"strings"
	"unsafe"
)

// request and response circle
type RRcircle struct {
	Req_      *request.Req
	Res_      *response.Response
	CircleNum int
	// uri after re
	OriginPath   string
	PathLocation []int
	ProxyConn    net.Conn
	// Handler
}

// each request event is saved in this struct
type Event struct {
	Conn     net.Conn
	Lis_info listener.Listener
	Timer    *timer.Timer
	Log      string
	Type     uint64
	RR       RRcircle
}

// distribute event
// LisType(2) tcp proxy
func Handle_event(ev *Event) {
	// handle tcp proxy
	if ev.Lis_info.LisType == 2 {
		Proxy_event_tcp(ev.Conn, ev.Lis_info.Cfg[0].Proxy_addr)
		return
	}
	if process_request(ev) == 0 {
		return // client close
	}
	log_append(ev, " "+ev.RR.Req_.Method)
	log_append(ev, " "+ev.RR.Req_.Path+" \""+ev.RR.Req_.Get_header("Host")+"\"")

	hosts := ev.Lis_info.HostMap[ev.RR.Req_.Get_header("Host")]
	for _, cfg := range hosts {
		re := regexp.MustCompile(cfg.Path) // we can compile this when load config
		res := re.FindStringIndex(ev.RR.Req_.Path)
		if res != nil {
			originPath := ev.RR.Req_.Path[res[1]:]
			ev.RR.OriginPath = originPath
			ev.RR.PathLocation = res

			switch cfg.Proxy {
			case 0:
				if HandelSlash(cfg, ev) {
					return
				}
				// according to user's confgure and requets endporint handle events
				Static_event(cfg, ev)
				return
			case 1, 2:
				ChangeHead(cfg, ev)
				// according to user's confgure and requets endporint handle events
				Proxy_event(cfg, ev)
				return
			}
		}
	}
	write_bytes_close(ev, response.Default_not_found())
}

func HandelSlash(cfg listener.ListenCfg, ev *Event) (flag bool) {
	if ev.RR.OriginPath == "" && cfg.Path != "/" {
		_event_301(ev, ev.RR.Req_.Path[ev.RR.PathLocation[0]:ev.RR.PathLocation[1]]+"/")
		return true
	}
	return false
}

func ChangeHead(cfg listener.ListenCfg, ev *Event) {
	for _, item := range cfg.ProxySetHeader {
		if item.HeaderKey == 100 {
			if item.HeaderValue == "$host" {
				ev.RR.Req_.Set_header("Host", cfg.Proxy_addr, cfg)
			}
		}
	}
	ev.RR.Req_.Set_header("Host", cfg.Proxy_addr, cfg)
	ev.RR.Req_.Set_header("Connection", "close", cfg)
	ev.RR.Req_.Flush()
}

// to do: improve this function
func ProcessCacheConfig(ev *Event, cfg listener.ListenCfg, resCode string) (md5 string, expire int) {
	cacheKeyRule := cfg.ProxyCache.Key
	keys := strings.Split(cacheKeyRule, "$")
	rule := map[string]string{ // 配置缓存key字段的生成规则
		"request_method": ev.RR.Req_.Method,
		"request_uri":    ev.RR.Req_.Path,
		"host":           ev.RR.Req_.Get_header("Host"),
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
	valid := cfg.ProxyCache.Valid
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

func CacheData(ev *Event, cfg listener.ListenCfg, resCode string, data []byte, size int) {
	// according to usr's config, create a key
	uriStringMd5, expireTime := ProcessCacheConfig(ev, cfg, resCode)
	cache.GCacheContainer.WriteCache(uriStringMd5, expireTime, cfg.ProxyCache.Path, data, size)
	// fmt.Println(cfg.ProxyCache.Key, cfg.ProxyCache.Path, cfg.ProxyCache.MaxSize, cfg.ProxyCache.Valid)
}

func process_request(ev *Event) int {
	// read data (bytes and str) from socket
	byte_row, str_row := read_data(ev)
	// save requte information to ev.RR.Req_
	ev.RR.Req_ = request.Req_init()
	if byte_row == nil { // client closed
		close(ev)
		return 0
	} else {
		ev.RR.Req_.Http_parse(str_row)
		ev.RR.Req_.Parse_body(byte_row)
		// parse host
		ev.RR.Req_.Parse_host(ev.Lis_info)
	}
	return 1
}

func log_append(ev *Event, log string) {
	ev.Log = ev.Log + log
}

// read data from EventFd
// attention: row str only can be used when parse FirstLine or Headers
// because request body maybe contaions '\0'
func read_data(ev *Event) ([]byte, string) {
	buffer := make([]byte, 1024*4)
	n, err := ev.Conn.Read(buffer)
	if err != nil {
		if err == io.EOF || n == 0 { // read None, remoteAddr is closed
			// message.PrintInfo(ev.Conn.RemoteAddr(), " closed")
			return nil, ""
		}
		opErr := (*net.OpError)(unsafe.Pointer(reflect.ValueOf(err).Pointer()))
		if opErr.Err.Error() == "i/o timeout" {
			message.PrintWarn("read timeout")
			return nil, ""
		}
		fmt.Println("Error reading from client 176:", err)
	}
	str_row := string(buffer[:n])
	// buffer = buffer[:n]
	return buffer, str_row // return row str or bytes
}

// write row bytes
func write_bytes(ev *Event, data []byte) {
	for len(data) > 0 {
		n, err := ev.Conn.Write(data)
		if err != nil {
			opErr := (*net.OpError)(unsafe.Pointer(reflect.ValueOf(err).Pointer()))
			if opErr.Err.Error() == "i/o timeout" {
				message.PrintWarn("write timeout")
				return
			}
			fmt.Println("Error writing to client 193:", err)
			return
		}
		data = data[n:]
	}
}

// only close the connection
func close(ev *Event) {
	err := ev.Conn.Close()
	if err != nil {
		fmt.Println("Error Close:", err)
	}
}

// write row bytes and close
func write_bytes_close(ev *Event, data []byte) {
	write_bytes(ev, data)
	close(ev)
}
