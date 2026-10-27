package request

import (
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fmt"
	"testing"
)

type RequestError struct {
	Code    int
	Message string
}

var (
	RequestOk           = &RequestError{0, "Request OK"}
	None                = &RequestError{1, "None"}
	UnknowInvalid       = &RequestError{2, "Unknow invalid"}
	FirstLineInvalid    = &RequestError{3, "First line invalid"}
	MethodInvalid       = &RequestError{4, "Method invalid"}
	RequestNeedReadMore = &RequestError{5, "Request need read more"}
	InvalidHeaders      = &RequestError{6, "Invalid headers"}
	ProtocolInvalid     = &RequestError{7, "Protocol invalid"}
	PathInvalid         = &RequestError{8, "Path invalid"}
)

func (e *RequestError) Error() string {
	return fmt.Sprintf("Request error code: %d, Message: %s", e.Code, e.Message)
}

type requestTest struct {
	Row  string
	Req  request.Request
	Err  string
	Body string
}

var normalTests = []requestTest{
	{
		Row: "GET / HTTP/1.1\r\n" +
			"Host: example.com\r\n" +
			"Connection: close\r\n" +
			"\r\n",
		Req: request.Request{
			Method:   "GET",
			Path:     "/",
			Protocol: "HTTP/1.1",
			Headers: map[string]string{
				"Connection": "close",
				"Host":       "example.com",
			},
		},
		Err:  RequestOk.Error(),
		Body: "",
	},
	{
		Row: "POST /api HTTP/1.1\r\n" +
			"Host: example.com\r\n" +
			"Connection: close\r\n" +
			"\r\n",
		Req: request.Request{
			Method:   "POST",
			Path:     "/api",
			Protocol: "HTTP/1.1",
			Headers: map[string]string{
				"Connection": "close",
				"Host":       "example.com",
			},
		},
		Err:  RequestOk.Error(),
		Body: "",
	},
	{
		Row: "PUT /api/update/123 HTTP/1.1\r\n" +
			"Host: example.com\r\n" +
			"Content-Type: application/json\r\n" +
			"\r\n" +
			"{ \"name\": \"Jane Smith\", \"age\": 35}",
		Req: request.Request{
			Method:   "PUT",
			Path:     "/api/update/123",
			Protocol: "HTTP/1.1",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"Host":         "example.com",
			},
		},
		Err:  RequestOk.Error(),
		Body: "{ \"name\": \"Jane Smith\", \"age\": 35}",
	},
	// {
	// 	Row: "POST / HTTP/1.1\r\n" +
	// 		"Host: foo.com\r\n" +
	// 		"Transfer-Encoding: chunked\r\n\r\n" +
	// 		"3\r\nfoo\r\n" +
	// 		"3\r\nbar\r\n" +
	// 		"0\r\n" +
	// 		"Trailer-Key: Trailer-Value\r\n" +
	// 		"\r\n",
	// 	Req: request.Req{
	// 		Method:   "POST",
	// 		Path:     "/",
	// 		Protocol: "HTTP/1.1",
	// 		Headers: map[string]string{
	// 			"Host":              "foo.com",
	// 			"Transfer-Encoding": "chunked",
	// 		},
	// 	},
	// 	Err: RequestOk.Error(),
	// 	Body: "foobar",
	// },
	{
		Row: "CONNECT www.google.com:443 HTTP/1.1\r\n\r\n",
		Req: request.Request{
			Method:   "CONNECT",
			Path:     "www.google.com:443",
			Protocol: "HTTP/1.1",
			Headers:  map[string]string{},
		},
		Err:  RequestOk.Error(),
		Body: "",
	},
	{
		Row: "OPTIONS * HTTP/1.1\r\nServer: foo\r\n\r\n",
		Req: request.Request{
			Method:   "OPTIONS",
			Path:     "*",
			Protocol: "HTTP/1.1",
			Headers: map[string]string{
				"Server": "foo",
			},
		},
		Err:  RequestOk.Error(),
		Body: "",
	},
}

