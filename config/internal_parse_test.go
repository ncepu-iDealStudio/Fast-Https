package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePathType(t *testing.T) {
	cases := []struct {
		name      string
		typeStr   string
		proxyPass string
		want      uint16
	}{
		{"local", "local", "", LOCAL},
		{"rewrite", "rewrite", "", REWRITE},
		{"devmod", "devmod", "", DEVMOD},
		{"proxy http", "proxy", "http://127.0.0.1:9000", PROXY_HTTP},
		{"proxy https", "proxy", "https://127.0.0.1:9443", PROXY_HTTPS},
		{"proxy no scheme falls back to local", "proxy", "127.0.0.1:9000", LOCAL},
		{"proxy unknown scheme falls back", "proxy", "ftp://x", LOCAL},
		{"unknown type", "magic", "", LOCAL},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parsePathType(c.typeStr, c.proxyPass); got != c.want {
				t.Fatalf("parsePathType(%q,%q) = %d, want %d", c.typeStr, c.proxyPass, got, c.want)
			}
		})
	}
}

func TestParseZip(t *testing.T) {
	cases := []struct {
		name string
		zip  []string
		want uint16
	}{
		{"empty", nil, ZIP_NONE},
		{"br", []string{"br"}, ZIP_BR},
		{"gzip", []string{"gzip"}, ZIP_GZIP},
		{"gzip then br", []string{"gzip", "br"}, ZIP_GZIP_BR},
		{"br then gzip", []string{"br", "gzip"}, ZIP_GZIP_BR},
		{"unknown single", []string{"zstd"}, ZIP_NONE},
		{"mismatched pair", []string{"gzip", "zstd"}, ZIP_NONE},
		{"too many", []string{"gzip", "br", "zstd"}, ZIP_NONE},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseZip(c.zip); got != c.want {
				t.Fatalf("parseZip(%v) = %d, want %d", c.zip, got, c.want)
			}
		})
	}
}

func TestTrimProxyPass(t *testing.T) {
	cases := map[string]string{
		"http://1.2.3.4:80":  "1.2.3.4:80",
		"https://a.b":        "a.b",
		"plain-host":         "plain-host",
	}
	for in, want := range cases {
		if got := trimProxyPass(in); got != want {
			t.Errorf("trimProxyPass(%q) = %q, want %q", in, got, want)
		}
	}
}

// include 目录遍历分支：合法 server 块、非 json 跳过、空目录、坏 json 报错
func TestProcessIncludeCfg(t *testing.T) {
	dir := t.TempDir()

	write := func(rel, content string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	const serverA = `{"listen":"18081","server_name":"a.local","location":[{"url":"/","type":"local","root":"./a"}]}`
	const serverB = `{"listen":"18082","server_name":"b.local","location":[{"url":"/x","type":"rewrite","rewrite":"/y"}]}`
	write("a.json", serverA)
	write("nested/b.json", serverB)           // 嵌套目录也要展开
	write("notes.txt", "not a config")        // 非 json 跳过
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &Fast_Https{Include: []string{dir}}
	if err := processIncludeCfg(cfg); err != nil {
		t.Fatalf("processIncludeCfg: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("servers = %d, want 2 (json files only)", len(cfg.Servers))
	}
	// 按文件名遍历顺序不可假设，按键找
	byName := map[string]Server{}
	for _, s := range cfg.Servers {
		byName[s.ServerName] = s
	}
	if s := byName["a.local"]; s.Listen != "18081" || len(s.Path) != 1 || s.Path[0].PathType != LOCAL {
		t.Fatalf("server a parsed wrong: %+v", s)
	}
	if s := byName["b.local"]; s.Path[0].Rewrite != "/y" {
		t.Fatalf("server b parsed wrong: %+v", s)
	}

	// 坏 json：报错而不是 panic
	write("broken.json", "{not json")
	if err := processIncludeCfg(&Fast_Https{Include: []string{dir}}); err == nil {
		t.Fatal("broken json must fail the include walk")
	}

	// 不存在的 include 路径：Walk 直接报错
	if err := processIncludeCfg(&Fast_Https{Include: []string{filepath.Join(dir, "ghost")}}); err == nil {
		t.Fatal("missing include dir must fail")
	}

	// 无 include：no-op
	if err := processIncludeCfg(&Fast_Https{}); err != nil {
		t.Fatalf("empty include must be a no-op: %v", err)
	}
}

// Reload 指向不存在的文件：报错且当前快照保持不动
func TestReloadMissingFileKeepsSnapshot(t *testing.T) {
	before := Current()
	if err := ReloadFile(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("missing file must fail reload")
	}
	if Current() != before {
		t.Fatal("failed reload must leave the snapshot untouched")
	}
}
