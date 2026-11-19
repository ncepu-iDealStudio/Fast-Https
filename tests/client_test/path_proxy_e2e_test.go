package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPathProxyE2E 验证「一个站点多个 location」的完整链路：
// 静态 / 读磁盘，/api 转发保留完整路径，/strip 按 nginx 规则去掉前缀，
// /assets 指向另一个静态目录。上游回显收到的请求行，用来断言转发路径。
func TestPathProxyE2E(t *testing.T) {
	up := startEchoUpstream(t)
	port := reserveTCPPort(t)
	runtimeDir := t.TempDir()
	writePathProxyRuntime(t, runtimeDir, port, "http://"+up.addr)

	proc, output := startCacheZoneServer(t, runtimeDir)
	defer stopCacheZoneServer(t, proc, port)
	if err := waitPortState(fmt.Sprintf("127.0.0.1:%d", port), true, 15*time.Second); err != nil {
		t.Fatalf("Fast-Https did not listen: %v\n%s", err, output.String())
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	cases := []struct {
		name string
		path string
		want string
	}{
		{"static root", "/hello.txt", "static-root"},
		{"static subdir location", "/assets/app.js", "static-assets"},
		{"proxy keeps full path", "/api/users?x=1", "/api/users?x=1"},
		{"proxy strips prefix", "/strip/users?x=1", "/users?x=1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := getBody(t, base+c.path); got != c.want {
				t.Fatalf("GET %s = %q, want %q\n%s", c.path, got, c.want, output.String())
			}
		})
	}
}

// writePathProxyRuntime 写多 location 站点：静态根、静态子目录、保留前缀的代理、
// 去掉前缀的代理。全部内联在主配置里，避免依赖 conf.d 路径改写。
func writePathProxyRuntime(t *testing.T, root string, port int, upstream string) {
	t.Helper()
	for _, dir := range []string{"config", "config/cert", "httpdoc/root", "httpdoc/assets", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create runtime directory %s: %v", dir, err)
		}
	}
	writeRuntimeFile(t, filepath.Join(root, "httpdoc", "root", "hello.txt"), "static-root")
	writeRuntimeFile(t, filepath.Join(root, "httpdoc", "assets", "app.js"), "static-assets")
	config := fmt.Sprintf(`{
  "http": {
    "server": [{
      "listen": "%d",
      "server_name": "127.0.0.1",
      "location": [
        {"url": "/", "type": "local", "root": "./httpdoc/root"},
        {"url": "/assets", "type": "local", "root": "./httpdoc/assets"},
        {"url": "/api", "type": "proxy", "proxy_pass": %q},
        {"url": "/strip", "type": "proxy", "proxy_pass": "%s/"}
      ]
    }],
    "include": []
  }
}
`, port, upstream, upstream)
	writeRuntimeFile(t, filepath.Join(root, "config", "fast-https.json"), config)
	writeRuntimeFile(t, filepath.Join(root, "config", "mime.json"), `{}`)
}

// echoUpstream 回显请求行（方法 + 目标），用来断言代理转发出去的路径。
type echoUpstream struct {
	ln   net.Listener
	addr string
}

func startEchoUpstream(t *testing.T) *echoUpstream {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	up := &echoUpstream{ln: ln, addr: ln.Addr().String()}
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
				line, err := br.ReadString('\n')
				if err != nil {
					return
				}
				fields := strings.Fields(line)
				body := "bad-request-line"
				if len(fields) >= 2 {
					body = fields[1] // 请求目标：方法后的第二段
				}
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
					len(body), body)
			}(conn)
		}
	}()
	return up
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: read body: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status = %d, body=%q", url, resp.StatusCode, body)
	}
	return string(body)
}
