package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"fast-https/modules/proxy"
)

func TestUpstreamDrainKeepsLastAddress(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "fast-https.json")
	body := `{
	  "http": {"server": [{
	    "listen": "80",
	    "server_name": "a.example",
	    "location": [{"url": "/", "type": "proxy", "proxy_pass": "http://10.0.0.8:8080"}]
	  }]}
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	drainFile := filepath.Join(dir, "drained.json")
	t.Setenv("FASTHTTPS_UPSTREAM_DRAIN_FILE", drainFile)
	t.Setenv("FASTHTTPS_STATE_DIR", dir)
	out, _, code := runCLI(t, "upstream", "drain", "10.0.0.8:8080", "--config", cfgPath, "--yes", "--json")
	if code != 6 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("config file changed")
	}
	if _, err := os.Stat(drainFile); !os.IsNotExist(err) {
		t.Fatalf("drain file was written: %v", err)
	}
	_ = proxy.ErrLastUpstream
}
