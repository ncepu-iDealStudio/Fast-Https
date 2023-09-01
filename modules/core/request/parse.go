package request

import (
	"fast-https/modules/core/listener"
	"strings"

	"github.com/chenhg5/collection"
)

const (
	REQUEST_OK         = 0
	NONE               = 1
	UNKNOW_INVALID     = 2
	FIRST_LINE_INVALID = 3
	METHOD_INVALID     = 4
)

// this struct is saved in Event
// which contaions event's method,path,servername(headers)
type Req struct {
	Method   string
	Path     string
	Protocol string
	Encoding []string
	Headers  map[string]string
	Body     []byte
}

var http_method = []string{
	"GET",
	"POST",
	"PUT",
	"DELETE",
	"HEAD",
	"OPTIONS",
	"TRACE",
	"CONNECT",
}

func Req_init() *Req {
	return &Req{
		Headers: make(map[string]string),
	}
}

// parse Host
func (r *Req) Parse_host(lis_info listener.ListenInfo) {
	if lis_info.Port == "80" {
		r.Headers["Host"] = r.Headers["Host"] + ":80"
	} else if lis_info.Port == "443" {
		r.Headers["Host"] = r.Headers["Host"] + ":443"
	}
}

// reset request's header
func (r *Req) Set_header(key string, val string, cfg listener.ListenCfg) {

	r.Headers[key] = val

	// _, ref := r.Headers["Referer"]
	// if ref {
	// 	ori := r.Headers["Referer"]
	// 	after := strings.Replace(ori, cfg.ServerName, r.Headers["Host"], -1)
	// 	r.Headers["Referer"] = after
	// }

	// _, ori := r.Headers["Origin"]
	// if ori {
	// 	if cfg.Proxy == 1 {
	// 		r.Headers["Origin"] = "http://" + cfg.Proxy_addr
	// 	} else if cfg.Proxy == 2 {
	// 		r.Headers["Origin"] = "https://" + cfg.Proxy_addr
	// 	} else {
	// 		fmt.Println("SET header error...")
	// 	}
	// }
}

// flush request struct
func (r *Req) Flush() {}

// get request header
func (r *Req) Get_header(key string) string {
	return r.Headers[key]
}

// whether the request connection is keep alive
func (r *Req) Is_keepalive() bool {
	conn := r.Get_header("Connection")
	if conn == "keep-alive" {
		return true
	} else {
		return false
	}
}

// get request row bytes
func (r *Req) Byte_row() []byte {
	rowStr := r.Method + " " +
		r.Path + " " +
		r.Protocol + "\r\n"
	for k, v := range r.Headers {
		rowStr = rowStr + k + ": " + v + "\r\n"
	}
	rowStr = rowStr + "\r\n"

	rowByte := []byte(rowStr)
	rowByte = append(rowByte, r.Body...)

	return rowByte
}

// parse row tcp str to a req object
func (r *Req) Http_parse(request string) int {

	if request == "" {
		return NONE
	}
	requestLine := strings.Split(request, "\r\n")
	if requestLine == nil {
		return UNKNOW_INVALID // invalid request
	}
	parts := strings.Split(requestLine[0], " ")
	if parts == nil || len(parts) < 3 {
		return FIRST_LINE_INVALID // invalid first line
	}
	if !collection.Collect(http_method).Contains(parts[0]) {
		return METHOD_INVALID // invalid method
	}

	r.Method = parts[0]
	r.Path = parts[1]
	r.Protocol = parts[2]

	lines := strings.Split(request, "\r\n")[1:]
	for _, line := range lines {
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		r.Headers[key] = value
	}

	return REQUEST_OK // valid
}

// get request's body
func (r *Req) Parse_body(tmpByte []byte) {
	var i int // last byte position before \r\n\r\n
	var remain_len int
	var res []byte

	total_len := len(tmpByte)
	for i = 0; i < total_len-4; i++ {
		if tmpByte[i] == byte(13) && tmpByte[i+1] == byte(10) && tmpByte[i+2] == byte(13) && tmpByte[i+3] == byte(10) {
			break
		}
	}

	remain_len = total_len - i - 4

	if remain_len == 0 {
		res = (tmpByte[i+4:])
	} else {
		res = (tmpByte[i+4:])[:remain_len]
	}

	r.Body = res
}
