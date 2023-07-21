package request

import (
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

type Req struct {
	Method   string
	Path     string
	Encoding []string
	Host     string
	Protocol string
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

func Http_parse(request string) (Req, int) {

	if request == "" {
		return Req{}, NONE
	}
	// fmt.Printf(request)
	requestLine := strings.Split(request, "\r\n")
	if requestLine == nil {
		return Req{}, UNKNOW_INVALID // invlaid request
	}
	parts := strings.Split(requestLine[0], " ")
	if parts == nil || len(parts) < 3 {
		return Req{}, FIRST_LINE_INVALID // invlaid first line
	}
	method := parts[0]
	if !collection.Collect(http_method).Contains(method) {
		return Req{}, METHOD_INVALID // invlaid method
	}
	path := parts[1]
	protocol := parts[2]

	var host string

	headers := make(map[string]string)
	lines := strings.Split(request, "\r\n")[1:]
	for _, line := range lines {
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		headers[key] = value
		if strings.Compare(key, "Host") == 0 {
			host = value
		}
	}

	// fmt.Println("Headers:", headers)
	encoding := []string{"gzip"}

	return Req{
		Method:   method,
		Path:     path,
		Encoding: encoding,
		Host:     host,
		Protocol: protocol,
	}, REQUEST_OK
}
