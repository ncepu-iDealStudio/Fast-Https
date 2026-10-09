package config_test

import (
	"fast-https/config"
	"os"
	"path/filepath"
	"testing"
)

func TestReloadFileKeepsPreviousConfigWhenInvalid(t *testing.T) {
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
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	previousConfig := config.GConfig
	previousMIME := config.GContentTypeMap
	t.Cleanup(func() {
		config.GConfig = previousConfig
		config.GContentTypeMap = previousMIME
	})

	dir := t.TempDir()
	path := filepath.Join(dir, "fast-https.json")
	good := `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "keep.local",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(path, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadFile(path); err != nil {
		t.Fatalf("load valid config: %v", err)
	}
	if got := config.GConfig.Servers[0].ServerName; got != "keep.local" {
		t.Fatalf("server name = %q", got)
	}

	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.ReloadFile(path); err == nil {
		t.Fatal("invalid config should be rejected")
	}
	if got := config.GConfig.Servers[0].ServerName; got != "keep.local" {
		t.Fatalf("invalid reload changed server name to %q", got)
	}

	updated := `{
  "http": {
    "server": [
      {
        "listen": 8081,
        "server_name": "next.local",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.ReloadFile(path); err != nil {
		t.Fatalf("valid reload: %v", err)
	}
	if got := config.GConfig.Servers[0].ServerName; got != "next.local" {
		t.Fatalf("server name = %q", got)
	}
}

func TestReloadFileRejectsMissingCertificate(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := wd
	for {
		if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("cannot find repository root")
		}
		root = next
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
	good := `{
  "http": {
    "server": [
      {
        "listen": 8080,
        "server_name": "keep.local",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(path, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadFile(path); err != nil {
		t.Fatal(err)
	}

	missing := `{
  "http": {
    "server": [
      {
        "listen": "8443 ssl",
        "server_name": "broken.local",
        "ssl_certificate": "missing.pem",
        "ssl_certificate_key": "missing-key.pem",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root", "index": ["index.html"]}
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(path, []byte(missing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.ReloadFile(path); err == nil {
		t.Fatal("missing certificate should be rejected")
	}
	if got := config.GConfig.Servers[0].ServerName; got != "keep.local" {
		t.Fatalf("server name = %q", got)
	}
}
