package jsonc

import (
	"strings"
	"testing"
)

func fixed(value string) func(string) ([]byte, error) {
	return func(string) ([]byte, error) { return []byte(value), nil }
}

// 已有该键：只换值，键前后的注释、字段顺序、缩进一个字节都不动。
func TestPatchMemberReplacesValue(t *testing.T) {
	raw := []byte(`{
  // 主配置
  "http": { "server": [] },
  "acme": {"enabled": false},
  "log_root": "./logs" // 行尾注释
}`)

	out, err := PatchMember(raw, "acme", fixed(`{"enabled": true}`), PatchOptions{})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, `"acme": {"enabled": true},`) {
		t.Fatalf("value not replaced:\n%s", got)
	}
	if !strings.Contains(got, "// 主配置") || !strings.Contains(got, "// 行尾注释") {
		t.Fatalf("comments must survive:\n%s", got)
	}
	if !strings.Contains(got, `"http": { "server": [] },`) {
		t.Fatalf("neighbouring members must be untouched:\n%s", got)
	}
}

func TestPatchMemberAppendsAfterLastMember(t *testing.T) {
	raw := []byte(`{
  "http": {}
}`)

	out, err := PatchMember(raw, "acme", fixed(`{"enabled": true}`), PatchOptions{})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, `"http": {},`) {
		t.Fatalf("existing member must gain a comma:\n%s", got)
	}
	if !strings.Contains(got, "\n  \"acme\": {\"enabled\": true}\n") {
		t.Fatalf("appended member formatting:\n%s", got)
	}
}

// 原文件写了尾随逗号：不能再补一个，否则出现两个逗号。
func TestPatchMemberAppendsWithTrailingComma(t *testing.T) {
	raw := []byte(`{
  "http": {},
}`)

	out, err := PatchMember(raw, "acme", fixed(`1`), PatchOptions{})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if strings.Contains(string(out), ",,") {
		t.Fatalf("double comma:\n%s", out)
	}
}

func TestPatchMemberEmptyObject(t *testing.T) {
	out, err := PatchMember([]byte(`{}`), "acme", fixed(`1`), PatchOptions{})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if strings.Contains(string(out), "{},") {
		t.Fatalf("empty object must not gain a comma:\n%s", out)
	}
}

// 追加时按文件自身的缩进风格生成值，调用方据此排版多行内容。
func TestPatchMemberReportsIndent(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{raw: "{\n  \"a\": 1\n}", want: "  "},
		{raw: "{\n\t\"a\": 1\n}", want: "\t"},
		{raw: "{\n    \"a\": 1\n}", want: "    "},
	}
	for _, c := range cases {
		var got string
		if _, err := PatchMember([]byte(c.raw), "b", func(indent string) ([]byte, error) {
			got = indent
			return []byte("1"), nil
		}, PatchOptions{}); err != nil {
			t.Fatalf("%q: %v", c.raw, err)
		}
		if got != c.want {
			t.Fatalf("%q: indent = %q, want %q", c.raw, got, c.want)
		}
	}
}

// 注释里提到同名键时丢掉末尾空隙，避免"这段示例还要手工取消注释"的歧义。
func TestPatchMemberDropsGapWhenAsked(t *testing.T) {
	raw := []byte(`{
  "http": {}
  // "acme": { ... }
}`)

	out, err := PatchMember(raw, "acme", fixed(`{"enabled": true}`), PatchOptions{DropGapContaining: `"acme"`})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if strings.Contains(string(out), `"acme": { ... }`) {
		t.Fatalf("commented sample should be dropped:\n%s", out)
	}

	kept, err := PatchMember(raw, "acme", fixed(`{"enabled": true}`), PatchOptions{})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if !strings.Contains(string(kept), `"acme": { ... }`) {
		t.Fatalf("gap must be kept without the option:\n%s", kept)
	}
}

// 嵌套结构与字符串里的花括号/引号不能被当成成员边界。
func TestPatchMemberHandlesNestedAndStrings(t *testing.T) {
	raw := []byte(`{
  "location": [
    { "url": "/", "root": "./httpdoc/root" }
  ],
  "banner": "say \"} acme { end\"",
  "acme": {"enabled": false}
}`)

	out, err := PatchMember(raw, "acme", fixed(`{"enabled": true}`), PatchOptions{})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, `"acme": {"enabled": true}`) {
		t.Fatalf("acme not replaced:\n%s", got)
	}
	if !strings.Contains(got, `"banner": "say \"} acme { end\"",`) {
		t.Fatalf("string value must be untouched:\n%s", got)
	}
	if !strings.Contains(got, `{ "url": "/", "root": "./httpdoc/root" }`) {
		t.Fatalf("array value must be untouched:\n%s", got)
	}
}

// 只有值里出现 "acme" 不算已有该键。
func TestPatchMemberIgnoresKeyInStringValue(t *testing.T) {
	raw := []byte(`{
  "note": "acme"
}`)
	out, err := PatchMember(raw, "acme", fixed(`1`), PatchOptions{})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if strings.Count(string(out), `"acme"`) != 2 { // 值里的 + 新增的键
		t.Fatalf("unexpected patch:\n%s", out)
	}
}

func TestPatchMemberRejectsNonObject(t *testing.T) {
	if _, err := PatchMember([]byte(`[]`), "acme", fixed(`1`), PatchOptions{}); err == nil {
		t.Fatal("array root should be rejected")
	}
	if _, err := PatchMember([]byte(`{`), "acme", fixed(`1`), PatchOptions{}); err == nil {
		t.Fatal("unterminated object should be rejected")
	}
	if _, err := PatchMember([]byte(``), "acme", fixed(`1`), PatchOptions{}); err == nil {
		t.Fatal("empty file should be rejected")
	}
}

func TestScanObjectMembers(t *testing.T) {
	raw := []byte("{\n  // c\n  \"a\": [1, 2],\n  \"b\": \"x\"\n}")
	obj, err := ScanObject(StripComments(raw))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(obj.Members) != 2 {
		t.Fatalf("members = %+v", obj.Members)
	}
	if obj.Members[0].Key != "a" || obj.Members[1].Key != "b" {
		t.Fatalf("keys = %q/%q", obj.Members[0].Key, obj.Members[1].Key)
	}
	if got := string(raw[obj.Members[0].ValueStart:obj.Members[0].ValueEnd]); got != "[1, 2]" {
		t.Fatalf("value span = %q", got)
	}
}
