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

func Response_init() *Response {
	return &Response{
		headers: make(map[string]string),
	}
}

func (r *Response) Set_first_line(statusCode int, statusText string) {
	r.firstLine = fmt.Sprintf("HTTP/1.1 %d %s", statusCode, statusText)
}

func (r *Response) Set_header(key, value string) {
	r.headers[key] = value
}

func (r *Response) Set_body(body []byte) {
	r.body = body
}

// Generate a response data (bytes)
// attention: this function must return bytes, not str
// once response contain '\0', it will doesn't work
func (r *Response) Generate_response() []byte {
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
	response := Response_init()

	response.Set_first_line(200, "OK")
	response.Set_header("Content-Type", "text/html")
	response.Set_body([]byte("<h1>Hello, World!</h1>"))

	Response := response.Generate_response()
	fmt.Println(Response)
}
