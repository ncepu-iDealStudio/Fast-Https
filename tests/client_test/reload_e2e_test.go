package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReloadPortSwitchE2E(t *testing.T) {
	if os.Getenv("FASTHTTPS_E2E_RELOAD") != "1" {
		t.Skip("set FASTHTTPS_E2E_RELOAD=1 to run reload e2e test")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root failed: %v", err)
	}

	configPath := filepath.Join(repoRoot, "config", "fast-https.json")
	originalConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read original config failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(configPath, originalConfig, 0o644)
	})

	initialConfig := fmt.Sprintf(`{
  "http": {
    "server": [
      {
        "listen": %d,
        "server_name": "localhost",
        "location": [
          {
            "url": "/",
            "type": "local",
            "root": "./httpdoc/root",
            "index": ["index.html"]
          }
        ]
      }
    ],
    "include": []
  }
}
`, 19080)

	if err := os.WriteFile(configPath, []byte(initialConfig), 0o644); err != nil {
		t.Fatalf("write initial config failed: %v", err)
	}

	serverCmd, outBuf, err := startDevServer(repoRoot)
	if err != nil {
		t.Fatalf("start dev server failed: %v", err)
	}
	t.Cleanup(func() {
		_ = stopServer(repoRoot)
		if serverCmd.ProcessState == nil || !serverCmd.ProcessState.Exited() {
			_ = serverCmd.Process.Kill()
		}
	})

	if err := waitPortState("127.0.0.1:19080", true, 20*time.Second); err != nil {
		t.Fatalf("initial port not ready: %v\nserver output:\n%s", err, outBuf.String())
	}

	if serverCmd.ProcessState != nil && serverCmd.ProcessState.Exited() {
		t.Fatalf("server exited before reload\n%s", outBuf.String())
	}
	if err := httpGetResponds("http://127.0.0.1:19080/"); err != nil {
		t.Fatalf("initial endpoint check failed: %v", err)
	}

	reloadConfig := fmt.Sprintf(`{
  "http": {
    "server": [
      {
        "listen": %d,
        "server_name": "localhost",
        "location": [
          {
            "url": "/",
            "type": "local",
            "root": "./httpdoc/root",
            "index": ["index.html"]
          }
        ]
      }
    ],
    "include": []
  }
}
`, 19081)
	if err := os.WriteFile(configPath, []byte(reloadConfig), 0o644); err != nil {
		t.Fatalf("write reload config failed: %v", err)
	}

	if err := runCommand(repoRoot, 20*time.Second, "go", "run", "fast-https.go", "reload"); err != nil {
		t.Fatalf("reload command failed: %v\nserver output:\n%s", err, outBuf.String())
	}

	if err := waitPortState("127.0.0.1:19081", true, 20*time.Second); err != nil {
		t.Fatalf("reloaded port not ready: %v\nserver output:\n%s", err, outBuf.String())
	}
	if err := waitPortState("127.0.0.1:19080", false, 20*time.Second); err != nil {
		t.Fatalf("old port should be closed after reload: %v\nserver output:\n%s", err, outBuf.String())
	}

	if err := httpGetResponds("http://127.0.0.1:19081/"); err != nil {
		t.Fatalf("reloaded endpoint check failed: %v", err)
	}
}

func startDevServer(repoRoot string) (*exec.Cmd, *bytes.Buffer, error) {
	bin := filepath.Join(os.TempDir(), "fast-https-reload-e2e.exe")
	build := exec.Command("go", "build", "-o", bin, "fast-https.go")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		return nil, nil, fmt.Errorf("build server: %w; %s", err, out)
	}
	cmd := exec.Command(bin, "dev")
	cmd.Dir = repoRoot
	setReloadProcessGroup(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return cmd, &out, nil
}

func stopServer(repoRoot string) error {
	return runCommand(repoRoot, 20*time.Second, "go", "run", "fast-https.go", "stop")
}

func runCommand(dir string, timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command timeout: %s %s", name, strings.Join(args, " "))
	}
	if err != nil {
		return fmt.Errorf("command failed: %w; output: %s", err, string(out))
	}
	return nil
}

func waitPortState(addr string, shouldOpen bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		isOpen := err == nil
		if conn != nil {
			_ = conn.Close()
		}
		if isOpen == shouldOpen {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if shouldOpen {
		return fmt.Errorf("port %s did not open before timeout", addr)
	}
	return fmt.Errorf("port %s did not close before timeout", addr)
}

func httpGetResponds(url string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	return nil
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	current := cwd
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
		current = next
	}
	return "", errors.New("cannot locate repository root from current working directory")
}
