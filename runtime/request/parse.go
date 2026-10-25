package request

import (
	"bytes"
	"errors"
	"fast-https/runtime/listener"
	"fmt"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
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

	// multipartScanned 记录 multipart 判结束时已经扫过的 Body 前缀长度。
	// 每轮读只扫新增部分，避免大上传每轮全量重扫的 O(n²)。
	multipartScanned int
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
	switch lis_info.Port {
	case "80":
		r.Headers["Host"] = r.Headers["Host"] + ":80"
	case "443":
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
	r.multipartScanned = 0
	for k := range r.Headers {
		delete(r.Headers, k)
	}
	// keep-alive 复用请求对象时，上一个请求的 query 不能残留到下一个请求
	for k := range r.Query {
		delete(r.Query, k)
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
	keepAlive := false
	for _, token := range strings.Split(strings.ToLower(r.GetConnection()), ",") {
		switch strings.TrimSpace(token) {
		case "close":
			return false
		case "keep-alive":
			keepAlive = true
		}
	}
	if keepAlive {
		return true
	}
	return r.Protocol == "HTTP/1.1" || r.Protocol == "HTTP/2" || r.Protocol == "HTTP/3"
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
// 头未接收完整时返回 RequestNeedReadMore，调用方应继续读 socket 后重新解析；
// 头非法时返回对应 *RequestError，调用方应拒绝该请求。
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
	if !slices.Contains(http_method, parts[0]) {
		return MethodInvalid // invalid method
	}
	r.Method = parts[0]

	if parts[1] == "" {
		return PathInvalid
	}
	r.Path = parts[1]

	if !slices.Contains(http_protocol, parts[2]) {
		return ProtocolInvalid // invalid protocol
	}
	r.Protocol = parts[2]

	lines := requestLine[1:]
	if len(lines) == 1 {
		return RequestNeedReadMore
	}

	for i := 0; i < len(lines); i++ {
		if lines[i] == "" { // there is "\r\n\r\n", \r\n"
			if i+1 < len(lines) {
				return RequestOk // valid
			}
			return RequestNeedReadMore // 缓冲以空行结尾，头可能还没发完
		}
		parts := strings.SplitN(lines[i], ":", 2)
		if len(parts) == 1 { // No ":"
			if i+1 == len(lines) {
				return RequestNeedReadMore // 最后一行可能是被截断的半个头
			}
			return InvalidHeaders // invalid headers
		}
		key := strings.TrimSpace(parts[0])
		if key == "" { // ": value" 这样的行不合法
			return InvalidHeaders
		}
		// HTTP 头名大小写不敏感，统一成 canonical 形式供后续读取
		key = textproto.CanonicalMIMEHeaderKey(key)
		value := strings.TrimSpace(parts[1])
		if key == "Content-Length" {
			if n, err := strconv.Atoi(value); err != nil || n < 0 {
				return InvalidHeaders
			}
		}
		r.Headers[key] = value
	}

	return RequestNeedReadMore // valid
}

// get request's body
func (r *Request) ParseBody(tmpByte []byte) error {
	i := bytes.Index(tmpByte, []byte("\r\n\r\n"))
	if i == -1 {
		return errors.New("parse body error")
	}
	r.Body.Write(tmpByte[i+4:])
	return nil
}

func (r *Request) RequestBodyValid() bool {
	contentType := r.GetContentType()
	if strings.Contains(contentType, "multipart/form-data") {
		po := strings.Index(contentType, "boundary=")
		boundaryStr := contentType[po+len("boundary="):]
		// 从上次扫到的位置继续找终止边界：分块到达的大 body
		// 不能每轮都全量重扫。保留 needle 长度的重叠区，
		// 防止边界跨读块被截断
		body := r.Body.Bytes()
		if r.multipartScanned > len(body) {
			r.multipartScanned = 0
		}
		if bytes.Contains(body[r.multipartScanned:], []byte(boundaryStr+"--")) {
			r.multipartScanned = 0
			return true
		}
		r.multipartScanned = len(body) - len(boundaryStr)
		if r.multipartScanned < 0 {
			r.multipartScanned = 0
		}
		return false
	}

	contentLength := r.GetContentLength()
	if contentLength != "" {
		n, err := strconv.Atoi(contentLength)
		if err != nil {
			// ParseHeader 已拒绝非法 Content-Length，这里兜底不 panic
			return false
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
