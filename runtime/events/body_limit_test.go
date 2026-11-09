package events

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
)

// TestParseRequestBodyTooLargeReturns413 覆盖「请求体超过 max_body_size」。
//
// 原来的实现直接 return 0 关连接，客户端只看到连接被重置，分不清是
// 「服务端拒绝」还是「网络故障」。现在必须先回 413 再关闭：客户端拿到
// 状态码才知道该把请求改小，而不是无脑重试。
func TestParseRequestBodyTooLargeReturns413(t *testing.T) {
	prev := config.Current()
	t.Cleanup(func() { config.Store(prev, map[string]string{}) })

	// 把上限压到很小，避免测试里造几 MB 的 body
	cfg := *prev
	cfg.Limit.MaxBodySize = 32
	config.Store(&cfg, map[string]string{})

	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	body := strings.Repeat("x", 128)
	raw := "POST /upload HTTP/1.1\r\nHost: example.com\r\n" +
		"Content-Length: 128\r\n\r\n" + body

	statusCh := make(chan string, 1)
	go func() {
		_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
		line, err := bufio.NewReader(client).ReadString('\n')
		if err != nil {
			statusCh <- "read error: " + err.Error()
			return
		}
		statusCh <- strings.TrimSpace(line)
	}()

	// 写入方不要求发完：handler 判定超限后会立刻停读并关连接
	go func() {
		_, _ = client.Write([]byte(raw))
	}()

	ev := core.NewEvent(&listener.Listener{}, server)
	// WriteResponseClose 走 EventWrite，core.NewEvent 不装它（生产环境由
	// 监听器注入），这里直接挂同包的实现。
	ev.EventWrite = EventWrite
	codeCh := make(chan int, 1)
	go func() { codeCh <- parseRequest(ev, filters.NewFilter()) }()

	select {
	case status := <-statusCh:
		if !strings.Contains(status, "413") {
			t.Fatalf("状态行 = %q，期望包含 413", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("没有收到响应")
	}

	select {
	case code := <-codeCh:
		if code != 0 {
			t.Fatalf("parseRequest() = %d，超限后应返回 0 结束连接", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parseRequest 未返回")
	}
}
