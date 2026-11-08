package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnoseConfigInvalid(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(main, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCLI(t, "diagnose", "--config", main, "--json")
	if code != 0 {
		t.Fatalf("诊断查询应成功，exit = %d\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	if doc["ok"] != true || doc["command"] != "diagnose" {
		t.Fatalf("封套不对：%v", doc)
	}
	data := doc["data"].(map[string]any)
	if data["overall"] != "error" {
		t.Fatalf("overall = %v", data["overall"])
	}
	if checkStatus(t, data, "config") != "error" {
		t.Fatalf("config 检查应为 error：%v", data["checks"])
	}
}

func TestDiagnoseUnavailableVsFailed(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	out, _, code := runCLI(t, "diagnose", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if checkStatus(t, data, "runtime") != "unavailable" || checkStatus(t, data, "monitor") != "unavailable" {
		t.Fatalf("runtime/monitor 现在应是 unavailable：%v", data["checks"])
	}
	if data["overall"] == "error" && checkStatus(t, data, "config") == "ok" {
		t.Fatalf("unavailable 不应把 overall 拉成 error：%v", data)
	}
}

func TestDiagnoseReadOnly(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	before := hashTree(t, f.dir)
	if _, _, code := runCLI(t, "diagnose", "--config", f.main, "--json"); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	after := hashTree(t, f.dir)
	if len(before) != len(after) {
		t.Fatalf("文件数变了：%d -> %d", len(before), len(after))
	}
	for path, sum := range before {
		if after[path] != sum {
			t.Fatalf("只读命令改了 %s", path)
		}
	}
}

func TestDiagnoseNoSecrets(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	out, errOut, code := runCLI(t, "diagnose", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	textOut, textErr, textCode := runCLI(t, "diagnose", "--config", f.main)
	if textCode != 0 {
		t.Fatalf("文本模式 exit = %d\n%s", textCode, textOut)
	}
	blob := out + errOut + textOut + textErr
	for _, secret := range fixtureSecrets {
		if strings.Contains(blob, secret) {
			t.Errorf("输出含有敏感值 %q", secret)
		}
	}
}

func checkStatus(t *testing.T, data map[string]any, name string) string {
	t.Helper()
	raw, _ := json.Marshal(data["checks"])
	var checks []map[string]any
	if err := json.Unmarshal(raw, &checks); err != nil {
		t.Fatal(err)
	}
	for _, c := range checks {
		if c["name"] == name {
			s, _ := c["status"].(string)
			return s
		}
	}
	t.Fatalf("没有名为 %s 的检查：%s", name, raw)
	return ""
}
