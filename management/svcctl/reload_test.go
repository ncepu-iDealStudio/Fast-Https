package svcctl

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func writeReloadCfg(t *testing.T, port int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fast-https.json")
	body := `{
  "http": {
    "server": [{
      "listen": ` + strconv.Itoa(port) + `,
      "server_name": "localhost",
      "location": [{"url": "/", "type": "local", "root": "."}]
    }]
  }
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReloadPortPrecheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	path := writeReloadCfg(t, port)

	signaled := false
	_, err = Reload(context.Background(), Options{
		ConfigPath: path,
		PID:        1,
		DryRun:     false,
		Timeout:    50 * time.Millisecond,
		Probe:      func(int) bool { return false },
		Alive:      func(int) bool { return true },
		Signal: func(int) error {
			signaled = true
			return nil
		},
		Sleep: func(time.Duration) {},
	})
	var pe *PortError
	if !errors.As(err, &pe) || pe.Port != port {
		t.Fatalf("err = %v, want PortError %d", err, port)
	}
	if signaled {
		t.Fatal("端口占用时不应发信号")
	}
}

func TestReloadDryRunDoesNotSignal(t *testing.T) {
	path := writeReloadCfg(t, 18080)
	signaled := false
	res, err := Reload(context.Background(), Options{
		ConfigPath: path,
		PID:        1,
		DryRun:     true,
		Probe:      func(int) bool { return true },
		Alive:      func(int) bool { return true },
		Bind:       func(int) error { return nil },
		Signal: func(int) error {
			signaled = true
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || signaled {
		t.Fatalf("dry-run 不应发信号：%+v signaled=%v", res, signaled)
	}
	if res.Verification.Level != "basic" || res.Verification.ReloadConfirmed {
		t.Fatalf("verification = %+v", res.Verification)
	}
}

func TestReloadNotRunning(t *testing.T) {
	path := writeReloadCfg(t, 18081)
	_, err := Reload(context.Background(), Options{
		ConfigPath: path,
		PidPath:    filepath.Join(t.TempDir(), "missing.pid"),
		Probe:      func(int) bool { return true },
		Bind:       func(int) error { return nil },
	})
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("err = %v", err)
	}
}
