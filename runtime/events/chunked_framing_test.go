package events

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"fast-https/core"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
)

const e2eChunkedBody = "b\r\nhello world\r\n0\r\n\r\n"

type upstreamResult struct {
	contentLength int64
	transferEnc   []string
	body          string
	err           error
}

// 真实端到端复现：客户端经 net.Pipe 走完整的 parseRequest 管线，转发字节
// 原样写进一个 raw TCP 上游，上游用严格解析器（net/http）读一条请求。
//
// 修复前上游收到的是：
//
//	A: Content-Length: 5 + Transfer-Encoding: chunked + 裸 11 字节 body
//	   → 按 CL 取数的上游只读 5 字节，剩余字节被当成下一条请求（CL.TE 走私）
//	B: Transfer-Encoding: chunked + 裸 body（无分块帧）→ 严格上游直接 400
//
// 修复后两个变体都应变成 Content-Length: 11 的自洽请求。
func TestParseRequestChunkedForwardsConsistentFraming(t *testing.T) {
	cases := []struct {
		name    string
		headers string
	}{
		{"CL.TE smuggling", "Content-Length: 5\r\nTransfer-Encoding: chunked\r\n"},
		{"legit chunked upload", "Transfer-Encoding: chunked\r\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Skipf("loopback listen unavailable: %v", err)
			}
			defer ln.Close()

			resultCh := make(chan upstreamResult, 1)
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					resultCh <- upstreamResult{err: err}
					return
				}
				defer conn.Close()
				_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
				req, err := http.ReadRequest(bufio.NewReader(conn))
				if err != nil {
					resultCh <- upstreamResult{err: err}
					return
				}
				body, err := io.ReadAll(req.Body)
				resultCh <- upstreamResult{
					contentLength: req.ContentLength,
					transferEnc:   req.TransferEncoding,
					body:          string(body),
					err:           err,
				}
			}()

			server, client := net.Pipe()
			defer client.Close()
			raw := "POST /index.html HTTP/1.1\r\nHost: example.com\r\n" +
				c.headers + "\r\n" + e2eChunkedBody
			go func() {
				_, _ = client.Write([]byte(raw))
				time.Sleep(150 * time.Millisecond)
			}()

			ev := core.NewEvent(&listener.Listener{}, server)
			codeCh := make(chan int, 1)
			go func() { codeCh <- parseRequest(ev, filters.NewFilter()) }()
			select {
			case code := <-codeCh:
				if code != 1 {
					t.Fatalf("parseRequest() = %d, want 1", code)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("parseRequest timed out")
			}

			upstream, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatalf("dial upstream: %v", err)
			}
			defer upstream.Close()
			forwarded := ev.RR.Req.ByteRow()
			if _, err := upstream.Write(forwarded); err != nil {
				t.Fatalf("write forwarded bytes: %v", err)
			}

			select {
			case got := <-resultCh:
				if got.err != nil {
					t.Fatalf("strict upstream rejected forwarded request: %v\n%s", got.err, forwarded)
				}
				if got.contentLength != 11 {
					t.Fatalf("upstream ContentLength = %d, want 11\n%s", got.contentLength, forwarded)
				}
				if len(got.transferEnc) != 0 {
					t.Fatalf("upstream TransferEncoding = %v, want empty\n%s", got.transferEnc, forwarded)
				}
				if got.body != "hello world" {
					t.Fatalf("upstream body = %q, want %q", got.body, "hello world")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("upstream never parsed the forwarded request")
			}
		})
	}
}
