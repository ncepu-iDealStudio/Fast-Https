package main

import (
	"bytes"
	"encoding/json"
	initialization "fast-https/init"
	"fast-https/modules/core/server"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProxyHTTPIntegration(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jsp/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, "<main>hello %s</main>", r.URL.Query().Get("name"))
		case "/python/echo":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Upstream-Request-ID", r.Header.Get("X-Request-ID"))
			_ = json.NewEncoder(w).Encode(map[string]string{"body": string(body)})
		case "/static/site.css":
			w.Header().Set("Content-Type", "text/css")
			_, _ = io.WriteString(w, "body{color:#234}")
		case "/missing":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	port := reserveTCPPort(t)
	runtimeDir := t.TempDir()
	writeProxyRuntime(t, runtimeDir, port, backend.URL)

	cmd := exec.Command(os.Args[0], "-test.run=^TestProxyServerProcess$")
	cmd.Dir = runtimeDir
	cmd.Env = append(os.Environ(), "FASTHTTPS_PROXY_HELPER=1")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start isolated Fast-Https process: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitForHTTPServer(proxyURL, 15*time.Second); err != nil {
		t.Fatalf("Fast-Https did not become ready: %v\nserver output:\n%s", err, output.String())
	}

	client := &http.Client{Timeout: 5 * time.Second}
	t.Run("dynamic HTML upstream", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, proxyURL+"/jsp/page?name=proxy", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET through proxy: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%q", resp.StatusCode, http.StatusOK, body)
		}
		if got, want := resp.Header.Get("Content-Type"), "text/html; charset=utf-8"; got != want {
			t.Errorf("Content-Type = %q, want %q", got, want)
		}
		if got, want := string(body), "<main>hello proxy</main>"; got != want {
			t.Errorf("body = %q, want %q", got, want)
		}
	})

	t.Run("JSON API request body and headers", func(t *testing.T) {
		payload := `{"message":"hello from client"}`
		req, err := http.NewRequest(http.MethodPost, proxyURL+"/python/echo", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "proxy-e2e-42")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST through proxy: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%q", resp.StatusCode, http.StatusOK, body)
		}
		if got, want := resp.Header.Get("X-Upstream-Request-ID"), "proxy-e2e-42"; got != want {
			t.Errorf("upstream-observed request ID = %q, want %q", got, want)
		}
		var decoded struct {
			Body string `json:"body"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode upstream response: %v; body=%s", err, body)
		}
		if decoded.Body != payload {
			t.Errorf("upstream received body %q, want %q", decoded.Body, payload)
		}
	})

	t.Run("static asset response", func(t *testing.T) {
		resp, err := client.Get(proxyURL + "/static/site.css")
		if err != nil {
			t.Fatalf("GET static asset through proxy: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d; body=%q", resp.StatusCode, http.StatusOK, body)
		}
		if got, want := resp.Header.Get("Content-Type"), "text/css"; got != want {
			t.Errorf("Content-Type = %q, want %q", got, want)
		}
		if got, want := string(body), "body{color:#234}"; got != want {
			t.Errorf("body = %q, want %q", got, want)
		}
	})

	t.Run("upstream error response", func(t *testing.T) {
		resp, err := client.Get(proxyURL + "/missing")
		if err != nil {
			t.Fatalf("GET missing page through proxy: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want %d; body=%q", resp.StatusCode, http.StatusNotFound, body)
		}
		if !strings.Contains(string(body), "404 page not found") {
			t.Errorf("upstream error body was not preserved: %q", body)
		}
	})
}

func TestProxyServerProcess(t *testing.T) {
	if os.Getenv("FASTHTTPS_PROXY_HELPER") != "1" {
		t.Skip("helper process for TestProxyHTTPIntegration")
	}
	initialization.InitSystem()
	server.ServerInit().Run()
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve TCP port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func writeProxyRuntime(t *testing.T, root string, port int, upstream string) {
	t.Helper()
	for _, dir := range []string{"config", "config/cert", "httpdoc/root", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create runtime directory %s: %v", dir, err)
		}
	}
	config := fmt.Sprintf(`{
  "http": {
    "server": [{
      "listen": "%d",
      "server_name": "127.0.0.1",
      "location": [{
        "url": "/",
        "type": "proxy",
        "proxy_pass": %q
      }]
    }],
    "include": []
  }
}
`, port, upstream)
	writeRuntimeFile(t, filepath.Join(root, "config", "fast-https.json"), config)
	writeRuntimeFile(t, filepath.Join(root, "config", "mime.json"), `{}`)
}

func writeRuntimeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write runtime file %s: %v", path, err)
	}
}

func waitForHTTPServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url + "/jsp/page?name=ready")
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
	return fmt.Errorf("HTTP server at %s not ready: %w", url, lastErr)
}
