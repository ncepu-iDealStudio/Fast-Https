package response

import (
	"fmt"
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
	FirstLineInvalid = &ResponError{3, "First line invalid"}
	MethodInvalid    = &ResponError{4, "Method invalid"}
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
	for key, value := range r.Headers {
		response += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	res = append(res, []byte(response)...)
	res = append(res, []byte("\r\n")...)
	res = append(res, r.Body...)

	return res
}

func (r *Response) GenerateHeaderBytes() []byte {
	var res []byte
	response := r.FirstLine + HTTP_SPLIT
	for key, value := range r.Headers {
		response += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	res = append(res, []byte(response)...)
	res = append(res, []byte("\r\n")...)

	return res
}

func (r *Response) HttpResParse(request string) error {

	if request == "" {
		return None
	}
	requestLine := strings.Split(request, "\r\n")
	if requestLine == nil {
		return UnknowInvalid // invalid request
	}
	parts := strings.Split(requestLine[0], " ")
	if parts == nil || len(parts) < 3 {
		return FirstLineInvalid // invalid first line
	}
	r.FirstLine = parts[0] + " " + parts[1] + " " + parts[2]

	lines := strings.Split(request, "\r\n")[1:]
	for _, line := range lines {
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		key = strings.ToUpper(key[:1]) + key[1:]
		value := strings.TrimSpace(parts[1])
		r.Headers[key] = value
	}
	i := strings.Index(string(request), "\r\n\r\n")
	r.Body = []byte(request)[i+4:]
	return ResponseOk // valid
}

// get request header
func (r *Response) GetHeader(key string) string {
	return r.Headers[key]
}

func (r *Response) DelHeader(key string) {
	delete(r.Headers, key)
}

func Test() {
	response := ResponseInit()

	response.SetFirstLine(200, "OK")
	response.SetHeader("Content-Type", "text/html")
	response.SetBody([]byte("<h1>Hello, World!</h1>"))

	Response := response.GenerateResponse()
	fmt.Println(Response)
}
