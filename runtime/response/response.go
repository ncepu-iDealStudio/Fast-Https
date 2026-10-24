// Package response 负责构建并序列化 HTTP 响应。
package response

import (
	"fmt"
	"net/textproto"
	"sort"
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
	r.FirstLine = fmt.Sprintf("HTTP/1.1 %d %s", statusCode, statusText)
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

// Generate a response data (bytes)
// attention: this function must return bytes, not str
// once response contain '\0', it will doesn't work
func (r *Response) GenerateResponse() []byte {
	var res []byte
	response := r.FirstLine + HTTP_SPLIT
	for _, key := range sortedHeaderKeys(r.Headers) {
		response += fmt.Sprintf("%s: %s\r\n", key, r.Headers[key])
	}
	res = append(res, []byte(response)...)
	res = append(res, []byte("\r\n")...)
	res = append(res, r.Body...)

	return res
}

func (r *Response) GenerateHeaderBytes() []byte {
	var res []byte
	response := r.FirstLine + HTTP_SPLIT
	for _, key := range sortedHeaderKeys(r.Headers) {
		response += fmt.Sprintf("%s: %s\r\n", key, r.Headers[key])
	}
	res = append(res, []byte(response)...)
	res = append(res, []byte("\r\n")...)

	return res
}

// HttpResParse 解析上游响应。非法头返回对应 *ResponError；已解析出的头保留，
// 缺少终止空行时 Body 保持原值。头名统一 canonical 化供后续读取。
func (r *Response) HttpResParse(request string) error {

	if request == "" {
		return None
	}
	requestLine := strings.Split(request, "\r\n")
	parts := strings.Split(requestLine[0], " ")
	if len(parts) < 3 {
		return FirstLineInvalid // invalid first line
	}
	r.FirstLine = parts[0] + " " + parts[1] + " " + parts[2]

	for _, line := range requestLine[1:] {
		if line == "" {
			break
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) == 1 { // 无 ":" 的行不合法
			return InvalidHeaders
		}
		key := strings.TrimSpace(kv[0])
		if key == "" { // ": value" 这样的行不合法
			return InvalidHeaders
		}
		key = textproto.CanonicalMIMEHeaderKey(key)
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
