package jsonc

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestUnmarshalKeepsStrings(t *testing.T) {
	src := []byte(`{
  // 行注释
  "proxy_pass": "http://127.0.0.1:9000/api", /* 块注释 */
  "root": "./httpdoc/*/root",
  "escaped": "a\"//b"
}`)

	var got map[string]string
	if err := Unmarshal(src, &got); err != nil {
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

func TestUnmarshalTrailingComma(t *testing.T) {
	src := []byte(`{
  "list": [1, 2, 3,],
  "obj": {"a": "b",},
}`)
	var got map[string]any
	if err := Unmarshal(src, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("keys = %v", got)
	}
}

// 预处理不改变字节数：json.SyntaxError.Offset 才能直接对回原文件位置。
func TestStripCommentsKeepsLength(t *testing.T) {
	src := []byte("{\n// xxx\n\"a\": 1,\n}\n")
	out := StripComments(src)
	if len(out) != len(src) {
		t.Fatalf("len = %d, want %d", len(out), len(src))
	}
	if strings.Count(string(out), "\n") != strings.Count(string(src), "\n") {
		t.Fatal("newlines must be preserved")
	}
}

// 语法错误的位置指向原文件：注释被替换成空格而不是删除，偏移才能对得上。
func TestStripCommentsReportsOriginalOffset(t *testing.T) {
	src := []byte("{\n  // c\n  \"a\" 1\n}\n")
	var v map[string]any
	err := Unmarshal(src, &v)
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

// 纯 JSON 与 JSONC 的解析结果必须完全一致。
func TestUnmarshalPlainJSONUnchanged(t *testing.T) {
	src := []byte(`{"a": 1, "b": ["x", "y"]}`)
	var got, want map[string]any
	if err := Unmarshal(src, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := json.Unmarshal(src, &want); err != nil {
		t.Fatalf("stdlib unmarshal: %v", err)
	}
	if len(got) != len(want) || got["a"] != want["a"] {
		t.Fatalf("got %v, want %v", got, want)
	}
}
