// Package response 负责构建并序列化 HTTP 响应。
package response

import (
	"fmt"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
)

// const (
// 	RESPONSE_OK        = 0
// 	NONE               = 1
// 	UNKNOW_INVALID     = 2
// 	FIRST_LINE_INVALID = 3
// 	METHOD_INVALID     = 4
// )

type ResponError struct {
	Code    int
	Message string
}

var (
	ResponseOk       = &ResponError{0, "Response OK"}
	None             = &ResponError{1, "None"}
	UnknowInvalid    = &ResponError{2, "Unknow invalid"}
	FirstLineInvalid = &ResponError{3, "Invalid first line"}
	MethodInvalid    = &ResponError{4, "Method invalid"}
	InvalidHeaders   = &ResponError{6, "Invalid headers"}
)

func (e *ResponError) Error() string {
	return fmt.Sprintf("Respon error code: %d, Message: %s", e.Code, e.Message)
}

// every event will return a Response object
// except tcp proxy
type Response struct {
	FirstLine string
	Headers   map[string]string
	Body      []byte
}

func ResponseInit() *Response {
	return &Response{
		Headers: make(map[string]string),
	}
}

func (r *Response) SetFirstLine(statusCode int, statusText string) {
	r.FirstLine = "HTTP/1.1 " + strconv.Itoa(statusCode) + " " + statusText
}

func (r *Response) SetHeader(key, value string) {
	r.Headers[key] = value
}

func (r *Response) SetBody(body []byte) {
	r.Body = body
}

func (r *Response) GetBody() []byte {
	return r.Body
}

// generateHeaders 把状态行与头部追加到 dst，单一预分配路径，
// 替代此前 Sprintf + += 的多次中间分配。
func (r *Response) generateHeaders(dst []byte) []byte {
	dst = append(dst, r.FirstLine...)
	dst = append(dst, HTTP_SPLIT...)
	for _, key := range sortedHeaderKeys(r.Headers) {
		dst = append(dst, key...)
		dst = append(dst, ": "...)
		dst = append(dst, r.Headers[key]...)
		dst = append(dst, HTTP_SPLIT...)
	}
	dst = append(dst, HTTP_SPLIT...)
	return dst
}

// Generate a response data (bytes)
// attention: this function must return bytes, not str
// once response contain '\0', it will doesn't work
func (r *Response) GenerateResponse() []byte {
	dst := r.generateHeaders(make([]byte, 0, len(r.FirstLine)+64*len(r.Headers)+len(r.Body)+8))
	return append(dst, r.Body...)
}

func (r *Response) GenerateHeaderBytes() []byte {
	return r.generateHeaders(make([]byte, 0, len(r.FirstLine)+64*len(r.Headers)+8))
}

// HttpResParse 解析上游响应。非法头返回对应 *ResponError；已解析出的头保留，
// 缺少终止空行时 Body 保持原值。头名统一 canonical 化供后续读取。
func (r *Response) HttpResParse(request string) error {

	if request == "" {
		return None
	}
	requestLine := strings.Split(request, "\r\n")
	first := requestLine[0]
	// 上游用裸 LF 分行时整个"首行"里就带着 \n：若放行会在序列化时
	// 原样写给客户端（响应拆分），直接拒绝。
	if strings.ContainsAny(first, "\r\n") {
		return FirstLineInvalid
	}
	parts := strings.Split(first, " ")
	if len(parts) < 2 {
		return FirstLineInvalid // invalid first line
	}
	reason := ""
	if len(parts) > 2 {
		// reason 短语可以含空格（如 "Service Unavailable"），必须整体保留
		reason = strings.Join(parts[2:], " ")
	}
	r.FirstLine = parts[0] + " " + parts[1] + " " + reason

	for _, line := range requestLine[1:] {
		if line == "" {
			break
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) == 1 { // 无 ":" 的行不合法
			return InvalidHeaders
		}
		// 头名两侧不允许空白（RFC 7230 §3.2.4；obs-fold 折行残迹同理），
		// 该约束对上游响应与对客户端请求一致。
		if k := strings.TrimSpace(kv[0]); len(k) != len(kv[0]) || len(k) == 0 {
			return InvalidHeaders
		}
		// 上游是不可信输入：头值含裸 CR/LF 时若放行，会在响应序列化
		// 时原样写给客户端——构成响应头注入。拒绝。
		if strings.ContainsAny(kv[0], "\r\n") || strings.ContainsAny(kv[1], "\r\n") {
			return InvalidHeaders
		}
		key := textproto.CanonicalMIMEHeaderKey(kv[0])
		value := strings.TrimSpace(kv[1])
		r.Headers[key] = value
	}
	if i := strings.Index(request, "\r\n\r\n"); i != -1 {
		r.Body = []byte(request[i+4:])
	}
	return ResponseOk // valid
}

// get request header
func (r *Response) GetHeader(key string) string {
	return r.Headers[key]
}

func (r *Response) DelHeader(key string) {
	delete(r.Headers, key)
}

func sortedHeaderKeys(headers map[string]string) []string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