func TestParseHeader(t *testing.T) {
	for _, test := range normalTests {
		req := request.RequestInit(false)
		err := req.ParseHeader([]byte(test.Row))
		if err.Error() != test.Err {
			t.Errorf("ParseHeader() got %s, want %s", err.Error(), test.Err)
		}
		if req.Method != test.Req.Method {
			t.Errorf("ParseHeader() got method %s, want %s", req.Method, test.Req.Method)
		}
		if req.Path != test.Req.Path {
			t.Errorf("ParseHeader() got path %s, want %s", req.Path, test.Req.Path)
		}
		if req.Protocol != test.Req.Protocol {
			t.Errorf("ParseHeader() got protocol %s, want %s", req.Protocol, test.Req.Protocol)
		}
		// if req.HeaderLen != test.Req.HeaderLen {
		// 	t.Errorf("ParseHeader() got header length %d, want %d", req.HeaderLen, test.Req.HeaderLen)
		// }
		for k, v := range test.Req.Headers {
			if req.Headers[k] != v {
				t.Errorf("ParseHeader() got header %s: %s, want %s", k, req.Headers[k], v)
			}
		}
	}
}

var errorTests = []requestTest{
	{
		Row: "",
		Req: request.Request{},
		Err: None.Error(),
	},
	{
		// 头已到但终止空行未到，属于"需要继续读"而不是非法
		Row: "GET / HTTP/1.1\r\n" +
			"Host: example.com\r\n" +
			"Connection: close\r\n",
		Req: request.Request{},
		Err: RequestNeedReadMore.Error(),
	},
	{
		Row: "GET / HTTP/1.1\r\n",
		Req: request.Request{},
		Err: RequestNeedReadMore.Error(),
	},
	{
		Row: "GET / HTTP/1.1\r\r" +
			"Host: example.com\r\r" +
			"Connection: close\r\r",
		Req: request.Request{},
		Err: UnknowInvalid.Error(),
	},
	{
		Row: "GET / HTTP/9.9\r\n" +
			"Host: example.com\r\n" +
			"Connection: close\r\n",
		Req: request.Request{},
		Err: ProtocolInvalid.Error(),
	},
	{
		Row: "HIT / HTTP/1.1\r\n" +
			"Host: example.com\r\n" +
			"Connection: close\r\n",
		Req: request.Request{},
		Err: MethodInvalid.Error(),
	},
	{
		Row: "GET  HTTP/1.1\r\n" +
			"Host: test\r\n\r\n",
		Req: request.Request{},
		Err: PathInvalid.Error(),
	},
}

func TestParseHeaderError(t *testing.T) {
	for _, test := range errorTests {
		req := request.RequestInit(false)
		err := req.ParseHeader([]byte(test.Row))
		if err.Error() != test.Err {
			t.Errorf("ParseHeader() got %s; Want %s", err.Error(), test.Err)
		}
	}
}

func TestParseBody(t *testing.T) {
	for _, test := range normalTests {
		req := request.RequestInit(false)
		req.ParseHeader([]byte(test.Row))
		req.ParseBody([]byte(test.Row))
		if req.Body.String() != test.Body {
			t.Errorf("ParseBody() got %s; Want %s", req.Body.String(), test.Body)
		}
	}
}

func TestIsKeepalive(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		conn     string
		want     bool
	}{
		{name: "HTTP/1.1 default", protocol: "HTTP/1.1", want: true},
		{name: "HTTP/1.0 default", protocol: "HTTP/1.0", want: false},
		{name: "explicit keep-alive", protocol: "HTTP/1.0", conn: "keep-alive", want: true},
		{name: "case-insensitive keep-alive", protocol: "HTTP/1.0", conn: "Keep-Alive", want: true},
		{name: "explicit close", protocol: "HTTP/1.1", conn: "close", want: false},
		{name: "close token takes precedence", protocol: "HTTP/1.1", conn: "keep-alive, close", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := request.Request{Protocol: test.protocol, Headers: map[string]string{"Connection": test.conn}}
			if got := req.IsKeepalive(); got != test.want {
				t.Errorf("IsKeepalive() = %t, want %t", got, test.want)
			}
		})
	}
}

