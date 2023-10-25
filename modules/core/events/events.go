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

// each request event is saved in this struct
type Event struct {
	Conn      net.Conn
	ProxyConn net.Conn
	Lis_info  listener.ListenInfo
	Req_      *request.Req
	Res_      *response.Response
	Timer     *timer.Timer
	Log       string
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
	log_append(ev, " "+ev.Req_.Method)
	log_append(ev, " "+ev.Req_.Path+" \""+ev.Req_.Get_header("Host")+"\"")

	for _, cfg := range ev.Lis_info.Cfg {
		switch cfg.Proxy {
		case 0: // Proxy: 0, static events
			// if ev.Req_.Get_header("Host") == cfg.ServerName && strings.HasPrefix(ev.Req_.Path, cfg.Path) {

			re := regexp.MustCompile(cfg.Path)
			res := re.FindStringIndex(ev.Req_.Path)
			if ev.Req_.Get_header("Host") == cfg.ServerName && res != nil {
				fmt.Println("matched", res)
				row_file_path := ev.Req_.Path[res[1]:]
				if row_file_path == "" && cfg.Path != "/" {
					fmt.Println("301")
					_event_301(ev, ev.Req_.Path[res[0]:res[1]]+"/")
					return
				}
				// according to user's confgure and requets endporint handle events
				Static_event(cfg, row_file_path, ev)
				return
			}
		case 1, 2: // proxy: 1 or 2,  proxy events

			re := regexp.MustCompile(cfg.Path)
			res := re.FindStringIndex(ev.Req_.Path)
			if ev.Req_.Get_header("Host") == cfg.ServerName && res != nil {

				for _, item := range cfg.ProxySetHeader {
					if item.HeaderKey == 100 {
						if item.HeaderValue == "$host" {
							ev.Req_.Set_header("Host", cfg.Proxy_addr, cfg)
						}
					}
				}
				ev.Req_.Set_header("Host", cfg.Proxy_addr, cfg)
				ev.Req_.Set_header("Connection", "close", cfg)
				ev.Req_.Flush()
				flush_bytes := ev.Req_.Byte_row()

				// according to user's confgure and requets endporint handle events
				Proxy_event(flush_bytes, cfg, ev)
				return
			}
		}
	}
	write_bytes_close(ev, response.Default_not_found())
}

// to do: improve this function
func ProcessCacheConfig(ev *Event, cfg listener.ListenCfg, resCode string) (md5 string, expire int) {
	cacheKeyRule := cfg.ProxyCache.Key
	keys := strings.Split(cacheKeyRule, "$")
	rule := map[string]string{ // 配置缓存key字段的生成规则
		"request_method": ev.Req_.Method,
		"request_uri":    ev.Req_.Path,
		"host":           ev.Req_.Get_header("Host"),
	}

	ruleString := ""
	for _, item := range keys {
		str, ok := rule[item]
		if !ok { // 未配置相应字段的生成规则，跳过即可
			continue
		}
		ruleString += str
	}
	// fmt.Println("-------------------", ev.Req_.Path)
	fmt.Println("generate cache key value=", ruleString)
	md5 = cache.GetMd5(ruleString)

	// convert ["200:1h", "304:1h", "any:30m"]
	valid := cfg.ProxyCache.Valid
	for _, c := range valid {
		split := strings.Split(c, ":")
		if split[0] != resCode || split[0] == "any" {
			expire = utils.ParseTime(split[1])
			fmt.Println("generate cache expire time=", expire)
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
	// save requte information to ev.Req_
	ev.Req_ = request.Req_init()
	if byte_row == nil { // client closed
		close(ev)
		return 0
	} else {
		ev.Req_.Http_parse(str_row)
		ev.Req_.Parse_body(byte_row)
		// parse host
		ev.Req_.Parse_host(ev.Lis_info)
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
		fmt.Println("Error reading from client 104:", err)
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
			fmt.Println("Error writing to client 155:", err)
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
