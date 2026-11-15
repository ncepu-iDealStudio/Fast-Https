package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigDefaultsWhenFileMissing(t *testing.T) {
	t.Setenv(EnvDir, "")
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Interval.D() != DefaultInterval || cfg.Timeout.D() != DefaultTimeout {
		t.Errorf("interval/timeout = %s/%s", cfg.Interval.D(), cfg.Timeout.D())
	}
	if cfg.SnapshotURL != DefaultSnapshotURL {
		t.Errorf("snapshot_url = %q", cfg.SnapshotURL)
	}
	if cfg.EventKeep != DefaultEventKeep || cfg.EventMaxBytes != DefaultEventMaxBytes {
		t.Errorf("rotation = %d/%d", cfg.EventKeep, cfg.EventMaxBytes)
	}
	if !cfg.PortProbe || !cfg.StuckHint {
		t.Errorf("optional rules = %v/%v, want both on by default", cfg.PortProbe, cfg.StuckHint)
	}
}

// 两个可选规则默认开，写了 false 才是关掉。
func TestConfigOptionalRulesCanBeDisabled(t *testing.T) {
	t.Setenv(EnvDir, "")

	on, err := LoadConfig(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !on.PortProbe || !on.StuckHint {
		t.Fatalf("defaults = %v/%v, want both on", on.PortProbe, on.StuckHint)
	}

	path := filepath.Join(t.TempDir(), "monitor.json")
	if err := os.WriteFile(path, []byte(`{"port_probe": false, "stuck_hint": false}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	off, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if off.PortProbe || off.StuckHint {
		t.Fatalf("explicit false was ignored: %v/%v", off.PortProbe, off.StuckHint)
	}
}

// Monitor 只读自己的配置文件：站点配置里的字段一律忽略，也不去读 fast-https.json。
func TestConfigSeparate(t *testing.T) {
	t.Setenv(EnvDir, "")
	dir := t.TempDir()
	// 同目录放一份坏掉的站点配置，确认它不会被读到。
	site := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(site, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write site config: %v", err)
	}

	monitorDir := filepath.ToSlash(filepath.Join(dir, "m"))
	pidPath := filepath.ToSlash(filepath.Join(dir, "fast-https.pid"))
	path := filepath.Join(dir, "monitor.json")
	body := fmt.Sprintf(`{
  "interval": "2s",
  "timeout": "500ms",
  "snapshot_url": "http://127.0.0.1:9999/runtime",
  "dir": %q,
  "pid_file": %q,
  "event_keep": 5,
  "http": {"server": [{"listen": "443 ssl"}]},
  "site": [{"server_name": "example.com"}]
}`, monitorDir, pidPath)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write monitor config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Interval.D() != 2*time.Second || cfg.Timeout.D() != 500*time.Millisecond {
		t.Errorf("interval/timeout = %s/%s", cfg.Interval.D(), cfg.Timeout.D())
	}
	if cfg.Dir != monitorDir {
		t.Errorf("dir = %q, want %q", cfg.Dir, monitorDir)
	}
	if cfg.PidFile != pidPath {
		t.Errorf("pid_file = %q, want %q", cfg.PidFile, pidPath)
	}
	if cfg.EventKeep != 5 {
		t.Errorf("event_keep = %d, want 5", cfg.EventKeep)
	}
}

func TestConfigEnvDirAppliesOnlyWithoutFileValue(t *testing.T) {
	envDir := filepath.Join(t.TempDir(), "from-env")
	t.Setenv(EnvDir, envDir)

	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Dir != envDir {
		t.Errorf("dir = %q, want the env value %q", cfg.Dir, envDir)
	}

	fileDir := filepath.Join(t.TempDir(), "from-file")
	path := filepath.Join(t.TempDir(), "monitor.json")
	body := fmt.Sprintf(`{"dir": %q}`, filepath.ToSlash(fileDir))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err = LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if filepath.Clean(cfg.Dir) != filepath.Clean(fileDir) {
		t.Errorf("dir = %q, want the config file value %q", cfg.Dir, fileDir)
	}
}

// 字段缺失或写成空值等于「没设置」，回落到默认值，不当作配置错误。
func TestConfigBlankValuesFallBackToDefaults(t *testing.T) {
	t.Setenv(EnvDir, "")
	path := filepath.Join(t.TempDir(), "monitor.json")
	body := `{"dir": "   ", "event_keep": 0, "event_max_bytes": 0, "snapshot_url": ""}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Dir != DefaultDir || cfg.SnapshotURL != DefaultSnapshotURL {
		t.Errorf("dir/url = %q/%q, want defaults", cfg.Dir, cfg.SnapshotURL)
	}
	if cfg.EventKeep != DefaultEventKeep || cfg.EventMaxBytes != DefaultEventMaxBytes {
		t.Errorf("rotation = %d/%d, want defaults", cfg.EventKeep, cfg.EventMaxBytes)
	}
}

func TestConfigRejectsBadValues(t *testing.T) {
	t.Setenv(EnvDir, "")
	cases := []struct {
		name string
		body string
		want string
	}{
		{"interval too small", `{"interval": "10ms"}`, "too small"},
		{"timeout above interval", `{"interval": "1s", "timeout": "2s"}`, "must not exceed"},
		{"url without scheme", `{"snapshot_url": "127.0.0.1:9088/runtime"}`, "invalid snapshot_url"},
		{"bad duration", `{"interval": "soon"}`, "invalid duration"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "monitor.json")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			_, err := LoadConfig(path)
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestDurationAcceptsNumberAndString(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.json")
	if err := os.WriteFile(path, []byte(`{"interval": 3}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Interval.D() != 3*time.Second {
		t.Errorf("interval = %s, want 3s", cfg.Interval.D())
	}
}
