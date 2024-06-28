package request

import (
	"bytes"
	"errors"
	"fast-https/modules/core/listener"
	"fmt"
	"strconv"
	"strings"

	"github.com/chenhg5/collection"
)

// const (
// 	REQUEST_OK             = 0
// 	NONE                   = 1
// 	UNKNOW_INVALID         = 2
// 	FIRST_LINE_INVALID     = 3
// 	METHOD_INVALID         = 4
// 	REQUEST_NEED_READ_MORE = 5

// 	INVALID_HEADERS = 6
// )

type RequestError struct {
	Code    int
	Message string
}

var (
	RequestOk           = &RequestError{0, "Request OK"}
	None                = &RequestError{1, "None"}
	UnknowInvalid       = &RequestError{2, "Unknow invalid"}
	FirstLineInvalid    = &RequestError{3, "First line invalid"}
	MethodInvalid       = &RequestError{4, "Method invalid"}
	RequestNeedReadMore = &RequestError{5, "Request need read more"}
	InvalidHeaders      = &RequestError{6, "Invalid headers"}
	ProtocolInvalid     = &RequestError{7, "Protocol invalid"}
	PathInvalid         = &RequestError{8, "Path invalid"}
)

func (e *RequestError) Error() string {
	return fmt.Sprintf("Request error code: %d, Message: %s", e.Code, e.Message)
}

