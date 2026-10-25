package request

import (
	"bytes"
	"errors"
	"fast-https/runtime/listener"
	"fmt"
	"net/textproto"
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
var crlf = []byte("\r\n")

// methodSet/protocolSet：map 的 string(b) 查找不产生分配，
// 命中即复用常量串——同时完成方法/协议的合法性校验。
var (
	methodSet   = make(map[string]string, len(http_method))
	protocolSet = make(map[string]string, len(http_protocol))
)

// commonCanonKeys 预置常见头名的 canonical 形式：
// 命中即零分配复用常量串，未命中回落到 textproto（每键一次分配）。
var commonCanonKeys = func() map[string]string {
	names := []string{
		"Accept", "Accept-Encoding", "Accept-Language", "Authorization",
		"Cache-Control", "Connection", "Content-Encoding", "Content-Length",
		"Content-Type", "Cookie", "Date", "Etag", "Expect", "Host",
		"If-Modified-Since", "If-None-Match", "Last-Modified", "Location",
		"Origin", "Range", "Referer", "Server", "Set-Cookie", "Te",
		"Transfer-Encoding", "Upgrade", "User-Agent", "Vary",
		"Www-Authenticate", "X-Forwarded-For", "X-Forwarded-Host",
		"X-Forwarded-Proto", "X-Real-Ip", "X-Request-Id",
	}
	m := make(map[string]string, len(names))
	for _, n := range names {
		m[n] = n
	}
	return m
}()

func init() {
	for _, m := range http_method {
		methodSet[m] = m
	}
	for _, p := range http_protocol {
		protocolSet[p] = p
	}
}

// canonHeaderKey 返回头名的 canonical 形式。
func canonHeaderKey(b []byte) string {
	if s, ok := commonCanonKeys[string(b)]; ok {
		return s
	}
	return textproto.CanonicalMIMEHeaderKey(string(b))
}

// errNoColon 表示头行没有冒号，由调用方根据它是否为最后一段决定
// 返回 InvalidHeaders 还是 NeedReadMore。
var errNoColon = errors.New("header line without colon")

func (r *Request) parseHeaderLine(line []byte) error {
	colon := bytes.IndexByte(line, ':')
	if colon < 0 {
		return errNoColon
	}
	key := line[:colon]
	// 头名两侧不允许空白："Name : v" 是 RFC 7230 §3.2.4 明确要求
	// 拒绝的走私混淆形式；行首空白（obs-fold 折行残迹）同样拒绝。
	if k := bytes.TrimSpace(key); len(k) != len(key) || len(k) == 0 {
		return InvalidHeaders
	}
	// HTTP 头名大小写不敏感，统一成 canonical 形式供后续读取
	ks := canonHeaderKey(key)
	value := bytes.TrimSpace(line[colon+1:])
	// 头名与头值都不允许出现裸 CR/LF：正常解析流程按 \r\n 切行不会
	// 留下这两个字符，但值中间的裸 \n（如 "a\nEvil: x"）若被放行，
	// 会在 ByteRow 重建报文时原样写出——构成请求头注入/走私。
	if bytes.ContainsAny(key, "\r\n") || bytes.ContainsAny(value, "\r\n") {
		return InvalidHeaders
	}
	if ks == "Content-Length" {
		if n, err := strconv.Atoi(string(value)); err != nil || n < 0 {
			return InvalidHeaders
		}
	}
	r.Headers[ks] = string(value)
	return nil
}

// parse row tcp str to a req object
// 头未接收完整时返回 RequestNeedReadMore，调用方应继续读 socket 后重新解析；
// 头非法时返回对应 *RequestError，调用方应拒绝该请求。
//
// 全程在原始字节切片上切行，只为最终存入 map 的键值分配字符串，
// 不再整包 string() 拷贝、不再为每行分配中间字符串。
func (r *Request) ParseHeader(request_byte []byte) error {
	if len(request_byte) == 0 {
		return None
	}

	// RFC 7230 §3.5：请求行之前应忽略空行（部分客户端以 CRLF 开场）
	for bytes.HasPrefix(request_byte, crlf) {
		request_byte = request_byte[2:]
	}
	if len(request_byte) == 0 {
		return RequestNeedReadMore // 目前只收到开场空行，请求行还没到
	}

	// 请求行
	lineEnd := bytes.Index(request_byte, crlf)
	if lineEnd < 0 {
		// 缓冲里还没有任何完整 CRLF。结尾孤立的 \r 可能是跨 TCP 段
		// 到达的半个 CRLF，剥掉后再看：仍残留 CR/LF 说明对端在用
		// LF 或孤立 CR 分行（走私变体），立即拒绝；否则就是请求行
		// 尚未读全，交回调用方继续读（头分片到达的正常路径）。
		buf := request_byte
		if buf[len(buf)-1] == '\r' {
			buf = buf[:len(buf)-1]
		}
		if bytes.ContainsAny(buf, "\r\n") {
			return UnknowInvalid
		}
		return RequestNeedReadMore
	}
	first := request_byte[:lineEnd]
	rest := request_byte[lineEnd+2:]

	sp1 := bytes.IndexByte(first, ' ')
	if sp1 < 0 {
		return FirstLineInvalid // invalid first line
	}
	sp2 := bytes.IndexByte(first[sp1+1:], ' ')
	if sp2 < 0 {
		return FirstLineInvalid
	}
	sp2 += sp1 + 1
	path := first[sp1+1 : sp2]
	protocol := first[sp2+1:]
	if bytes.IndexByte(protocol, ' ') >= 0 {
		// 首行字段数超过 3
		return FirstLineInvalid
	}
	// 请求行任何字段都不允许出现裸 CR/LF：路径里的 "\n" 若被放行，
	// 会在 ByteRow 重建请求时把第二行拆出来（请求走私）。
	if bytes.ContainsAny(first, "\r\n") {
		return FirstLineInvalid
	}

	if m, ok := methodSet[string(first[:sp1])]; ok {
		r.Method = m
	} else {
		return MethodInvalid // invalid method
	}

	if len(path) == 0 {
		return PathInvalid
	}
	r.Path = string(path)

	if p, ok := protocolSet[string(protocol)]; ok {
		r.Protocol = p
	} else {
		return ProtocolInvalid // invalid protocol
	}

	if len(rest) == 0 {
		// 整个缓冲只有请求行（带或不带结尾 CRLF）
		return RequestNeedReadMore
	}

	// 逐行解析头部，行是原缓冲的切片视图
	pos := 0
	for {
		nl := bytes.Index(rest[pos:], crlf)
		if nl < 0 {
			// 最后一段（无 CRLF 结尾）：可能是被截断的半个头
			if err := r.parseHeaderLine(rest[pos:]); err != nil {
				if err == errNoColon {
					return RequestNeedReadMore
				}
				return err
			}
			return RequestNeedReadMore
		}
		line := rest[pos : pos+nl]
		pos += nl + 2
		if len(line) == 0 {
			// 终止空行：后面是 body
			return RequestOk
		}
		if err := r.parseHeaderLine(line); err != nil {
			if err == errNoColon {
				return InvalidHeaders
			}
			return err
		}
	}
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
