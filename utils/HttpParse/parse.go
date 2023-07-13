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
}

var data = "GET /config HTTP/1.1\r\nAccept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7\r\nAccept-Encoding: gzip, deflate, br\r\nConnection: keep-alive\r\nHost: gitee.com\r\n"

var R_rea Req

func Process_HttpParse() {

	re := regexp.MustCompile(`^(\w+)\s+([^ ]+)\s+HTTP/1.1\r\n.*\r\nAccept-Encoding:\s*([^\r\n]+)\r\n.*\r\nHost:\s*([^\r\n]+)\r\n`)
	matches := re.FindStringSubmatch(data)

	// 将 Accept-Encoding 字段的值分割成数组
	encodings := strings.Split(matches[3], ", ")
	var encoding []string
	for _, e := range encodings {
		encoding = append(encoding, strings.TrimSpace(e))
	}

	// 将解析结果存储到 Req 结构体中
	if len(matches) > 4 {
		req := Req{
			Method:   matches[1],
			Path:     matches[2],
			Encoding: encoding,
			Host:     matches[4],
		}
		fmt.Printf("%+v\n", req)
	} else {
		fmt.Println("解析失败")
	}

}
