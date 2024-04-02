package response

import (
	"fast-https/modules/core/response"
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