// this struct is saved in Event
// which contaions event's method,path,servername(headers)
type Request struct {
	// HTTP first line
	Method   string
	Path     string
	Query    PathQuery
	Protocol string
	Encoding []string
	// HTTP Headers
	Headers   map[string]string
	HeaderLen int
	Body      bytes.Buffer
	BodyLen   int
	H2        bool
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

var http_protocol = []string{
	"HTTP/0.9",
	"HTTP/1.0",
	"HTTP/1.1",
	"HTTP/2",
	"HTTP/3",
}

func RequestInit(h2 bool) *Request {
	return &Request{
		Headers: make(map[string]string),
		Query:   make(map[string]string),
		H2:      h2,
	}
}

func (r *Request) parseQueryParams() {
	queryIndex := strings.Index(r.Path, "?")
	if queryIndex == -1 {
		return
	}

	queryStr := r.Path[queryIndex+1:]
	params := strings.Split(queryStr, "&")

	for _, param := range params {

		valIndex := strings.Index(param, "=")
		if valIndex != -1 {
			key := param[:valIndex]
			valStr := param[valIndex+1:]
			r.Query[key] = valStr
		}

	}

	//r.Query["44"] = `<a onblur="alert(secret)" href="http://www.google.com">Google</a>`

}

// parse Host
func (r *Request) ParseHost(lis_info *listener.Listener) {
	if r.Headers["Host"] == "" {
		return
	}
	if lis_info.Port == "80" {
		r.Headers["Host"] = r.Headers["Host"] + ":80"
	} else if lis_info.Port == "443" {
		r.Headers["Host"] = r.Headers["Host"] + ":443"
	}

	r.parseQueryParams() // temp put here
}

// reset request's header
func (r *Request) SetHeader(key string, val string, cfg *listener.ListenCfg) {

	r.Headers[key] = val

	// _, ref := r.Headers["Referer"]
	// if ref {
	// 	ori := r.Headers["Referer"]
	// 	after := strings.Replace(ori, cfg.ServerName, r.Headers["Host"], -1)
	// 	r.Headers["Referer"] = after
	// }

	// _, ori := r.Headers["Origin"]
	// if ori {
	// 	if cfg.Type == 1 {
	// 		r.Headers["Origin"] = "http://" + cfg.ProxyAddr
	// 	} else if cfg.Type == 2 {
	// 		r.Headers["Origin"] = "https://" + cfg.ProxyAddr
	// 	} else {
	// 		fmt.Println("SET header error...")
	// 	}
	// }
}

// flush request struct
func (r *Request) Flush() {
	r.Body.Reset()
	for k := range r.Headers {
		delete(r.Headers, k)
	}
}

// get request header
func (r *Request) GetHeader(key string) string {
	return r.Headers[key]
}

func (r *Request) GetHost() string {
	return r.Headers["Host"]
}

func (r *Request) GetConnection() string {
	return r.Headers["Connection"]
}

func (r *Request) GetContentType() string {
	return r.Headers["Content-Type"]
}

func (r *Request) GetContentLength() string {
	return r.Headers["Content-Length"]
}

func (r *Request) GetUpgrade() string {
	return r.Headers["Upgrade"]
}

func (r *Request) GetTransferEncoding() string {
	return r.Headers["Transfer-Encoding"]
}

func (r *Request) GetAuthorization() string {
	return r.Headers["Authorization"]
}

// whether the request connection is keep alive
func (r *Request) IsKeepalive() bool {
	conn := r.GetConnection()
	if conn == "keep-alive" {
		return true
	} else {
		return false
	}
}

// get request row bytes
func (r *Request) ByteRow() []byte {
	rowStr := r.Method + " " +
		r.Path + " " +
		r.Protocol + "\r\n"
	for k, v := range r.Headers {
		rowStr = rowStr + k + ": " + v + "\r\n"
	}
	rowStr = rowStr + "\r\n"

	rowByte := []byte(rowStr)
	rowByte = append(rowByte, r.Body.Bytes()...)

	return rowByte
}

// parse row tcp str to a req object
func (r *Request) ParseHeader(request_byte []byte) error {
	request := string(request_byte)
	if request == "" {
		return None
	}
	requestLine := strings.Split(request, "\r\n")
	if len(requestLine) < 2 {
		return UnknowInvalid // invalid request
	}
	parts := strings.Split(requestLine[0], " ")
	if len(parts) != 3 {
		return FirstLineInvalid // invalid first line
	}
	if !collection.Collect(http_method).Contains(parts[0]) {
		return MethodInvalid // invalid method
	}
	r.Method = parts[0]

	if parts[1] == "" {
		return PathInvalid
	}
	r.Path = parts[1]

	if !collection.Collect(http_protocol).Contains(parts[2]) {
		return ProtocolInvalid // invalid protocol
	}
	r.Protocol = parts[2]

	lines := requestLine[1:]
	if len(lines) == 1 {
		return RequestNeedReadMore
	}

	for i := 0; i < len(lines); i++ {
		if lines[i] == "" && len(lines) > i+1 { // there is "\r\n\r\n", \r\n"
			return RequestOk // valid
		}
		parts := strings.SplitN(lines[i], ":", 2)
		if len(parts) == 1 { // No ":"
			return InvalidHeaders // invalid headers
		}
		key := strings.TrimSpace(parts[0])
		// key = strings.ToTitle(key)
		key = strings.ToUpper(key[:1]) + key[1:]
		value := strings.TrimSpace(parts[1])
		r.Headers[key] = value
	}

	return RequestNeedReadMore // valid
}

func (r *Request) RequestHeaderValid() bool {
	return true
}

func (r *Request) TryFixHeader(other []byte) error {
	return nil
}

// get request's body
func (r *Request) ParseBody(tmpByte []byte) error {

	flag := false
	total_len := len(tmpByte)
	// last byte position before \r\n\r\n
	i := strings.Index(string(tmpByte), "\r\n\r\n")
	if i != -1 {
		flag = true
	}
	if !flag {
		return errors.New("parse body error")
	}

	remain_len := total_len - (i + 4)
	if remain_len == 0 {
		res := tmpByte[i+4:]
		r.Body.Write(res)
	} else {
		res := tmpByte[i+4:][:remain_len]
		r.Body.Write(res)
	}

	return nil
}

func (r *Request) RequestBodyValid() bool {
	contentType := r.GetContentType()
	if strings.Contains(contentType, "multipart/form-data") {
		po := strings.Index(contentType, "boundary=")
		boundaryStr := contentType[po+len("boundary="):]
		if strings.Contains(r.Body.String(), boundaryStr+"--") {
			return true
		} else {
			return false
		}
	}

	contentLength := r.GetContentLength()
	if contentLength != "" {
		n, err := strconv.Atoi(contentLength)
		if err != nil {
			panic(err)
		}
		if r.Body.Len() != n { // content length not equal to body length
			return false
		}
	}

	return true
}

func (r *Request) TryFixBody(other []byte) {
	// r.Body = append(r.Body, other...)
	r.Body.Write(other)
}
