package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProxyPassArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fast-https.json")
	body := `{
	  "http": {
	    "server": [{
	      "listen": "80",
	      "server_name": "a.example",
	      "location": [{
	        "url": "/",
	        "type": "proxy",
	        "proxy_pass": ["http://10.0.0.1:8080", "http://10.0.0.2:8080"]
	      }]
	    }]
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := processRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 1 || len(cfg.Servers[0].Path) != 1 {
		t.Fatalf("servers = %+v", cfg.Servers)
	}
	path0 := cfg.Servers[0].Path[0]
	if path0.PathType != PROXY_HTTP {
		t.Fatalf("type = %d", path0.PathType)
	}
	if path0.ProxyData != "10.0.0.1:8080" {
		t.Fatalf("first = %q", path0.ProxyData)
	}
	if len(path0.ProxyPeers) != 2 || path0.ProxyPeers[1] != "10.0.0.2:8080" {
		t.Fatalf("peers = %#v", path0.ProxyPeers)
	}
}
