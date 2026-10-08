package config_test

import (
	"fast-https/config"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileResolvesRelativePaths(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := wd
	if _, statErr := os.Stat(filepath.Join(wd, "go.mod")); statErr != nil {
		root = filepath.Dir(wd)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	previousConfig := config.GConfig
	previousMIME := config.GContentTypeMap
	t.Cleanup(func() {
		config.GConfig = previousConfig
		config.GContentTypeMap = previousMIME
	})

	dir := t.TempDir()
	path := filepath.Join(dir, "fast-https.json")
	body := `{
  "http": {
    "server": [
      {
        "listen": "443 ssl",
        "server_name": "localhost",
        "ssl_certificate": "config/cert/localhost.pem",
        "ssl_certificate_key": "config/cert/localhost-key.pem",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ],
    "include": ["./config/conf.d"]
  }
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadFile(path); err != nil {
		t.Fatalf("load config: %v", err)
	}
	server := config.GConfig.Servers[0]
	if !filepath.IsAbs(server.Path[0].Root) {
		t.Fatalf("site root = %q", server.Path[0].Root)
	}
	if !filepath.IsAbs(server.SSLCertificate) || !filepath.IsAbs(server.SSLCertificateKey) {
		t.Fatalf("cert = %q key = %q", server.SSLCertificate, server.SSLCertificateKey)
	}
	if !filepath.IsAbs(config.GConfig.LogRoot) {
		t.Fatalf("log root = %q", config.GConfig.LogRoot)
	}
	if len(config.GConfig.Include) == 0 || !filepath.IsAbs(config.GConfig.Include[0]) {
		t.Fatalf("include = %#v", config.GConfig.Include)
	}
}
