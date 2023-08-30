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

	Host       string
	Encoding   []string
	Connection string // <keep-alive> <close>

	Headers map[string]string
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
		r.Host = r.Host + ":80"
	} else if lis_info.Port == "443" {
		r.Host = r.Host + ":443"
	}
}

// reset request's headers
func (r *Req) Set_headers(key string, val string) {
	if key == "Host" {
		r.Host = val
	}
	if key == "Connection" {
		r.Connection = val
	}
	r.Headers[key] = val
	r.Headers["Connection"] = "close"
}

// flush request struct
func (r *Req) Flush() {

}

func (r *Req) Byte_row() []byte {
	rowStr := r.Method + " " +
		r.Path + " " +
		r.Protocol + "\r\n"
	for k, v := range r.Headers {
		rowStr = rowStr + k + ": " + v + "\r\n"
	}
	rowStr = rowStr + "\r\n"
	return []byte(rowStr)
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
		if strings.Compare(key, "Host") == 0 {
			r.Host = value
		}
		if strings.Compare(key, "Connection") == 0 {
			r.Connection = value
		}
	}

	// fmt.Println(r)

	return REQUEST_OK // valid
}
