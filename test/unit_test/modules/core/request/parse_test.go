package request

import (
	"fast-https/modules/core/request"
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
		Row: "GET / HTTP/1.1\r\n" +
			"Host: example.com\r\n" +
			"Connection: close\r\n",
		Req: request.Request{},
		Err: InvalidHeaders.Error(),
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
