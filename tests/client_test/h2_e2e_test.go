package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"

	"fast-https/cmd"
	initialization "fast-https/init"
	"fast-https/runtime/server"
)

func TestH2ServerProcess(t *testing.T) {
	if os.Getenv("FASTHTTPS_H2_HELPER") != "1" {
		t.Skip("helper process for TestH2Integration")
	}
	initialization.InitSystem()
	cmd.RegisterHandlers()
	cmd.RegisterServices()
	server.ServerInit().Run()
}

// TestH2Integration 用真实 TLS + ALPN 跑通 HTTP/2：
// 协议栈由 golang.org/x/net/http2 承担，响应经 static 模块写出。
func TestH2Integration(t *testing.T) {
	port := reserveTCPPort(t)
	runtimeDir := t.TempDir()
	writeH2Runtime(t, runtimeDir, port)

	proc := exec.Command(os.Args[0], "-test.run=^TestH2ServerProcess$")
	proc.Dir = runtimeDir
	proc.Env = append(os.Environ(), "FASTHTTPS_H2_HELPER=1")
	var output bytes.Buffer
	proc.Stdout = &output
	proc.Stderr = &output
	if err := proc.Start(); err != nil {
		t.Fatalf("start Fast-Https h2 process: %v", err)
	}
	t.Cleanup(func() {
		if proc.Process != nil {
			_ = proc.Process.Kill()
			_ = proc.Wait()
		}
	})

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // 自签名测试证书
				NextProtos:         []string{"h2"},
			},
		},
	}
	base := fmt.Sprintf("https://127.0.0.1:%d", port)
	if err := waitForH2Server(client, base+"/", 15*time.Second); err != nil {
		t.Fatalf("HTTP/2 server did not become ready: %v\nserver output:\n%s", err, output.String())
	}

	t.Run("negotiates h2 and serves static body", func(t *testing.T) {
		resp, err := client.Get(base + "/")
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		defer resp.Body.Close()

		if resp.ProtoMajor != 2 {
			t.Errorf("proto = %s, want HTTP/2", resp.Proto)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
			t.Errorf("content-type = %q, want text/html", got)
		}
		// HTTP/2 禁止逐跳首部，模块设置的 Connection 必须被过滤掉。
		if got := resp.Header.Get("Connection"); got != "" {
			t.Errorf("hop-by-hop header leaked to HTTP/2: Connection=%q", got)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(body), "h2-index") {
			t.Errorf("body = %q, want it to contain h2-index", body)
		}
	})

	t.Run("missing file returns 404 over h2", func(t *testing.T) {
		resp, err := client.Get(base + "/not-exists.html")
		if err != nil {
			t.Fatalf("GET missing: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})
}

func waitForH2Server(client *http.Client, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("readiness request returned status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("h2 server at %s not ready: %w", url, lastErr)
}

func writeH2Runtime(t *testing.T, root string, port int) {
	t.Helper()
	for _, dir := range []string{"config/cert", "httpdoc/root", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create runtime directory %s: %v", dir, err)
		}
	}

	writeRuntimeFile(t, filepath.Join(root, "httpdoc", "root", "index.html"), "<h1>h2-index</h1>")
	writeH2Cert(t, filepath.Join(root, "config", "cert"))

	config := fmt.Sprintf(`{
  "http": {
    "server": [{
      "listen": "%d ssl h2",
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
`, port)
	writeRuntimeFile(t, filepath.Join(root, "config", "fast-https.json"), config)
	writeRuntimeFile(t, filepath.Join(root, "config", "mime.json"), `{"html":"text/html; charset=utf-8"}`)
}

func writeH2Cert(t *testing.T, dir string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	crt := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(filepath.Join(dir, "site.pem"), crt, 0o644); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "site-key.pem"), keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}
