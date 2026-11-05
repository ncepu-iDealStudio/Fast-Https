package jsonc

import (
	"encoding/json"
	"strings"
	"testing"
)

// decode 用去注释后的文本做结构断言；带尾随逗号的用例不走这里。
func decode(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(StripComments(raw), &out); err != nil {
		t.Fatalf("结果不是合法 JSON：%v\n---\n%s", err, raw)
	}
	return out
}

func mustPatch(t *testing.T, raw string, edits ...Edit) string {
	t.Helper()
	out, err := PatchPath([]byte(raw), edits)
	if err != nil {
		t.Fatalf("PatchPath: %v\n---\n%s", err, raw)
	}
	return string(out)
}

func TestPatchReplacesValueAndKeepsEverythingElse(t *testing.T) {
	raw := `{
  // 站点说明
  "listen": 80,
  "server_name": "old.example",
  "location": [
    { "url": "/", "root": "/old" }
  ]
}`
	got := mustPatch(t, raw, Edit{Path: []Step{Key("server_name")}, Value: []byte(`"new.example"`)})

	if !strings.Contains(got, "// 站点说明") {
		t.Fatalf("注释被吃掉了：\n%s", got)
	}
	if !strings.Contains(got, `"listen": 80`) || !strings.Contains(got, `"root": "/old"`) {
		t.Fatalf("无关字段被改动：\n%s", got)
	}
	if decode(t, []byte(got))["server_name"] != "new.example" {
		t.Fatalf("server_name 没改成功：\n%s", got)
	}
}

func TestPatchAppendsMissingKey(t *testing.T) {
	raw := `{
  "listen": 80,
  "server_name": "a.example"
}`
	got := mustPatch(t, raw,
		Edit{Path: []Step{Key("ssl_certificate")}, Value: []byte(`"/etc/cert/a.pem"`)},
		Edit{Path: []Step{Key("ssl_certificate_key")}, Value: []byte(`"/etc/cert/a-key.pem"`)},
	)

	m := decode(t, []byte(got))
	if m["ssl_certificate"] != "/etc/cert/a.pem" || m["ssl_certificate_key"] != "/etc/cert/a-key.pem" {
		t.Fatalf("追加失败：\n%s", got)
	}
	if !strings.Contains(got, `"listen": 80,`) {
		t.Fatalf("原最后一个成员应该补上逗号：\n%s", got)
	}
}

func TestPatchNestedObjectAndArrayIndex(t *testing.T) {
	raw := `{
  "location": [
    {
      "url": "/",
      "type": "local",
      "root": "/old",
      "auth": { "type": "basic", "user": "u", "pswd": "p" }
    }
  ]
}`
	got := mustPatch(t, raw,
		Edit{Path: []Step{Key("location"), At(0), Key("root")}, Value: []byte(`"/new"`)},
	)

	if !strings.Contains(got, `"auth"`) {
		t.Fatalf("未建模的 auth 被删掉了：\n%s", got)
	}
	m := decode(t, []byte(got))
	loc := m["location"].([]any)[0].(map[string]any)
	if loc["root"] != "/new" {
		t.Fatalf("root 没改成功：\n%s", got)
	}
}

func TestPatchAppendsArrayElement(t *testing.T) {
	raw := `{
  "listen": 80,
  "location": [
    { "url": "/", "root": "/a" }
  ]
}`
	got := mustPatch(t, raw,
		Edit{Path: []Step{Key("location"), Append()}, Value: []byte("{\n  \"url\": \"/api\",\n  \"type\": \"proxy\"\n}")},
	)

	m := decode(t, []byte(got))
	elems := m["location"].([]any)
	if len(elems) != 2 {
		t.Fatalf("应该有 2 个 location：\n%s", got)
	}
	if elems[1].(map[string]any)["url"] != "/api" {
		t.Fatalf("追加的元素不对：\n%s", got)
	}
}

func TestPatchAppendsIntoEmptyArray(t *testing.T) {
	raw := `{
  "location": []
}`
	got := mustPatch(t, raw,
		Edit{Path: []Step{Key("location"), Append()}, Value: []byte(`{ "url": "/" }`)},
	)
	if len(decode(t, []byte(got))["location"].([]any)) != 1 {
		t.Fatalf("空数组追加失败：\n%s", got)
	}
}

func TestPatchDeletesMiddleAndLastMember(t *testing.T) {
	raw := `{
  "listen": 80,
  "server_name": "a.example",
  "ssl_certificate": "/c.pem",
  "location": []
}`

	middle := mustPatch(t, raw, Edit{Path: []Step{Key("server_name")}, Value: nil})
	m := decode(t, []byte(middle))
	if _, ok := m["server_name"]; ok {
		t.Fatalf("server_name 没删掉：\n%s", middle)
	}
	if m["ssl_certificate"] != "/c.pem" {
		t.Fatalf("删中间成员影响了后面的字段：\n%s", middle)
	}

	last := mustPatch(t, raw, Edit{Path: []Step{Key("location")}, Value: nil})
	m = decode(t, []byte(last))
	if _, ok := m["location"]; ok {
		t.Fatalf("location 没删掉：\n%s", last)
	}
	if !strings.Contains(last, `"ssl_certificate": "/c.pem"`) {
		t.Fatalf("删最后一个成员把前一个字段也带走了：\n%s", last)
	}
}

func TestPatchDeleteMissingKeyIsNoop(t *testing.T) {
	raw := "{\n  \"listen\": 80\n}"
	if got := mustPatch(t, raw, Edit{Path: []Step{Key("nope")}, Value: nil}); got != raw {
		t.Fatalf("删除不存在的键应该是空操作：\n%s", got)
	}
}

func TestPatchReportsPathInError(t *testing.T) {
	raw := `{"location": [{"url": "/"}]}`
	_, err := PatchPath([]byte(raw), []Edit{{Path: []Step{Key("location"), At(0), Key("root")}, Value: []byte(`"/x"`)}})
	if err != nil {
		t.Fatalf("键不存在时应追加，不该报错：%v", err)
	}

	_, err = PatchPath([]byte(raw), []Edit{{Path: []Step{Key("location"), At(3), Key("root")}, Value: []byte(`"/x"`)}})
	if err == nil {
		t.Fatal("下标越界应报错")
	}
	if !strings.Contains(err.Error(), "location[3]") {
		t.Fatalf("错误信息应带路径，实际：%v", err)
	}
}

// 站点文件允许尾随逗号，删除成员后不能留下 `,,` 这种结构。
func TestPatchKeepsTrailingCommaFilesValid(t *testing.T) {
	raw := `{
  "listen": 80,
  "server_name": "a.example",
  "location": [],
}`
	got := mustPatch(t, raw, Edit{Path: []Step{Key("server_name")}, Value: nil})
	if strings.Contains(got, ",,") {
		t.Fatalf("删出了连续逗号：\n%s", got)
	}
	if got != `{
  "listen": 80,
  "location": [],
}` {
		t.Fatalf("删中间成员的结果不对：\n%s", got)
	}
}
