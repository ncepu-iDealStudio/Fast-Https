package events

import (
	"net/http"
	"strings"
	"testing"

	"fast-https/core"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

func TestIsHopByHopHeader(t *testing.T) {
	for _, key := range []string{
		"Connection", "connection", "Keep-Alive", "Proxy-Connection",
		"Transfer-Encoding", "Upgrade", "Te", "Trailer",
	} {
		if !isHopByHopHeader(key) {
			t.Errorf("%q must be hop-by-hop", key)
		}
	}
	for _, key := range []string{"Content-Type", "Set-Cookie", "X-Custom", "Date"} {
		if isHopByHopHeader(key) {
			t.Errorf("%q must not be hop-by-hop", key)
		}
	}
}

func TestStatusFromFirstLine(t *testing.T) {
	cases := map[string]int{
		"HTTP/1.1 200 OK":                 200,
		"HTTP/1.1 404 Not Found":          404,
		"HTTP/1.1 503 Service Unavailable": 503,
		"HTTP/1.1 200":                    200,
		"":                                200, // 兜底 200
		"garbage":                         200,
		"HTTP/1.1 abc OK":                 200, // 非数字兜底
		"HTTP/1.1 99 Low":                 200, // 越界兜底
		"HTTP/1.1 1000 Big":               200, // 越界兜底
	}
	for line, want := range cases {
		if got := statusFromFirstLine(line); got != want {
			t.Errorf("statusFromFirstLine(%q) = %d, want %d", line, got, want)
		}
	}
}

func TestFillRequestMapsFields(t *testing.T) {
	r, err := http.NewRequest("POST", "https://example.com/a/b?x=1&x=2&y=z", strings.NewReader("request-body"))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("User-Agent", "h2-client")
	r.Header.Set("Accept", "text/html")

	req := request.RequestInit(true)
	fillRequest(req, r)

	if req.Method != "POST" || req.Protocol != "HTTP/2" {
		t.Fatalf("method/protocol = %s/%s", req.Method, req.Protocol)
	}
	if req.Path != "/a/b?x=1&x=2&y=z" {
		t.Fatalf("path = %q", req.Path)
	}
	if req.Headers["Host"] != "example.com" {
		t.Fatalf("host = %q", req.Headers["Host"])
	}
	if req.Headers["User-Agent"] != "h2-client" {
		t.Fatalf("headers = %v", req.Headers)
	}
	// 同名 query 取首值
	if req.Query["x"] != "1" || req.Query["y"] != "z" {
		t.Fatalf("query = %v", req.Query)
	}
	if req.Body.String() != "request-body" || req.BodyLen != 12 {
		t.Fatalf("body = %q len=%d", req.Body.String(), req.BodyLen)
	}

	// 无 body 的请求不得误读
	r2, _ := http.NewRequest("GET", "https://example.com/", nil)
	req2 := request.RequestInit(true)
	fillRequest(req2, r2)
	if req2.Body.Len() != 0 {
		t.Fatalf("body should stay empty, got %q", req2.Body.String())
	}
}

// fakeRW 记录 h2ResponseWriter 写出的头、状态码与体
type fakeRW struct {
	hdr    http.Header
	status int
	body   strings.Builder
}

func newFakeRW() *fakeRW          { return &fakeRW{hdr: make(http.Header)} }
func (f *fakeRW) Header() http.Header { return f.hdr }
func (f *fakeRW) WriteHeader(code int) { f.status = code }
func (f *fakeRW) Write(b []byte) (int, error) {
	if f.status == 0 {
		f.status = 200
	}
	return f.body.Write(b)
}

// h2ResponseWriter：逐跳头过滤、状态码提取、幂等写
func TestH2ResponseWriterWrite(t *testing.T) {
	r, _ := http.NewRequest("GET", "https://example.com/", nil)
	rw := newFakeRW()
	writer := &h2ResponseWriter{w: rw, req: r}

	ev := core.NewEvent(nil, nil)
	ev.RR.Res = response.ResponseInit()
	ev.RR.Res.SetFirstLine(404, "Not Found")
	ev.RR.Res.SetHeader("Connection", "close") // 逐跳：必须被剔除
	ev.RR.Res.SetHeader("X-Custom", "kept")
	ev.RR.Res.SetHeader("Content-Length", "3")
	ev.RR.Res.Body = []byte("abc")

	if err := writer.write(ev); err != nil {
		t.Fatal(err)
	}
	if rw.hdr.Get("X-Custom") != "kept" {
		t.Error("X-Custom header lost")
	}
	if rw.hdr.Get("Connection") != "" {
		t.Error("hop-by-hop Connection must be stripped over h2")
	}
	if rw.status != 404 {
		t.Errorf("status = %d, want 404", rw.status)
	}
	if rw.body.String() != "abc" {
		t.Errorf("body = %q", rw.body.String())
	}

	// 二次写是 no-op（written 幂等）
	if err := writer.write(ev); err != nil {
		t.Fatal(err)
	}
	if rw.body.String() != "abc" {
		t.Errorf("duplicate write detected: %q", rw.body.String())
	}
}

// HEAD：保留模块给的 Content-Length，不写响应体
func TestH2ResponseWriterHeadOmitsBody(t *testing.T) {
	r, _ := http.NewRequest("HEAD", "https://example.com/", nil)
	rw := newFakeRW()
	writer := &h2ResponseWriter{w: rw, req: r}

	ev := core.NewEvent(nil, nil)
	ev.RR.Res = response.ResponseInit()
	ev.RR.Res.SetFirstLine(200, "OK")
	ev.RR.Res.SetHeader("Content-Length", "100")
	ev.RR.Res.Body = []byte("must-not-be-written")

	if err := writer.write(ev); err != nil {
		t.Fatal(err)
	}
	if rw.body.String() != "" {
		t.Fatalf("HEAD wrote body: %q", rw.body.String())
	}
	if rw.hdr.Get("Content-Length") != "100" {
		t.Fatalf("HEAD Content-Length = %q, want 100", rw.hdr.Get("Content-Length"))
	}
}
