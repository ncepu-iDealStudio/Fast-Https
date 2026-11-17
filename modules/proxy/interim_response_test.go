package proxy

import (
	"net"
	"strings"
	"testing"
)

// 1xx 中间响应（100 Continue / 103 Early Hints）必须被丢弃，
// 最终响应才是客户端看到的那一个。
func TestInterimResponseInSameRead(t *testing.T) {
	for _, interim := range []string{"HTTP/1.1 100 Continue\r\n\r\n", "HTTP/1.1 103 Early Hints\r\nLink: </a.css>\r\n\r\n"} {
		t.Run(strings.Fields(interim)[1], func(t *testing.T) {
			client, upstream := net.Pipe()
			t.Cleanup(func() { _ = client.Close(); _ = upstream.Close() })

			go func() {
				// 中间响应与最终响应落在同一次读取里
				_, _ = upstream.Write([]byte(interim + "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
			}()

			rka := &ReadKeepAlive{p: newMatrixProxy(client)}
			if err := runWithWatchdog(t, "interim-same-read", rka.proxyKeepAlive); err != nil {
				t.Fatalf("proxyKeepAlive() error: %v", err)
			}
			if got := string(rka.finalStr); strings.Contains(got, " 100 ") || strings.Contains(got, " 103 ") {
				t.Errorf("interim response leaked into final data: %q", got)
			}
			if rka.res == nil || !strings.Contains(rka.res.FirstLine, "200") {
				t.Errorf("final response = %+v, want 200", rka.res)
			}
			if string(rka.body) != "ok" {
				t.Errorf("body = %q, want %q", rka.body, "ok")
			}
		})
	}
}

// 中间响应与最终响应分两次到达：跳过 1xx 后必须继续读取。
func TestInterimResponseAcrossReads(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = upstream.Close() })

	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 100 Continue\r\n\r\n"))
		_, _ = upstream.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"))
	}()

	rka := &ReadKeepAlive{p: newMatrixProxy(client)}
	if err := runWithWatchdog(t, "interim-across-reads", rka.proxyKeepAlive); err != nil {
		t.Fatalf("proxyKeepAlive() error: %v", err)
	}
	if strings.Contains(string(rka.finalStr), "100 Continue") {
		t.Errorf("interim response leaked: %q", rka.finalStr)
	}
	if string(rka.body) != "hello" {
		t.Errorf("body = %q, want %q", rka.body, "hello")
	}
}

// 跳过 1xx 之后仍要识别空 body 状态，保证连接可以复用。
func TestInterimResponseThenBodylessStatus(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = upstream.Close() })

	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 204 No Content\r\n\r\n"))
	}()

	rka := &ReadKeepAlive{p: newMatrixProxy(client)}
	if err := runWithWatchdog(t, "interim-204", rka.proxyKeepAlive); err != nil {
		t.Fatalf("proxyKeepAlive() error: %v", err)
	}
	if !rka.emptyBody {
		t.Error("204 after interim must still be marked as having no body")
	}
	if len(rka.body) != 0 {
		t.Errorf("body = %q, want empty", rka.body)
	}
	if strings.Contains(string(rka.finalStr), "100 Continue") {
		t.Errorf("interim response leaked: %q", rka.finalStr)
	}
}

// 101 Switching Protocols 是协议升级，不按中间响应丢弃。
func TestSwitchingProtocolsNotTreatedAsInterim(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = upstream.Close() })

	go func() {
		_, _ = upstream.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\n\r\n"))
		_ = upstream.Close()
	}()

	rka := &ReadKeepAlive{p: newMatrixProxy(client)}
	if err := runWithWatchdog(t, "101", rka.proxyKeepAlive); err != nil {
		t.Fatalf("proxyKeepAlive() error: %v", err)
	}
	if rka.res == nil || !strings.Contains(rka.res.FirstLine, "101") {
		t.Errorf("101 must be kept as the response, got %+v", rka.res)
	}
}
