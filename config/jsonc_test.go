package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripJSONCommentsKeepsStrings(t *testing.T) {
	src := []byte(`{
  // 行注释
  "proxy_pass": "http://127.0.0.1:9000/api", /* 块注释 */
  "root": "./httpdoc/*/root",
  "escaped": "a\"//b"
}`)

	var got map[string]string
	if err := unmarshalConfig(src, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]string{
		"proxy_pass": "http://127.0.0.1:9000/api",
		"root":       "./httpdoc/*/root",
		"escaped":    `a"//b`,
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestStripJSONCommentsTrailingComma(t *testing.T) {
	src := []byte(`{
  "list": [1, 2, 3,],
  "obj": {"a": "b",},
}`)
	var got map[string]any
	if err := unmarshalConfig(src, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("keys = %v", got)
	}
}

// TestStripJSONCommentsKeepsLength 保证预处理不改变字节数：
// json.SyntaxError.Offset 才能直接对回原文件位置。
func TestStripJSONCommentsKeepsLength(t *testing.T) {
	src := []byte("{\n// xxx\n\"a\": 1,\n}\n")
	out := stripJSONComments(src)
	if len(out) != len(src) {
		t.Fatalf("len = %d, want %d", len(out), len(src))
	}
	if strings.Count(string(out), "\n") != strings.Count(string(src), "\n") {
		t.Fatal("newlines must be preserved")
	}
}

// TestStripJSONCommentsReportsOriginalOffset 保证语法错误的位置指向原文件：
// 注释被替换成空格而不是删除，字节偏移才能直接对回原文件。
func TestStripJSONCommentsReportsOriginalOffset(t *testing.T) {
	src := []byte("{\n  // c\n  \"a\" 1\n}\n")
	var v map[string]any
	err := unmarshalConfig(src, &v)
	if err == nil {
		t.Fatal("invalid json should fail")
	}

	var syn *json.SyntaxError
	if !errors.As(err, &syn) {
		t.Fatalf("want *json.SyntaxError, got %T: %v", err, err)
	}
	if syn.Offset <= 0 || int(syn.Offset) > len(src) {
		t.Fatalf("offset %d out of range %d", syn.Offset, len(src))
	}
	if got := src[syn.Offset-1]; got != '1' {
		t.Fatalf("offset %d points at %q, want the offending '1' from the original file",
			syn.Offset, got)
	}
}

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
