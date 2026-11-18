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
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestProxyCacheZoneE2E 走完整链路验证缓存区：
// 客户端 → 真实 Fast-Https 进程 → 上游。上游每次被访问都会返回不同的正文，
// 因此只要正文不变、上游命中次数不增加，就证明响应来自磁盘缓存。
// 同时覆盖 no-cache 绕过、POST 不缓存、重启后从缓存区目录恢复。
func TestProxyCacheZoneE2E(t *testing.T) {
	up := startCacheZoneUpstream(t)

	port := reserveTCPPort(t)
	runtimeDir := t.TempDir()
	writeCacheZoneRuntime(t, runtimeDir, port, "http://"+up.addr)

	proc, output := startCacheZoneServer(t, runtimeDir)
	defer stopCacheZoneServer(t, proc, port)

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitPortState(fmt.Sprintf("127.0.0.1:%d", port), true, 15*time.Second); err != nil {
		t.Fatalf("Fast-Https did not listen: %v\n%s", err, output.String())
	}

	// 第一次回源，写入缓存。落盘是异步队列，给写协程一点时间。
	if body := cacheZoneRequest(t, base+"/page", "", http.MethodGet); body != "hit-1" {
		t.Fatalf("first response = %q, want hit-1\n%s", body, output.String())
	}
	time.Sleep(300 * time.Millisecond)

	// 第二次应当直接命中缓存，上游命中次数保持 1。
	if body := cacheZoneRequest(t, base+"/page", "", http.MethodGet); body != "hit-1" {
		t.Fatalf("cached response = %q, want hit-1", body)
	}
	if got := up.hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d after cached request, want 1", got)
	}

	// 客户端显式 no-cache：必须回源并拿到新内容。
	if body := cacheZoneRequest(t, base+"/page", "no-cache", http.MethodGet); body != "hit-2" {
		t.Fatalf("no-cache response = %q, want hit-2", body)
	}
	time.Sleep(300 * time.Millisecond)

	// POST 不参与缓存：再次回源。
	if body := cacheZoneRequest(t, base+"/page", "", http.MethodPost); body != "hit-3" {
		t.Fatalf("POST response = %q, want hit-3", body)
	}

	// 重启进程后按缓存区目录恢复索引：命中上一次写入的 hit-2，不再回源。
	stopCacheZoneServer(t, proc, port)
	proc, output = startCacheZoneServer(t, runtimeDir)
	defer func() {
		if proc != nil && proc.Process != nil {
			_ = proc.Process.Kill()
			_ = proc.Wait()
		}
	}()
	if err := waitPortState(fmt.Sprintf("127.0.0.1:%d", port), true, 15*time.Second); err != nil {
		t.Fatalf("Fast-Https did not listen after restart: %v\n%s", err, output.String())
	}
	if body := cacheZoneRequest(t, base+"/page", "", http.MethodGet); body != "hit-2" {
		t.Fatalf("after restart response = %q, want hit-2 (recovered from cache dir)", body)
	}
	if got := up.hits.Load(); got != 3 {
		t.Fatalf("upstream hits after restart = %d, want 3 (no new upstream request)", got)
	}
}

// cacheZoneRequest 发一次请求并读回正文。header 非空时作为 Cache-Control。
func cacheZoneRequest(t *testing.T, url, cacheControl, method string) string {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cacheControl != "" {
		req.Header.Set("Cache-Control", cacheControl)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", method, url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s %s: status = %d, body=%q", method, url, resp.StatusCode, body)
	}
	return string(body)
}

type cacheZoneUpstream struct {
	ln   net.Listener
	addr string
	hits atomic.Int64
}

// startCacheZoneUpstream 起一个每次响应都不同的上游，并统计真实命中次数。
func startCacheZoneUpstream(t *testing.T) *cacheZoneUpstream {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	up := &cacheZoneUpstream{ln: ln, addr: ln.Addr().String()}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if line == "\r\n" || line == "\n" {
						break
					}
				}
				body := fmt.Sprintf("hit-%d", up.hits.Add(1))
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
					len(body), body)
			}(conn)
		}
	}()
	return up
}

// writeCacheZoneRuntime 写一个引用缓存区 edge 的代理站点。
func writeCacheZoneRuntime(t *testing.T, root string, port int, upstream string) {
	t.Helper()
	for _, dir := range []string{"config", "config/cert", "httpdoc/root", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create runtime directory %s: %v", dir, err)
		}
	}
	config := fmt.Sprintf(`{
  "cache": [
    {"name": "edge", "path": "./cache", "valid": ["200:60s"], "max_size": 0}
  ],
  "http": {
    "server": [{
      "listen": "%d",
      "server_name": "127.0.0.1",
      "location": [{
        "url": "/", "type": "proxy", "proxy_pass": %q, "proxy_cache": "edge"
      }]
    }],
    "include": []
  }
}
`, port, upstream)
	writeRuntimeFile(t, filepath.Join(root, "config", "fast-https.json"), config)
	writeRuntimeFile(t, filepath.Join(root, "config", "mime.json"), `{}`)
}

func startCacheZoneServer(t *testing.T, dir string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	proc := exec.Command(os.Args[0], "-test.run=^TestProxyServerProcess$")
	proc.Dir = dir
	proc.Env = append(os.Environ(), "FASTHTTPS_PROXY_HELPER=1")
	var output bytes.Buffer
	proc.Stdout = &output
	proc.Stderr = &output
	if err := proc.Start(); err != nil {
		t.Fatalf("start isolated Fast-Https process: %v", err)
	}
	return proc, &output
}

func stopCacheZoneServer(t *testing.T, proc *exec.Cmd, port int) {
	t.Helper()
	if proc == nil || proc.Process == nil {
		return
	}
	_ = proc.Process.Kill()
	_ = proc.Wait()
	if err := waitPortState(fmt.Sprintf("127.0.0.1:%d", port), false, 10*time.Second); err != nil {
		t.Fatalf("port did not close after stop: %v", err)
	}
}
