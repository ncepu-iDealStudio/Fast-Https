package websocket

import (
	"fast-https/core"
	"fast-https/modules/proxy"
	"io"
	"net"
	"testing"
	"time"
)

func TestWebSocketHandlerEcho(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()
	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		_, _ = conn.Write(buf)
	}()

	backend, err := net.Dial("tcp", upstream.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	clientSide, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer clientSide.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := clientSide.Accept()
		if err != nil {
			return
		}
		ev := core.NewEvent(nil, conn)
		ev.RR.CircleData = &proxy.Proxy{ProxyConn: backend}
		WebSocketHandler(ev)
	}()

	client, err := net.Dial("tcp", clientSide.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 4)
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "ping" {
		t.Fatalf("echo = %q", got)
	}
	client.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket handler did not return after the upstream closed")
	}
}

// CircleData 断言失败/上游连接缺失：必须安全关闭而不是空指针 panic
func TestWebSocketHandlerBadCircleData(t *testing.T) {
	for name, data := range map[string]interface{}{
		"nil":      nil,
		"wrong-t":  "not-a-proxy",
		"no-conn":  &proxy.Proxy{},
	} {
		t.Run(name, func(t *testing.T) {
			server, client := net.Pipe()
			t.Cleanup(func() { _ = client.Close() })
			ev := core.NewEvent(nil, server)
			ev.RR.CircleData = data

			done := make(chan struct{})
			go func() {
				defer close(done)
				WebSocketHandler(ev)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("handler hung on invalid circle data")
			}
			client.SetWriteDeadline(time.Now().Add(time.Second))
			if _, err := client.Write([]byte("x")); err == nil {
				t.Fatal("connection should be closed after invalid circle data")
			}
		})
	}
}

// 上游主动断开：两个搬运方向都要收尾退出
func TestWebSocketHandlerExitsWhenUpstreamCloses(t *testing.T) {
	backendClient, backendServer := net.Pipe()
	t.Cleanup(func() { _ = backendClient.Close() })

	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(nil, server)
	ev.RR.CircleData = &proxy.Proxy{ProxyConn: backendClient}

	done := make(chan struct{})
	go func() {
		defer close(done)
		WebSocketHandler(ev)
	}()

	// 上游关闭 → 客户端方向读错误 → handler 退出
	_ = backendServer.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not exit after upstream closed")
	}
}
