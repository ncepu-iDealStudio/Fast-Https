package proxy_tcp

import (
	"fast-https/modules/core"
	"io"
	"net"
	"testing"
	"time"
)

func TestProxyEventTcpEcho(t *testing.T) {
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
		ProxyEventTcp(core.NewEvent(nil, conn), upstream.Addr().String())
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
		t.Fatal("tcp proxy did not return after the upstream closed")
	}
}
