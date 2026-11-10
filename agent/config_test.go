package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigMissingUsesDefaults(t *testing.T) {
	t.Setenv("FASTHTTPS_MONITOR_DIR", "")
	t.Setenv("FASTHTTPS_AGENT_DIR", "")
	t.Setenv("FASTHTTPS_AGENT_MODEL_URL", "")
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MonitorDir != "logs/monitor" || cfg.StateDir != "state/agent" || cfg.CertWarnDays != 30 {
		t.Fatalf("defaults = %+v", cfg)
	}
	if cfg.ModelURL != "" {
		t.Fatalf("model url = %q", cfg.ModelURL)
	}
}

func TestConfigFileOverridesEnv(t *testing.T) {
	t.Setenv("FASTHTTPS_MONITOR_DIR", "from-env")
	t.Setenv("FASTHTTPS_AGENT_MODEL_URL", "http://127.0.0.1:9/v1")
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")
	body := `{
		"monitor_dir": "from-file",
		"cert_warn_days": 14,
		"runtime_url": "http://127.0.0.1:9088/runtime",
		"http": {"server": []}
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MonitorDir != "from-file" || cfg.CertWarnDays != 14 {
		t.Fatalf("file did not override: %+v", cfg)
	}
	if cfg.ModelURL != "http://127.0.0.1:9/v1" {
		t.Fatalf("empty file field cleared env model url: %q", cfg.ModelURL)
	}
}
