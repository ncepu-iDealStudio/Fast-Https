package response

import (
	"fast-https/runtime/response"
	"fmt"
	"testing"
)

type ResponError struct {
	Code    int
	Message string
}

var (
	ResponseOk       = &ResponError{0, "Response OK"}
	None             = &ResponError{1, "None"}
	UnknowInvalid    = &ResponError{2, "Unknow invalid"}
	FirstLineInvalid = &ResponError{3, "First line invalid"}
	MethodInvalid    = &ResponError{4, "Method invalid"}
)

func (e *ResponError) Error() string {
	return fmt.Sprintf("Respon error code: %d, Message: %s", e.Code, e.Message)
}

type responTest struct {
	Row    string
	Respon response.Response
}

var normalTests = []responTest{
	{
		Row: "HTTP/1.0 503 Service Unavailable\r\n" +
			"Content-Length: 6\r\n\r\n" +
			"abcdef",
		Respon: response.Response{
			FirstLine: "HTTP/1.0 503 Service Unavailable",
			Headers: map[string]string{
				"Content-Length": "6",
			},
			Body: []byte("abcdef"),
		},
	},
	{
		Row: "HTTP/1.0 200 OK\r\n" +
			"\r\n" +
			"abcdef",
		Respon: response.Response{
			FirstLine: "HTTP/1.0 200 OK",
			Headers:   map[string]string{},
			Body:      []byte("abcdef"),
		},
	},
	{
		Row: "HTTP/1.1 200 OK\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"abcdef",
		Respon: response.Response{
			FirstLine: "HTTP/1.1 200 OK",
			Headers: map[string]string{
				"Connection": "close",
			},
			Body: []byte("abcdef"),
		},
	},
	{
		Row: "HTTP/1.1 200 OK\r\n" +
			"Connection: close\r\n" +
			"\r\n" +
			"abcdef",
		Respon: response.Response{
			FirstLine: "HTTP/1.1 200 OK",
			Headers: map[string]string{
				"Connection": "close",
			},
			Body: []byte("abcdef"),
		},
	},
	// {
	// 	Row: "HTTP/1.1 200 OK\r\n" +
	// 		"Transfer-Encoding: chunked\r\n\r\n" +
	// 		"6\r\nabcdef\r\n0\r\n\r\n",
	// 	Respon: response.Response{
	// 		FirstLine: "HTTP/1.1 200 OK",
	// 		Headers: map[string]string{
	// 			"Transfer-Encoding": "chunked",
	// 		},
	// 		Body: []byte("abcdef"),
	// 	},
	// },
	{
		Row: "HTTP/1.1 200 OK\r\n" +
			"Content-Length: 0\r\n" +
			"\r\n",
		Respon: response.Response{
			FirstLine: "HTTP/1.1 200 OK",
			Headers: map[string]string{
				"Content-Length": "0",
			},
			Body: []byte(""),
		},
	},
	{
		Row: "HTTP/1.1 200 OK\r\n" +
			"Connection: close\r\n" +
			"\r\nfoo",
		Respon: response.Response{
			FirstLine: "HTTP/1.1 200 OK",
			Headers: map[string]string{
				"Connection": "close",
			},
			Body: []byte("foo"),
		},
	},
	{
		Row: "HTTP/1.1 204 No Content\r\n" +
			"Connection: close\r\n" +
			"Foo: Bar Baz\r\n" +
			"\r\n",
		Respon: response.Response{
			FirstLine: "HTTP/1.1 204 No Content",
			Headers: map[string]string{
				"Connection": "close",
				"Foo":        "Bar Baz",
			},
			Body: []byte(""),
		},
	},
}

func TestGenerateResponse(t *testing.T) {
	for _, test := range normalTests {
		res := test.Respon.GenerateResponse()
		if string(res) != test.Row {
			t.Errorf("Test failed! %s", test.Row)
		}
	}
}

func TestHttpResParse(t *testing.T) {
	for _, test := range normalTests {
		res := response.ResponseInit()
		err := res.HttpResParse(test.Row)
		if err.Error() != ResponseOk.Error() {
			t.Errorf("Test failed! %s", test.Row)
		}
		for k, v := range test.Respon.Headers {
			if res.Headers[k] != v {
				t.Errorf("Test failed! %s", test.Row)
			}
		}
	}
}

// 上游响应头值含裸 LF 必须拒绝：否则透传给客户端时构成响应头注入
func TestHttpResParseRejectsBareLFInHeaderValue(t *testing.T) {
	res := response.ResponseInit()
	err := res.HttpResParse("HTTP/1.1 200 OK\r\nSet-Cookie: a\nEvil: injected\r\n\r\nbody")
	if err != response.InvalidHeaders {
		t.Fatalf("bare LF in upstream header value got %v, want InvalidHeaders", err)
	}
}

// 上游响应头名含裸 CR 同样拒绝
func TestHttpResParseRejectsBareCRInHeaderName(t *testing.T) {
	res := response.ResponseInit()
	err := res.HttpResParse("HTTP/1.1 200 OK\r\nX-Evil\rFoo: bar\r\n\r\nbody")
	if err != response.InvalidHeaders {
		t.Fatalf("bare CR in upstream header name got %v, want InvalidHeaders", err)
	}
}

// 无终止空行的响应：头合法时正常解析，Body 保持零值（既有望保持的语义）
func TestHttpResParseWithoutTerminatorKeepsBody(t *testing.T) {
	res := response.ResponseInit()
	if err := res.HttpResParse("HTTP/1.1 200 OK\r\nContent-Length: 3"); err != response.ResponseOk {
		t.Fatalf("headers without terminator got %v", err)
	}
	if res.Body != nil {
		t.Fatalf("body = %q, want nil without \r\n\r\n", res.Body)
	}
}
