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

	"fast-https/management/rtclient"
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

func baseSnap() rtclient.Snapshot {
	return rtclient.Snapshot{
		SchemaVersion: rtclient.SchemaVersion,
		Server: rtclient.ServerInfo{
			State:            "running",
			PID:              7,
			StartedAt:        "2026-11-08T12:00:00.000Z",
			ConfigGeneration: 1,
		},
		Listeners: []rtclient.ListenerInfo{{Port: "18080"}},
	}
}

func TestApplyRuntimeVerification(t *testing.T) {
	path := writeReloadCfg(t, 18080)
	base := baseSnap()

	t.Run("generation bump", func(t *testing.T) {
		n := 0
		res, err := Reload(context.Background(), Options{
			ConfigPath: path,
			PID:        7,
			Timeout:    time.Second,
			Probe:      func(int) bool { return true },
			Bind:       func(int) error { return nil },
			Alive:      func(int) bool { return true },
			Signal:     func(int) error { return nil },
			Sleep:      func(time.Duration) {},
			Snapshot: func(context.Context) (rtclient.Snapshot, error) {
				n++
				s := base
				if n > 1 {
					s.Server.ConfigGeneration = 2
				}
				return s, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Verification.Level != "runtime" || !res.Verification.ReloadConfirmed {
			t.Fatalf("verification = %+v", res.Verification)
		}
	})

	t.Run("unchanged generation", func(t *testing.T) {
		_, err := Reload(context.Background(), Options{
			ConfigPath: path,
			PID:        7,
			Timeout:    50 * time.Millisecond,
			Probe:      func(int) bool { return true },
			Bind:       func(int) error { return nil },
			Alive:      func(int) bool { return true },
			Signal:     func(int) error { return nil },
			Sleep:      func(time.Duration) {},
			Snapshot:   func(context.Context) (rtclient.Snapshot, error) { return base, nil },
		})
		if !errors.Is(err, ErrReloadNotApplied) {
			t.Fatalf("err = %v, want ErrReloadNotApplied", err)
		}
	})

	t.Run("started_at changed", func(t *testing.T) {
		n := 0
		_, err := Reload(context.Background(), Options{
			ConfigPath: path,
			PID:        7,
			Timeout:    time.Second,
			Probe:      func(int) bool { return true },
			Bind:       func(int) error { return nil },
			Alive:      func(int) bool { return true },
			Signal:     func(int) error { return nil },
			Sleep:      func(time.Duration) {},
			Snapshot: func(context.Context) (rtclient.Snapshot, error) {
				n++
				s := base
				if n > 1 {
					s.Server.StartedAt = "2026-11-08T12:01:00.000Z"
					s.Server.ConfigGeneration = 2
				}
				return s, nil
			},
		})
		if !errors.Is(err, ErrReloadedAsRestart) {
			t.Fatalf("err = %v, want ErrReloadedAsRestart", err)
		}
	})
}

func TestReloadFallsBackToBasicWithoutSnapshot(t *testing.T) {
	path := writeReloadCfg(t, 18080)
	res, err := Reload(context.Background(), Options{
		ConfigPath: path,
		PID:        7,
		Timeout:    time.Second,
		Probe:      func(int) bool { return true },
		Bind:       func(int) error { return nil },
		Alive:      func(int) bool { return true },
		Signal:     func(int) error { return nil },
		Sleep:      func(time.Duration) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Verification.Level != "basic" || res.Verification.ReloadConfirmed {
		t.Fatalf("verification = %+v", res.Verification)
	}
}
