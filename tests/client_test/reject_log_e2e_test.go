package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRejectLogE2E 走真实进程验证 R-10：请求被拒绝后写结构化日志，
// 再用 `logs query --reject-reason` 从日志文件里查回来。
//
// 覆盖路径穿越、header 超限、body 超限、压缩跳过与未知 SNI。
func TestRejectLogE2E(t *testing.T) {
	if os.Getenv("FASTHTTPS_E2E_REJECT_LOG") != "1" {
		t.Skip("set FASTHTTPS_E2E_REJECT_LOG=1 to run reject log e2e")
	}
	if runtimeBusy() {
		t.Fatal("127.0.0.1:9088 is already in use; stop the other Fast-Https instance before this e2e")
	}

	repo, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	for _, dir := range []string{"config/cert", "config/conf.d", "httpdoc/root", "logs"} {
		if err := os.MkdirAll(filepath.Join(work, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyFile(filepath.Join(repo, "config", "mime.json"), filepath.Join(work, "config", "mime.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "httpdoc", "root", "index.html"), []byte("<html>reject-log-e2e</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	httpPort := reserveTCPPort(t)
	tlsPort := reserveTCPPort(t)
	writeSelfSigned(t, work, "known.example")
	writeRejectSite(t, work, httpPort, tlsPort)

	bin := filepath.Join(work, "fast-https-reject-log-e2e.exe")
	build := exec.Command("go", "build", "-o", bin, "fast-https.go")
	build.Dir = repo
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	proc, procLog := startRuntimeServer(t, bin, work)
	t.Cleanup(func() { stopRuntimeWork(bin, work, proc) })

	httpAddr := fmt.Sprintf("127.0.0.1:%d", httpPort)
	if err := waitPortState(httpAddr, true, 30*time.Second); err != nil {
		t.Fatalf("http port: %v\n%s", err, procLog.String())
	}
	if err := waitPortState(fmt.Sprintf("127.0.0.1:%d", tlsPort), true, 30*time.Second); err != nil {
		t.Fatalf("tls port: %v\n%s", err, procLog.String())
	}

	// 1) 路径穿越：原始请求行保证 ../ 不会被客户端库规范化掉。
	if status := rawStatus(t, httpAddr, "GET /../etc/passwd HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n"); !strings.Contains(status, " 404 ") {
		t.Fatalf("traversal status = %q, want 404", status)
	}

	// 2) header 超限（max_header_size = 512）。
	bigHeader := "GET / HTTP/1.1\r\nHost: 127.0.0.1\r\nX-Big: " + strings.Repeat("a", 1024) + "\r\n\r\n"
	if status := rawStatus(t, httpAddr, bigHeader); !strings.Contains(status, " 431 ") {
		t.Fatalf("big header status = %q, want 431", status)
	}

	// 3) body 超限（max_body_size = 16）。
	bigBody := "POST / HTTP/1.1\r\nHost: 127.0.0.1\r\nContent-Length: 64\r\n\r\n" + strings.Repeat("b", 64)
	if status := rawStatus(t, httpAddr, bigBody); !strings.Contains(status, " 413 ") {
		t.Fatalf("big body status = %q, want 413", status)
	}

	// 4) 压缩跳过：zip_min_length 远大于文件大小，debug 日志记 zip_min_length。
	req, err := http.NewRequest(http.MethodGet, "http://"+httpAddr+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{DisableCompression: true},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("compression request: %v\n%s", err, procLog.String())
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("compression request status = %d", resp.StatusCode)
	}

	// 5) 未知 SNI：握手回落到第一张证书，控制台 warn 镜像进 error.log。
	conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", tlsPort), &tls.Config{
		ServerName:         "unknown.example",
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("unknown sni handshake: %v\n%s", err, procLog.String())
	}
	_ = conn.Close()

	query := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = work
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"path_traversal", []string{"logs", "query", "--kind", "safe", "--reject-reason", "path_traversal", "--json"},
			[]string{`"reject_reason":"path_traversal"`, `"rule":"static_root"`, `"remote_ip":"127.0.0.1"`}},
		{"header_too_large", []string{"logs", "query", "--kind", "safe", "--reject-reason", "header_too_large", "--json"},
			[]string{`"reject_reason":"header_too_large"`, `"rule":"max_header_size"`, `"remote_ip":"127.0.0.1"`}},
		{"body_too_large", []string{"logs", "query", "--kind", "safe", "--reject-reason", "body_too_large", "--json"},
			[]string{`"reject_reason":"body_too_large"`, `"rule":"max_body_size"`, `"remote_ip":"127.0.0.1"`}},
		{"compression_skipped", []string{"logs", "query", "--kind", "safe", "--reject-reason", "compression_skipped", "--json"},
			[]string{`"reject_reason":"compression_skipped"`, `"rule":"zip_min_length"`}},
		{"unknown_sni", []string{"logs", "query", "--kind", "error", "--reject-reason", "unknown_sni", "--json"},
			[]string{`"reject_reason":"unknown_sni"`, `"rule":"tls.sni"`, `"remote_ip":"127.0.0.1"`}},
	} {
		out := waitQuery(t, query, tc.args, tc.want...)
		if tc.name == "path_traversal" && strings.Contains(out, "etc/passwd") {
			t.Fatalf("拒绝日志不应记录原始路径：%s", out)
		}
	}
}

// waitQuery 轮询查询命令直到命中全部字段：日志落盘与请求返回之间
// 有极短的写文件窗口，避免偶发空结果。
func waitQuery(t *testing.T, query func(...string) string, args []string, want ...string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for {
		last = query(args...)
		ok := true
		for _, w := range want {
			if !strings.Contains(last, w) {
				ok = false
				break
			}
		}
		if ok {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("query %v missing %v:\n%s", args, want, last)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// rawStatus 发一条原始 HTTP 请求并返回状态行。
func rawStatus(t *testing.T, addr, request string) string {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	return strings.TrimSpace(line)
}

func writeRejectSite(t *testing.T, work string, httpPort, tlsPort int) {
	t.Helper()
	body := fmt.Sprintf(`{
  "log_root": "./logs",
  "http": {
    "servers_limit": { "max_header_size": 512, "max_body_size": 16 },
    "server": [
      {
        "listen": "%d",
        "server_name": "127.0.0.1",
        "location": [{
          "url": "/",
          "type": "local",
          "root": "./httpdoc/root",
          "index": ["index.html"],
          "zip": ["gzip"],
          "zip_min_length": 1000000
        }]
      },
      {
        "listen": "%d ssl",
        "server_name": "known.example",
        "ssl_certificate": "./config/cert/site.pem",
        "ssl_certificate_key": "./config/cert/site-key.pem",
        "location": [{
          "url": "/",
          "type": "local",
          "root": "./httpdoc/root",
          "index": ["index.html"]
        }]
      }
    ],
    "include": []
  }
}
`, httpPort, tlsPort)
	if err := os.WriteFile(filepath.Join(work, "config", "fast-https.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeSelfSigned 在 work/config/cert 下生成一张只含指定域名的自签证书。
func writeSelfSigned(t *testing.T, work, name string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		DNSNames:              []string{name},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	certPath := filepath.Join(work, "config", "cert", "site.pem")
	keyPath := filepath.Join(work, "config", "cert", "site-key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
}
