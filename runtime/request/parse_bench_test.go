package request

import (
	"bufio"
	"bytes"
	"net/http"
	"strings"
	"testing"
)

// 与 net/http 官方解析器的对比基准。
//
// 公平性说明：
//   - net/http 侧复用 bufio.Reader 连续读 N 个相同请求，模拟真实
//     连接上的 reader 复用（官方 server 每连接复用 bufio）；
//   - 本包侧模拟 keep-alive 复用 Request 对象（Flush 后重新解析），
//     同时给出全新分配的对照；
//   - 两者都只测"字节 → 结构化请求"这一步，不含 socket 读。

var benchGET = []byte("GET /some/path?query=abc&x=1 HTTP/1.1\r\n" +
	"Host: www.example.com\r\n" +
	"User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36\r\n" +
	"Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8\r\n" +
	"Accept-Language: en-US,en;q=0.5\r\n" +
	"Accept-Encoding: gzip, br\r\n" +
	"Connection: keep-alive\r\n" +
	"Cookie: session=abcdef0123456789; theme=dark; lang=zh-CN\r\n" +
	"Upgrade-Insecure-Requests: 1\r\n" +
	"\r\n")

var benchPOST = []byte("POST /api/submit HTTP/1.1\r\n" +
	"Host: www.example.com\r\n" +
	"Content-Type: application/json\r\n" +
	"Content-Length: 64\r\n" +
	"Connection: keep-alive\r\n" +
	"\r\n" +
	`{"name":"fast-https","tags":["a","b"],"count":42,"ok":true}` + "\r\n\r\n")

// 本项目自研解析器：keep-alive 复用请求对象（与真实服务一致）
func BenchmarkSelfParseGETReuse(b *testing.B) {
	req := RequestInit(false)
	req.ParseHeader(benchGET)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req.Flush()
		if err := req.ParseHeader(benchGET); err != RequestOk {
			b.Fatal(err)
		}
		if err := req.ParseBody(benchGET); err != nil {
			b.Fatal(err)
		}
	}
}

// 本项目自研解析器：每请求全新分配（对照）
func BenchmarkSelfParseGETFresh(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := RequestInit(false)
		if err := req.ParseHeader(benchGET); err != RequestOk {
			b.Fatal(err)
		}
		if err := req.ParseBody(benchGET); err != nil {
			b.Fatal(err)
		}
	}
}

// cycleReader 无限循环重放同一段请求字节：内存固定，
// 避免 bytes.Repeat(…, b.N) 随迭代数膨胀带来的 GC 噪声。
type cycleReader struct {
	buf []byte
	pos int
}

func (c *cycleReader) Read(p []byte) (int, error) {
	if c.pos >= len(c.buf) {
		c.pos = 0
	}
	n := copy(p, c.buf[c.pos:])
	c.pos += n
	return n, nil
}

func newCycleStream(sample []byte, n int) *bufio.Reader {
	return bufio.NewReaderSize(&cycleReader{buf: bytes.Repeat(sample, n)}, 4096)
}

// net/http 官方解析器：复用 bufio（与真实 server 一致）
func BenchmarkNetHTTPParseGET(b *testing.B) {
	r := newCycleStream(benchGET, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.ReadRequest(r)
		if err != nil {
			b.Fatal(err)
		}
		_ = req.Body.Close()
	}
}

// 本项目自研解析器：带 body 的 POST
func BenchmarkSelfParsePOST(b *testing.B) {
	req := RequestInit(false)
	req.ParseHeader(benchPOST)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req.Flush()
		if err := req.ParseHeader(benchPOST); err != RequestOk {
			b.Fatal(err)
		}
		if err := req.ParseBody(benchPOST); err != nil {
			b.Fatal(err)
		}
	}
}

// net/http 官方解析器：带 body 的 POST
func BenchmarkNetHTTPParsePOST(b *testing.B) {
	r := newCycleStream(benchPOST, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.ReadRequest(r)
		if err != nil {
			b.Fatal(err)
		}
		ioCopyN(b, req.Body, 64)
		_ = req.Body.Close()
	}
}

func ioCopyN(b *testing.B, r interface{ Read([]byte) (int, error) }, n int64) {
	buf := make([]byte, n)
	if _, err := r.Read(buf); err != nil && !strings.Contains(err.Error(), "EOF") {
		b.Fatal(err)
	}
}