// 头名大小写不敏感：小写头必须能按 canonical 键读到
func TestParseHeaderCanonicalizesKeys(t *testing.T) {
	req := request.RequestInit(false)
	err := req.ParseHeader([]byte("POST / HTTP/1.1\r\n" +
		"host: example.com\r\n" +
		"content-length: 5\r\n" +
		"transfer-encoding: identity\r\n" +
		"\r\nhello"))
	if err != request.RequestOk {
		t.Fatalf("ParseHeader() got %s, want RequestOk", err.Error())
	}
	if req.GetHost() != "example.com" {
		t.Errorf("GetHost() = %q, want %q", req.GetHost(), "example.com")
	}
	if req.GetContentLength() != "5" {
		t.Errorf("GetContentLength() = %q, want %q", req.GetContentLength(), "5")
	}
	if req.GetTransferEncoding() != "identity" {
		t.Errorf("GetTransferEncoding() = %q, want %q", req.GetTransferEncoding(), "identity")
	}
}

// 非法 Content-Length 必须被拒绝，而不是在读取 body 时 panic
func TestParseHeaderRejectsInvalidContentLength(t *testing.T) {
	for _, row := range []string{
		"POST / HTTP/1.1\r\nHost: a\r\nContent-Length: abc\r\n\r\n",
		"POST / HTTP/1.1\r\nHost: a\r\nContent-Length: -5\r\n\r\n",
	} {
		req := request.RequestInit(false)
		if err := req.ParseHeader([]byte(row)); err != request.InvalidHeaders {
			t.Errorf("ParseHeader(%q) got %s, want InvalidHeaders", row, err.Error())
		}
	}
}

// 无名头的行（": value"）不再触发 key[:1] 越界 panic
func TestParseHeaderRejectsEmptyHeaderKey(t *testing.T) {
	req := request.RequestInit(false)
	err := req.ParseHeader([]byte("GET / HTTP/1.1\r\n: value\r\n\r\n"))
	if err != request.InvalidHeaders {
		t.Errorf("ParseHeader() got %s, want InvalidHeaders", err.Error())
	}
}

// 最后一行是半个头时需要继续读，不能当成非法
func TestParseHeaderPartialLastLineNeedsReadMore(t *testing.T) {
	req := request.RequestInit(false)
	err := req.ParseHeader([]byte("GET / HTTP/1.1\r\nHost: a\r\nConn"))
	if err != request.RequestNeedReadMore {
		t.Errorf("ParseHeader() got %s, want RequestNeedReadMore", err.Error())
	}
}

// keep-alive 复用请求对象时，Flush 必须清掉上一个请求的 query
func TestFlushClearsQuery(t *testing.T) {
	req := request.RequestInit(false)
	req.Query["x"] = "1"
	req.Query["y"] = "2"
	req.Flush()
	if len(req.Query) != 0 {
		t.Errorf("Flush() left query %v, want empty", req.Query)
	}
}

// ---- 查询串与请求体校验的剩余分支 ----

func TestParseQueryParamsVariants(t *testing.T) {
	req := request.RequestInit(false)
	req.Path = "/list?a=1&b=2&flag&c=3"
	req.Headers["Host"] = "h" // ParseHost 守卫要求 Host 非空
	req.ParseHost(&listener.Listener{Port: "8080"}) // 内部触发 parseQueryParams
	if req.Query["a"] != "1" || req.Query["b"] != "2" || req.Query["c"] != "3" {
		t.Fatalf("query = %v", req.Query)
	}
	// 无 = 的参数（flag）不产生条目
	if _, ok := req.Query["flag"]; ok {
		t.Fatal("parameter without '=' must be skipped")
	}

	// 无查询串、纯 '?'、'?' 后为空
	for _, p := range []string{"/plain", "/q?", "/q"} {
		r := request.RequestInit(false)
		r.Path = p
		r.Headers["Host"] = "h"
		r.ParseHost(&listener.Listener{Port: "8080"})
		if len(r.Query) != 0 {
			t.Fatalf("path %q produced query %v", p, r.Query)
		}
	}
}

