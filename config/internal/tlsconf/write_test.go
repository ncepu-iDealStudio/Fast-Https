package tlsconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config/internal/model"
)

const baseConfig = `{
  // 主配置
  "http": {"include": []},
  "waf": {"enabled": false}
}
`

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestWriteTLSSectionAppendsAndKeepsComments(t *testing.T) {
	path := writeTemp(t, baseConfig)
	want := &model.TLSConfig{DefaultCertificate: model.Certificate{Certificate: "def.pem", Key: "def.key"}}
	if err := WriteTLSSection(path, want); err != nil {
		t.Fatal(err)
	}
	out := readFile(t, path)
	for _, s := range []string{"// 主配置", `"default_certificate"`, `"def.pem"`, `"def.key"`, `"waf"`} {
		if !strings.Contains(out, s) {
			t.Fatalf("写入后缺少 %s:\n%s", s, out)
		}
	}
	got, err := ReadTLSDefaults(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultCertificate != want.DefaultCertificate {
		t.Fatalf("回读 = %+v", got.DefaultCertificate)
	}
}

func TestWriteTLSSectionReplacesExisting(t *testing.T) {
	path := writeTemp(t, `{"http": {"include": []}, "tls": {"default_certificate": {"certificate": "old.pem", "key": "old.key"}}}`)
	want := &model.TLSConfig{DefaultCertificate: model.Certificate{Certificate: "new.pem", Key: "new.key"}}
	if err := WriteTLSSection(path, want); err != nil {
		t.Fatal(err)
	}
	out := readFile(t, path)
	if strings.Contains(out, "old.pem") {
		t.Fatalf("旧值没被替换:\n%s", out)
	}
	if strings.Count(out, `"tls"`) != 1 {
		t.Fatalf("tls 段应只有一个:\n%s", out)
	}
	if !strings.Contains(out, `"new.pem"`) {
		t.Fatalf("新值没写进去:\n%s", out)
	}
}

func TestWriteTLSSectionClears(t *testing.T) {
	path := writeTemp(t, `{
  "http": {"include": []},
  "tls": {"default_certificate": {"certificate": "def.pem", "key": "def.key"}},
  "waf": {"enabled": false}
}`)
	if err := WriteTLSSection(path, &model.TLSConfig{}); err != nil {
		t.Fatal(err)
	}
	out := readFile(t, path)
	if strings.Contains(out, "tls") || strings.Contains(out, "default_certificate") {
		t.Fatalf("tls 段应被删除:\n%s", out)
	}
	if !strings.Contains(out, `"waf"`) {
		t.Fatalf("其它字段必须保留:\n%s", out)
	}
	if _, err := ReadTLSDefaults(path); err != nil {
		t.Fatalf("删除后文件应仍可解析：%v\n%s", err, out)
	}
}

func TestWriteTLSSectionClearWhenAbsent(t *testing.T) {
	path := writeTemp(t, baseConfig)
	if err := WriteTLSSection(path, nil); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != baseConfig {
		t.Fatalf("没有 tls 段时不应改动文件:\n%s", got)
	}
}

func TestPreviewDoesNotWrite(t *testing.T) {
	path := writeTemp(t, baseConfig)
	want := &model.TLSConfig{DefaultCertificate: model.Certificate{Certificate: "def.pem", Key: "def.key"}}
	patched, err := PreviewTLSSection(path, want)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patched), "def.pem") {
		t.Fatalf("预览内容不对:\n%s", patched)
	}
	if got := readFile(t, path); got != baseConfig {
		t.Fatal("Preview 不应写盘")
	}
}
