package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateConfigFileAcceptsComments(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "fast-https.json")
	body := `{
  // 主配置
  "http": {
    "server": [
      {
        "listen": 80,
        "server_name": "localhost",
        "location": [
          {"url": "/", "type": "local", "root": "./httpdoc/root"}, // 静态根
        ],
      },
    ],
    "include": []
  }
}`
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateConfigFile(main); err != nil {
		t.Fatalf("commented config should validate: %v", err)
	}
}

func TestValidateIncludeAcceptsComments(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "fast-https.json")
	include := filepath.Join(dir, "conf.d", "site.json")

	if err := os.MkdirAll(filepath.Dir(include), 0o755); err != nil {
		t.Fatal(err)
	}
	// include 用绝对路径：相对路径的解析基准是安装目录而不是配置文件目录，
	// 那是既有行为，不属于本用例要覆盖的范围。
	if err := os.WriteFile(main, []byte(`{
  "http": {
    "server": [{"listen": 80, "server_name": "a.local", "location": [{"url": "/", "type": "local", "root": "./httpdoc/root"}]}],
    "include": ["`+filepath.ToSlash(filepath.Dir(include))+`"]
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(include, []byte(`{
  // 拆分出来的站点
  "listen": "8081",
  "server_name": "api.local",
  "location": [{"url": "/api", "type": "proxy", "proxy_pass": "http://127.0.0.1:9000"}],
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateConfigFile(main); err != nil {
		t.Fatalf("commented include should validate: %v", err)
	}
}
