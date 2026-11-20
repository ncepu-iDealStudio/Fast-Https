package request

import (
	"strings"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		name   string
		target string
		want   string
		ok     bool
	}{
		{name: "root", target: "/", want: "/", ok: true},
		{name: "plain", target: "/a/b.html", want: "/a/b.html", ok: true},
		{name: "query kept", target: "/a?x=1&y=2", want: "/a?x=1&y=2", ok: true},
		{name: "merge slashes", target: "//a///b", want: "/a/b", ok: true},
		{name: "trailing slash kept", target: "/a//", want: "/a/", ok: true},
		{name: "decode once", target: "/a%2fb", want: "/a/b", ok: true},
		{name: "decode utf8", target: "/%E4%B8%AD", want: "/中", ok: true},
		{name: "percent then non-hex", target: "/100%25", want: "/100%", ok: true},
		{name: "query not merged", target: "/a?b//c", want: "/a?b//c", ok: true},

		{name: "double encoding", target: "/a%252fb", ok: false},
		{name: "trailing percent", target: "/a%2", ok: false},
		{name: "invalid hex", target: "/a%zz", ok: false},
		{name: "encoded slash twice", target: "/a%252F", ok: false},
		{name: "backslash", target: `/a\b`, ok: false},
		{name: "encoded backslash", target: "/a%5Cb", ok: false},
		{name: "nul byte", target: "/a%00b", ok: false},
		{name: "raw control", target: "/a\x01b", ok: false},
		{name: "too long", target: "/" + strings.Repeat("a", MaxPathLength), ok: false},

		{name: "asterisk form", target: "*", want: "*", ok: true},
		{name: "absolute form", target: "http://a/b", want: "http://a/b", ok: true},
		{name: "collapse internal dotdot", target: "/public/../admin/panel", want: "/admin/panel", ok: true},
		{name: "collapse single dot", target: "/a/./b/.", want: "/a/b/", ok: true},
		{name: "preserve leading dotdot for static reject log", target: "/../etc/passwd", want: "/../etc/passwd", ok: true},
		{name: "query control char rejected", target: "/a?x=1\x0d\x0aInjected: 1", ok: false},
		{name: "absolute form backslash rejected", target: `http://a/b\c`, ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizePath(tc.target)
			if ok != tc.ok {
				t.Fatalf("NormalizePath(%q) ok = %v, want %v", tc.target, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("NormalizePath(%q) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}

// 规范化发生在解析阶段：location 匹配与日志看到的都是合并斜杠后的路径。
func TestParseHeaderNormalizesPath(t *testing.T) {
	req := RequestInit(false)
	err := req.ParseHeader([]byte("GET //a//b%2Fc?x=1 HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	if err != RequestOk {
		t.Fatalf("parse: %v", err)
	}
	if req.Path != "/a/b/c?x=1" {
		t.Fatalf("path = %q, want %q", req.Path, "/a/b/c?x=1")
	}
}

func TestParseHeaderRejectsBadPath(t *testing.T) {
	req := RequestInit(false)
	err := req.ParseHeader([]byte("GET /a%252fb HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	if err != PathInvalid {
		t.Fatalf("err = %v, want PathInvalid", err)
	}
}

func TestParseHeaderFragmentedAndConflictingContentLength(t *testing.T) {
	// TCP 分片恰好停在 "Content-Length: " 时必须返回 RequestNeedReadMore 而非 400
	req := RequestInit(false)
	err := req.ParseHeader([]byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: "))
	if err != RequestNeedReadMore {
		t.Fatalf("fragmented Content-Length err = %v, want RequestNeedReadMore", err)
	}

	// 冲突的双 Content-Length 必须直接拒绝
	req2 := RequestInit(false)
	err2 := req2.ParseHeader([]byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nContent-Length: 10\r\n\r\n"))
	if err2 != InvalidHeaders {
		t.Fatalf("conflicting Content-Length err = %v, want InvalidHeaders", err2)
	}
}

func TestChunkedTokenAndTrailers(t *testing.T) {
	if !HasTransferEncodingToken("chunked;q=0", "chunked") {
		t.Fatal("chunked;q=0 must still be recognized as chunked Transfer-Encoding")
	}
	body, err := ReadChunked([]byte("5\r\nhello\r\n0\r\nX-Checksum: abc\r\n\r\n"), func() ([]byte, error) {
		return nil, nil
	}, 1024)
	if err != nil || string(body) != "hello" {
		t.Fatalf("chunked with trailer: body=%q err=%v", string(body), err)
	}
}
