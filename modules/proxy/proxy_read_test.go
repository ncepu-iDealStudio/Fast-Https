package proxy

import (
	"fast-https/config"
	"fast-https/runtime/response"
	"net"
	"strings"
	"testing"
)

func newTestReadKeepAlive(conn net.Conn) *ReadKeepAlive {
	rka := &ReadKeepAlive{p: &Proxy{ProxyType: config.PROXY_HTTP, ProxyConn: conn}}
	rka.res = response.ResponseInit()
	rka.res.SetFirstLine(200, "OK")
	rka.res.SetHeader("Transfer-Encoding", "chunked")
	return rka
}

// 上游 chunked 响应必须解码成原始 body，并改写 Transfer-Encoding/Content-Length
func TestParseChunkedDecodesBody(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		server.Write([]byte("3\r\nfoo\r\n3\r\nbar\r\n0\r\n\r\n"))
		server.Close()
	}()

	rka := newTestReadKeepAlive(client)
	if err := rka.parseChunked(); err != nil {
		t.Fatalf("parseChunked() error: %v", err)
	}
	if string(rka.body) != "foobar" {
		t.Errorf("body = %q, want %q", rka.body, "foobar")
	}
	if rka.res.GetHeader("Transfer-Encoding") != "" {
		t.Error("Transfer-Encoding should be removed after decoding")
	}
	if rka.res.GetHeader("Content-Length") != "6" {
		t.Errorf("Content-Length = %q, want %q", rka.res.GetHeader("Content-Length"), "6")
	}
	if !strings.HasSuffix(string(rka.finalStr), "foobar") {
		t.Errorf("finalStr should end with decoded body, got %q", rka.finalStr)
	}
}

// 截断的 chunked 流返回错误，让调用方回 500，而不是把半截数据透传给客户端
func TestParseChunkedRejectsTruncatedBody(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		server.Write([]byte("5\r\nabc"))
		server.Close()
	}()

	rka := newTestReadKeepAlive(client)
	if err := rka.parseChunked(); err == nil {
		t.Error("parseChunked() should fail on truncated chunked body")
	}
}
