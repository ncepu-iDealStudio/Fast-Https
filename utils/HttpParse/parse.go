package httpparse

import (
	"fmt"
	"regexp"
	"strings"
)

type Req struct {
	Method   string
	Path     string
	Encoding []string
	Host     string
	Protocol string
}

// there are some bugs 7.13 20:00
func Process_HttpParse(data string) Req {

	re := regexp.MustCompile(`^(\w+)\s+([^ ]+)\s+HTTP/1.1\r\n.*\r\nAccept-Encoding:\s*([^\r\n]+)\r\n.*\r\nHost:\s*([^\r\n]+)\r\n`)
	matches := re.FindStringSubmatch(data)

	// 将 Accept-Encoding 字段的值分割成数组

	var encoding []string
	if len(matches) >= 3 {
		encodings := strings.Split(matches[3], ", ")
		fmt.Println(data)
		for _, e := range encodings {
			encoding = append(encoding, strings.TrimSpace(e))
		}
	}

	// 将解析结果存储到 Req 结构体中
	if len(matches) > 4 {
		req := Req{
			Method:   matches[1],
			Path:     matches[2],
			Encoding: encoding,
			Host:     matches[4],
		}
		return req

	} else {
		fmt.Println("Parse error")
		return Req{}
	}
}

func HttpParse2(request string) (Req, int) {

	if request == "" {
		return Req{}, 10
	}
	requestLine := strings.Split(request, "\r\n")[0]
	parts := strings.Split(requestLine, " ")
	method := parts[0]
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
	}, 0
}
