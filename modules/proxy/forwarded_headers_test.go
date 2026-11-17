package proxy

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// echoUpstream 起一个回环上游，把收到的请求头原样回显给测试。
// 返回上游地址与「取回显内容」的通道。
func echoUpstream(t *testing.T) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	got := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		buf := bufio.NewReader(conn)
		var sb strings.Builder
		for {
			line, err := buf.ReadString('\n')
			sb.WriteString(line)
			if err != nil || strings.TrimSpace(line) == "" {
				break
			}
		}
		got <- sb.String()
	}()
	return ln.Addr().String(), got
}
