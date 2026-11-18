package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

// TestH2ToggleE2E 在真实进程上验证 listen 里的 h2 开关：
// 只写 "ssl" 时 ALPN 不出现 h2，连接保持在 HTTP/1.1；
// 写 "ssl h2" 才协商 h2。站点文件由 site add 原样写入监听串。
func TestH2ToggleE2E(t *testing.T) {
	cases := []struct {
		name      string
		listen    string
		wantProto string
		wantMajor int
	}{
		{"ssl only keeps http/1.1", "ssl", "", 1},
		{"ssl h2 negotiates h2", "ssl h2", "h2", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			port := reserveTCPPort(t)
			runtimeDir := t.TempDir()
			writeH2ToggleRuntime(t, runtimeDir, port, c.listen)
			proc, output := startH2ToggleServer(t, runtimeDir)
			defer func() {
				if proc != nil && proc.Process != nil {
					_ = proc.Process.Kill()
					_ = proc.Wait()
				}
			}()

			addr := fmt.Sprintf("127.0.0.1:%d", port)
			proto, err := waitALPN(addr, 15*time.Second)
			if err != nil {
				t.Fatalf("TLS server did not become ready: %v\nserver output:\n%s", err, output.String())
			}
			if proto != c.wantProto {
				t.Fatalf("negotiated protocol = %q, want %q\nserver output:\n%s", proto, c.wantProto, output.String())
			}

			resp, body := getOverTLS(t, addr, c.wantProto)
			if resp.ProtoMajor != c.wantMajor {
				t.Errorf("response proto = %s, want major %d", resp.Proto, c.wantMajor)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200; body=%q", resp.StatusCode, body)
			}
			if body != "<h1>h2-toggle</h1>" {
				t.Errorf("body = %q", body)
			}
		})
	}
}

// waitALPN 反复 TLS 握手直到服务端就绪，返回协商出的 ALPN 协议。
func waitALPN(addr string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr, &tls.Config{
			InsecureSkipVerify: true, // 自签名测试证书
			NextProtos:         []string{"h2", "http/1.1"},
		})
		if err == nil {
			proto := conn.ConnectionState().NegotiatedProtocol
			_ = conn.Close()
			return proto, nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	return "", lastErr
}

// getOverTLS 按协商结果选 h1 或 h2 客户端，取回首页正文。
func getOverTLS(t *testing.T, addr, proto string) (*http.Response, string) {
	t.Helper()
	url := "https://" + addr + "/"
	var transport http.RoundTripper
	if proto == "h2" {
		transport = &http2.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}},
		}
	} else {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"http/1.1"}},
		}
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(body)
}

func startH2ToggleServer(t *testing.T, dir string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	proc := exec.Command(os.Args[0], "-test.run=^TestH2ServerProcess$")
	proc.Dir = dir
	proc.Env = append(os.Environ(), "FASTHTTPS_H2_HELPER=1")
	var output bytes.Buffer
	proc.Stdout = &output
	proc.Stderr = &output
	if err := proc.Start(); err != nil {
		t.Fatalf("start Fast-Https process: %v", err)
	}
	return proc, &output
}

// writeH2ToggleRuntime 写一个监听串由调用方决定的 TLS 站点。
func writeH2ToggleRuntime(t *testing.T, root string, port int, listen string) {
	t.Helper()
	for _, dir := range []string{"config/cert", "httpdoc/root", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create runtime directory %s: %v", dir, err)
		}
	}
	writeRuntimeFile(t, filepath.Join(root, "httpdoc", "root", "index.html"), "<h1>h2-toggle</h1>")
	writeH2Cert(t, filepath.Join(root, "config", "cert"))

	config := fmt.Sprintf(`{
  "http": {
    "server": [{
      "listen": "%d %s",
      "server_name": "127.0.0.1",
      "ssl_certificate": "config/cert/site.pem",
      "ssl_certificate_key": "config/cert/site-key.pem",
      "location": [{
        "url": "/",
        "type": "local",
        "root": "./httpdoc/root",
        "index": ["index.html"]
      }]
    }],
    "include": []
  }
}
`, port, listen)
	writeRuntimeFile(t, filepath.Join(root, "config", "fast-https.json"), config)
	writeRuntimeFile(t, filepath.Join(root, "config", "mime.json"), `{"html":"text/html; charset=utf-8"}`)
}
