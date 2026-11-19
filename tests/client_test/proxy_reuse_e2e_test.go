package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

// TestProxyEmptyBodyReuseE2E 验证 R-12 的完整链路：
// 客户端 → 真实 Fast-Https 进程 → 上游。上游两次都返回没有 Content-Length
// 的 204，Fast-Https 必须复用同一条上游连接（accept 只发生一次）。
func TestProxyEmptyBodyReuseE2E(t *testing.T) {
	up := startEmptyBodyUpstream(t)

	port := reserveTCPPort(t)
	runtimeDir := t.TempDir()
	writeProxyRuntime(t, runtimeDir, port, "http://"+up.addr)

	proc := exec.Command(os.Args[0], "-test.run=^TestProxyServerProcess$")
	proc.Dir = runtimeDir
	proc.Env = append(os.Environ(), "FASTHTTPS_PROXY_HELPER=1")
	var output bytes.Buffer
	proc.Stdout = &output
	proc.Stderr = &output
	if err := proc.Start(); err != nil {
		t.Fatalf("start isolated Fast-Https process: %v", err)
	}
	t.Cleanup(func() {
		if proc.Process != nil {
			_ = proc.Process.Kill()
			_ = proc.Wait()
		}
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if err := waitPortState(addr, true, 15*time.Second); err != nil {
		t.Fatalf("Fast-Https did not listen: %v\n%s", err, output.String())
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 2; i++ {
		resp, err := client.Get("http://" + addr + "/")
		if err != nil {
			t.Fatalf("request %d: %v\n%s", i+1, err, output.String())
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			t.Fatalf("request %d: read body: %v", i+1, readErr)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("request %d: status = %d, want 204; body=%q\n%s", i+1, resp.StatusCode, body, output.String())
		}
		if len(body) != 0 {
			t.Errorf("request %d: body = %q, want empty", i+1, body)
		}
	}

	if got := up.accepts.Load(); got != 1 {
		t.Fatalf("upstream accepted %d connections, want 1 (empty-body response must be reused)", got)
	}
}

type emptyBodyUpstream struct {
	ln      net.Listener
	addr    string
	accepts atomic.Int64
}

// startEmptyBodyUpstream 起一个只回 204 No Content 的上游，并统计 accept 次数。
func startEmptyBodyUpstream(t *testing.T) *emptyBodyUpstream {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	up := &emptyBodyUpstream{ln: ln, addr: ln.Addr().String()}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			up.accepts.Add(1)
			go serveEmptyBodyUpstream(c)
		}
	}()
	return up
}

func serveEmptyBodyUpstream(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	for {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\r\n" {
				break
			}
		}
		if _, err := c.Write([]byte("HTTP/1.1 204 No Content\r\n\r\n")); err != nil {
			return
		}
	}
}
