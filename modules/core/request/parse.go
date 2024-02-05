package request

import (
	"fast-https/modules/core/listener"
	"fmt"
	"strconv"
	"strings"

	"github.com/chenhg5/collection"
)

const (
	REQUEST_OK             = 0
	NONE                   = 1
	UNKNOW_INVALID         = 2
	FIRST_LINE_INVALID     = 3
	METHOD_INVALID         = 4
	REQUEST_NEED_READ_MORE = 5

	INVALID_HEADERS = 6
)

// this struct is saved in Event
// which contaions event's method,path,servername(headers)
type Req struct {
	// HTTP first line
	Method   string `name:"request_method"`
	Path     string `name:"request_uri"`
	Protocol string
	Encoding []string
	// HTTP Headers
	Headers   map[string]string
	HeaderLen int
	Body      []byte
	BodyLen   int
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

func ReqInit() *Req {
	return &Req{
		Headers: make(map[string]string),
	}
}

// parse Host
func (r *Req) ParseHost(lis_info listener.Listener) {
	if r.Headers["Host"] == "" {
		return
	}
	if lis_info.Port == "80" {
		r.Headers["Host"] = r.Headers["Host"] + ":80"
	} else if lis_info.Port == "443" {
		r.Headers["Host"] = r.Headers["Host"] + ":443"
	}
}

// reset request's header
func (r *Req) SetHeader(key string, val string, cfg listener.ListenCfg) {

	r.Headers[key] = val

	_, ref := r.Headers["Referer"]
	if ref {
		ori := r.Headers["Referer"]
		after := strings.Replace(ori, cfg.ServerName, r.Headers["Host"], -1)
		r.Headers["Referer"] = after
	}

	_, ori := r.Headers["Origin"]
	if ori {
		if cfg.Type == 1 {
			r.Headers["Origin"] = "http://" + cfg.ProxyAddr
		} else if cfg.Type == 2 {
			r.Headers["Origin"] = "https://" + cfg.ProxyAddr
		} else {
			fmt.Println("SET header error...")
		}
	}
}

// flush request struct
func (r *Req) Flush() {}

// get request header
func (r *Req) GetHeader(key string) string {
	return r.Headers[key]
}

// whether the request connection is keep alive
func (r *Req) IsKeepalive() bool {
	conn := r.GetHeader("Connection")
	if conn == "keep-alive" {
		return true
	} else {
		return false
	}
}

// get request row bytes
func (r *Req) ByteRow() []byte {
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
func (r *Req) ParseHeader(request_byte []byte) int {
	request := string(request_byte)
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

	lines := requestLine[1:]
	if len(lines) == 1 {
		return REQUEST_NEED_READ_MORE
	}

	for i := 0; i < len(lines); i++ {
		if lines[i] == "" && len(lines) > i+1 { // there is "\r\n\r\n", \r\n"
			return REQUEST_OK // valid
		}
		parts := strings.SplitN(lines[i], ":", 2)
		if len(parts) == 1 { // No ":"
			return INVALID_HEADERS // invalid headers
		}
		key := strings.TrimSpace(parts[0])
		// key = strings.ToTitle(key)
		key = strings.ToUpper(key[:1]) + key[1:]
		value := strings.TrimSpace(parts[1])
		r.Headers[key] = value
	}

	return REQUEST_NEED_READ_MORE // valid
}

func (r *Req) RequestHeaderValid() bool {
	return true
}

func (r *Req) TryFixHeader(other []byte) error {
	return nil
}

// get request's body
func (r *Req) ParseBody(tmpByte []byte) {
	var i int // last byte position before \r\n\r\n
	var remain_len int
	var res []byte

	total_len := len(tmpByte)
	for i = 0; i < total_len-4; i++ {
		if tmpByte[i] == byte(13) && tmpByte[i+1] == byte(10) &&
			tmpByte[i+2] == byte(13) && tmpByte[i+3] == byte(10) {
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

func (r *Req) RequestBodyValid() bool {
	contentType := r.GetHeader("Content-Type")
	if strings.Index(contentType, "multipart/form-data") != -1 {
		po := strings.Index(contentType, "boundary=")
		boundaryStr := contentType[po+len("boundary="):]
		if strings.Index(string(r.Body), boundaryStr+"--") != -1 {
			return true
		} else {
			return false
		}
	}

	contentLength := r.GetHeader("Content-Length")
	if contentLength != "" {
		n, err := strconv.Atoi(contentLength)
		if err != nil {
			panic(err)
		}
		if len(r.Body) != n { // content length not equal to body length
			return false
		}
	}

	return true
}

func (r *Req) TryFixBody(other []byte) bool {
	r.Body = append(r.Body, other...)
	if !r.RequestBodyValid() {
		return false
	} else {
		return true
	}
}