func TestParseHostAppendsDefaultPort(t *testing.T) {
	req := request.RequestInit(false)
	req.Headers["Host"] = "a.example"
	req.ParseHost(&listener.Listener{Port: "80"})
	if req.GetHost() != "a.example:80" {
		t.Fatalf("host = %q, want :80 appended", req.GetHost())
	}

	req2 := request.RequestInit(false)
	req2.Headers["Host"] = "b.example"
	req2.ParseHost(&listener.Listener{Port: "443"})
	if req2.GetHost() != "b.example:443" {
		t.Fatalf("host = %q, want :443 appended", req2.GetHost())
	}

	// 非 80/443 端口或无 Host：保持原样
	req3 := request.RequestInit(false)
	req3.Headers["Host"] = "c.example"
	req3.ParseHost(&listener.Listener{Port: "8080"})
	if req3.GetHost() != "c.example" {
		t.Fatalf("host = %q, must stay unmodified", req3.GetHost())
	}
}

func TestRequestBodyValidBranches(t *testing.T) {
	t.Run("content-length mismatch", func(t *testing.T) {
		req := request.RequestInit(false)
		req.Headers["Content-Length"] = "5"
		req.Body.WriteString("abc") // 3 != 5
		if req.RequestBodyValid() {
			t.Fatal("short body must be invalid")
		}
	})

	t.Run("content-length non-numeric", func(t *testing.T) {
		req := request.RequestInit(false)
		req.Headers["Content-Length"] = "abc"
		req.Body.WriteString("abc")
		if req.RequestBodyValid() {
			t.Fatal("non-numeric CL must be invalid (no panic)")
		}
	})

	t.Run("no framing headers", func(t *testing.T) {
		req := request.RequestInit(false)
		if !req.RequestBodyValid() {
			t.Fatal("no CL body is vacuously valid")
		}
	})

	t.Run("multipart complete via incremental scan", func(t *testing.T) {
		req := request.RequestInit(false)
		req.Headers["Content-Type"] = "multipart/form-data; boundary=BOUND"
		req.TryFixBody([]byte("--BOUND\r\nfield\r\n"))
		if req.RequestBodyValid() {
			t.Fatal("multipart without terminal boundary must be invalid")
		}
		req.TryFixBody([]byte("--BOUND--\r\n"))
		if !req.RequestBodyValid() {
			t.Fatal("terminal boundary must complete the body")
		}
	})
}

func TestHeaderGettersAndSetHeader(t *testing.T) {
	req := request.RequestInit(false)
	req.SetHeader("X-Custom", "v", &listener.ListenCfg{})
	if req.GetHeader("X-Custom") != "v" {
		t.Fatalf("SetHeader/GetHeader roundtrip failed")
	}
	if req.GetUpgrade() != "" || req.GetAuthorization() != "" || req.GetContentType() != "" {
		t.Fatal("unset headers must read as empty strings")
	}
	req.Headers["Upgrade"] = "websocket"
	req.Headers["Authorization"] = "Basic x"
	if req.GetUpgrade() != "websocket" || req.GetAuthorization() != "Basic x" {
		t.Fatal("typed getters must read the map")
	}
}

func TestTryFixBodyAppends(t *testing.T) {
	req := request.RequestInit(false)
	req.TryFixBody([]byte("one"))
	req.TryFixBody([]byte("two"))
	if req.Body.String() != "onetwo" {
		t.Fatalf("body = %q", req.Body.String())
	}
}
