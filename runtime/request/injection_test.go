package request

import (
	"bytes"
	"strings"
	"testing"
)

// 头值含裸 LF 会被拒绝：否则 ByteRow 重建报文时原样写出第二行，
// 构成请求头注入/走私（配合代理转发给上游时尤其危险）。
func TestParseHeaderRejectsBareLFInHeaderValue(t *testing.T) {
	req := RequestInit(false)
	err := req.ParseHeader([]byte("GET / HTTP/1.1\r\nHost: a\nEvil: injected\r\n\r\n"))
	if err != InvalidHeaders {
		t.Fatalf("bare LF in header value got %v, want InvalidHeaders", err)
	}
}

// 头名含裸 CR 同样拒绝
func TestParseHeaderRejectsBareCRInHeaderName(t *testing.T) {
	req := RequestInit(false)
	err := req.ParseHeader([]byte("GET / HTTP/1.1\r\nX-Evil\rFoo: bar\r\n\r\n"))
	if err != InvalidHeaders {
		t.Fatalf("bare CR in header name got %v, want InvalidHeaders", err)
	}
}

// 整个请求用裸 LF 分行（经典走私变体）：找不到 \r\n 终止符，按非法拒绝
func TestParseHeaderRejectsBareLFLineEndings(t *testing.T) {
	req := RequestInit(false)
	err := req.ParseHeader([]byte("GET / HTTP/1.1\nHost: a\n\n"))
	if err != UnknowInvalid {
		t.Fatalf("bare-LF request got %v, want UnknowInvalid", err)
	}
}

// 请求行路径含裸 LF：拒绝，防止 ByteRow 重建时拆出第二个请求行
func TestParseHeaderRejectsBareLFInPath(t *testing.T) {
	req := RequestInit(false)
	err := req.ParseHeader([]byte("GET /a\nHTTP/1.1: 1 HTTP/1.1\r\nHost: a\r\n\r\n"))
	if err != FirstLineInvalid {
		t.Fatalf("bare LF in path got %v, want FirstLineInvalid", err)
	}
}

// 合法请求的多轮 keep-alive 复用不受上述检查影响
func TestParseHeaderHealthyRequestStillParses(t *testing.T) {
	req := RequestInit(false)
	if err := req.ParseHeader([]byte("POST /x?y=1 HTTP/1.1\r\nHost: a\r\nContent-Type: text/plain\r\nContent-Length: 3\r\n\r\nabc")); err != RequestOk {
		t.Fatalf("healthy request got %v", err)
	}
	if req.GetHost() != "a" || req.Path != "/x?y=1" || req.Method != "POST" {
		t.Fatalf("fields: %+v", req)
	}
}

// ByteRow 重建的报文不允许出现任何注入通道：头值中的控制字符
// 在解析层已被拒绝，重建结果只含合法 CRLF 行终止
func TestByteRowHasNoInjectionChannel(t *testing.T) {
	req := RequestInit(false)
	if err := req.ParseHeader([]byte("GET / HTTP/1.1\r\nHost: a\r\nUser-Agent: ua\r\n\r\n")); err != RequestOk {
		t.Fatal(err)
	}
	row := req.ByteRow()
	// 除行终止 CRLF 外，报文中不应有裸 \n
	for _, line := range bytes.Split(row, []byte("\r\n")) {
		if bytes.ContainsAny(line, "\r\n") {
			t.Fatalf("bare CR/LF inside rebuilt line: %q", line)
		}
	}
	if !strings.HasSuffix(string(row), "\r\n\r\n") {
		t.Fatal("rebuilt request must end with empty line")
	}
}
