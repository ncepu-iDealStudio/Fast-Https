package events

import (
	"fast-https/modules/cache"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fast-https/modules/core/timer"
	"fast-https/utils/message"
	"fmt"
	"io"
	"net"
	"reflect"
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
	ev.Log = " " + ev.Req_.Method
	ev.Log = ev.Log + " " + ev.Req_.Path + " \"" + ev.Req_.Get_header("Host") + "\""

	for _, cfg := range ev.Lis_info.Cfg {
		switch cfg.Proxy {
		case 0: // Proxy: 0, static events
			if ev.Req_.Get_header("Host") == cfg.ServerName && strings.HasPrefix(ev.Req_.Path, cfg.Path) {
				row_file_path := ev.Req_.Path[len(cfg.Path):]
				if row_file_path == "" && cfg.Path != "/" {
					_event_301(ev, cfg.Path+"/")
					return
				}
				// according to user's confgure and requets endporint handle events
				Static_event(cfg, row_file_path, ev)
				return
			}
		case 1, 2: // proxy: 1 or 2,  proxy events
			if ev.Req_.Get_header("Host") == cfg.ServerName {

				for _, item := range cfg.ProxySetHeader {
					if item.HeaderKey == 100 {
						var str string
						if item.HeaderValue == "$host" {
							str = cfg.Proxy_addr
							ev.Req_.Set_header("Host", str, cfg)
						}
					}
				}

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

func ProcessCacheConfig(ev *Event, cfg listener.ListenCfg, resCode string) (md5 string, expire int) {
	// to do: config convert cacheProxyKey to []int {1, 2, 3 ...}
	rule := []int{1, 2, 3}
	ruleString := ""
	for item := range rule {
		switch item {
		case 1: // request_method
			{
				ruleString += ev.Req_.Method
			}
		case 2: // host
			{
				ruleString += ev.Req_.Get_header("Host")
			}
		case 3: // request_uri
			{
				ruleString += ev.Req_.Path
			}
		}
	}
	md5 = cache.GetMd5(ruleString)

	// to do: convert ["200:1h", "304:1h", "any:30m"]
	expire = 10

	return
}

func CacheData(ev *Event, cfg listener.ListenCfg, resCode string, data []byte, size int) {
	// according to usr's config, create a key
	uriStringMd5, expireTime := ProcessCacheConfig(ev, cfg, resCode)
	cache.GCacheContainer.WriteCache(uriStringMd5, expireTime, cfg.ProxyCache.Path, data, size)

	fmt.Println(cfg.ProxyCache.Key, cfg.ProxyCache.Path, cfg.ProxyCache.MaxSize, cfg.ProxyCache.Valid)

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

// write row bytes and close
func write_bytes_close(ev *Event, data []byte) {
	write_bytes(ev, data)
	close(ev)
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
