package response

import (
	"fmt"
)

// every event will return a Response object
// except tcp proxy
type Response struct {
	firstLine string
	headers   map[string]string
	body      []byte
}

func ResponseInit() *Response {
	return &Response{
		headers: make(map[string]string),
	}
}

func (r *Response) SetFirstLine(statusCode int, statusText string) {
	r.firstLine = fmt.Sprintf("HTTP/1.1 %d %s", statusCode, statusText)
}

func (r *Response) SetHeader(key, value string) {
	r.headers[key] = value
}

func (r *Response) SetBody(body []byte) {
	r.body = body
}

// Generate a response data (bytes)
// attention: this function must return bytes, not str
// once response contain '\0', it will doesn't work
func (r *Response) GenerateResponse() []byte {
	var res []byte
	response := r.firstLine + HTTP_SPLIT
	for key, value := range r.headers {
		response += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	res = append(res, []byte(response)...)
	res = append(res, []byte("\r\n")...)
	res = append(res, r.body...)

	return res
}

func Test() {
	response := ResponseInit()

	response.SetFirstLine(200, "OK")
	response.SetHeader("Content-Type", "text/html")
	response.SetBody([]byte("<h1>Hello, World!</h1>"))

	Response := response.GenerateResponse()
	fmt.Println(Response)
}
