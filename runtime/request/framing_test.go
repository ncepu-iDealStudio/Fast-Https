package request

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

// "hello world" 11 字节的合法 chunked 帧。
const testChunkedBody = "b\r\nhello world\r\n0\r\n\r\n"

// TestFixRequestFramingAfterChunkedDecode 覆盖 issue 里的两个变体：
//
//   - A: CL=5 + TE=chunked + 11 字节 chunked body（CL.TE 走私形态）
//   - B: 只有 TE=chunked 的合法分块上传
//
// 两者解码后都是 11 字节裸 body，转发帧必须自洽：没有 Transfer-Encoding，
// Content-Length 等于 11；严格上游（net/http）能完整读出请求体。
func TestFixRequestFramingAfterChunkedDecode(t *testing.T) {
	cases := []struct {
		name    string
		headers string
	}{
		{"CL.TE smuggling", "Content-Length: 5\r\nTransfer-Encoding: chunked\r\n"},
		{"legit chunked upload", "Transfer-Encoding: chunked\r\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := "POST /index.html HTTP/1.1\r\nHost: example.com\r\n" +
				c.headers + "\r\n" + testChunkedBody

			req := RequestInit(false)
			if got := req.ParseHeader([]byte(raw)); got != RequestOk {
				t.Fatalf("ParseHeader() = %v", got)
			}
			if err := req.ParseBody([]byte(raw)); err != nil {
				t.Fatalf("ParseBody: %v", err)
			}
			body, err := ReadChunked(req.Body.Bytes(), func() ([]byte, error) {
				return nil, io.EOF
			}, 0)
			if err != nil {
				t.Fatalf("ReadChunked: %v", err)
			}

			req.Body.Reset()
			req.Body.Write(body)
			req.FixRequestFraming()

			forwarded := req.ByteRow()
			head, payload, ok := bytes.Cut(forwarded, []byte("\r\n\r\n"))
			if !ok {
				t.Fatalf("forwarded request has no header terminator: %q", forwarded)
			}
			headerText := string(head)
			if strings.Contains(strings.ToLower(headerText), "transfer-encoding") {
				t.Fatalf("Transfer-Encoding must be removed after decoding:\n%s", headerText)
			}
			if !strings.Contains(headerText, "Content-Length: 11") {
				t.Fatalf("Content-Length must be the decoded length 11:\n%s", headerText)
			}
			if strings.Contains(headerText, "Content-Length: 5") {
				t.Fatalf("stale client Content-Length survived (smuggling):\n%s", headerText)
			}
			if string(payload) != "hello world" {
				t.Fatalf("body = %q, want %q", payload, "hello world")
			}

			// 用严格上游解析器复核：修复前 TE + 裸 body 会直接解析失败（400）。
			parsed, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(forwarded)))
			if err != nil {
				t.Fatalf("strict upstream rejected forwarded request: %v\n%s", err, forwarded)
			}
			if parsed.ContentLength != 11 || len(parsed.TransferEncoding) != 0 {
				t.Fatalf("upstream framing: CL=%d TE=%v", parsed.ContentLength, parsed.TransferEncoding)
			}
			got, err := io.ReadAll(parsed.Body)
			if err != nil || string(got) != "hello world" {
				t.Fatalf("upstream body = %q, err=%v", got, err)
			}
		})
	}
}

func TestDelHeaderIsCaseInsensitive(t *testing.T) {
	req := RequestInit(false)
	req.Headers["transfer-encoding"] = "chunked"
	req.Headers["TRANSFER-ENCODING"] = "chunked"
	req.DelHeader("Transfer-Encoding")
	if len(req.Headers) != 0 {
		t.Fatalf("all case variants should be removed: %v", req.Headers)
	}
}
