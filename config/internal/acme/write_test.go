package acme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config/internal/model"
	"fast-https/config/internal/schema"
	"fast-https/config/internal/testutil"
)

// acmeWriteFixture 是带注释的示例主配置，期望 patch 后注释与 http 段都保住。
func acmeWriteFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testutil.DataDir(), "acme", "write_fixture.json"))
	if err != nil {
		t.Fatalf("read acme write fixture: %v", err)
	}
	return raw
}

func testAcmeConfig() *model.AcmeConfig {
	return &model.AcmeConfig{
		Enabled:         true,
		Email:           "ops@example.com",
		Staging:         false,
		Challenge:       []string{model.ACMEChallengeAuto},
		RenewBeforeDays: 30,
		Storage:         model.ACMECertStorageDir,
		CAs:             DefaultACMECAs(),
	}
}

// 没有 acme 段时插入，并且把默认配置里那段被注释掉的 acme 示例一起替换掉，
// 否则用户会以为还要手工取消注释。
func TestPatchACMESectionInsertsAndDropsCommentedSample(t *testing.T) {
	patched, err := patchACMESection(acmeWriteFixture(t), testAcmeConfig())
	if err != nil {
		t.Fatalf("patch: %v", err)
	}

	var probe struct {
		HTTP *struct {
			Include []string `json:"include"`
		} `json:"http"`
		Acme *model.AcmeConfig `json:"acme"`
	}
	if err := schema.Unmarshal(patched, &probe); err != nil {
		t.Fatalf("patched config does not parse: %v\n%s", err, patched)
	}
	if probe.Acme == nil || !probe.Acme.Enabled || probe.Acme.Email != "ops@example.com" {
		t.Fatalf("acme section = %+v", probe.Acme)
	}
	if probe.HTTP == nil || len(probe.HTTP.Include) != 1 {
		t.Fatalf("http section was damaged: %+v", probe.HTTP)
	}

	text := string(patched)
	if strings.Contains(text, `// ,"acme"`) {
		t.Fatalf("commented acme sample should be replaced:\n%s", text)
	}
	if strings.Count(text, `"acme":`) != 1 {
		t.Fatalf("acme key should appear exactly once as a real key:\n%s", text)
	}
	if !strings.Contains(text, "// 主配置示例") {
		t.Fatalf("unrelated comments must survive:\n%s", text)
	}
}

// 已有 acme 段时只替换值，其它键与注释保持原样，且不产生第二份 acme。
func TestPatchACMESectionReplacesExistingValue(t *testing.T) {
	raw := `{
  "acme": {
    "enabled": false,
    "email": ""
  },
  // http 段注释
  "http": { "server": [] }
}
`
	cfg := testAcmeConfig()
	patched, err := patchACMESection([]byte(raw), cfg)
	if err != nil {
		t.Fatalf("patch: %v", err)
	}

	var probe struct {
		Acme *model.AcmeConfig `json:"acme"`
		HTTP map[string]any    `json:"http"`
		Raw  map[string]bool   `json:"-"`
	}
	if err := schema.Unmarshal(patched, &probe); err != nil {
		t.Fatalf("patched config does not parse: %v\n%s", err, patched)
	}
	if probe.Acme == nil || !probe.Acme.Enabled || probe.Acme.Email != "ops@example.com" {
		t.Fatalf("acme section = %+v", probe.Acme)
	}
	if probe.HTTP == nil {
		t.Fatal("http section disappeared")
	}
	if strings.Count(string(patched), `"acme"`) != 1 {
		t.Fatalf("expected a single acme key:\n%s", patched)
	}
	if !strings.Contains(string(patched), "// http 段注释") {
		t.Fatalf("comments must survive replacement:\n%s", patched)
	}
}

// 空对象与尾随逗号两种边界。
func TestPatchACMESectionEdgeShapes(t *testing.T) {
	cases := map[string]string{
		"empty object": "{}",
		"trailing comma": `{
  "http": { "server": [] },
}`,
	}
	for name, raw := range cases {
		patched, err := patchACMESection([]byte(raw), testAcmeConfig())
		if err != nil {
			t.Fatalf("%s: patch: %v", name, err)
		}
		var probe struct {
			Acme *model.AcmeConfig `json:"acme"`
		}
		if err := schema.Unmarshal(patched, &probe); err != nil {
			t.Fatalf("%s: patched config does not parse: %v\n%s", name, err, patched)
		}
		if probe.Acme == nil || probe.Acme.Email != "ops@example.com" {
			t.Fatalf("%s: acme section = %+v", name, probe.Acme)
		}
	}
}

// 非对象根节点必须报错，而不是改出半截文件。
func TestPatchACMESectionRejectsNonObject(t *testing.T) {
	if _, err := patchACMESection([]byte(`[]`), testAcmeConfig()); err == nil {
		t.Fatal("array root should be rejected")
	}
	if _, err := patchACMESection([]byte(`{`), testAcmeConfig()); err == nil {
		t.Fatal("unterminated object should be rejected")
	}
}

// 写文件：真实默认配置能就地写入、回读解析、其余内容不变。
func TestWriteACMESectionOnDefaultConfig(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "fast-https.json"))
	if err != nil {
		t.Fatalf("read default config: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fast-https.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := testAcmeConfig()
	cfg.Storage = "config/cert/acme"
	if err := WriteACMESection(path, cfg); err != nil {
		t.Fatalf("write: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file should be renamed away")
	}
	var probe struct {
		Acme *model.AcmeConfig `json:"acme"`
	}
	if err := schema.Unmarshal(written, &probe); err != nil {
		t.Fatalf("written config does not parse: %v", err)
	}
	if probe.Acme == nil || !probe.Acme.Enabled || probe.Acme.Storage != "config/cert/acme" {
		t.Fatalf("acme section = %+v", probe.Acme)
	}
	if !strings.Contains(string(written), `"http"`) {
		t.Fatal("http section disappeared")
	}
	// 再写一次应当幂等：不新增第二个 acme 键。
	if err := WriteACMESection(path, cfg); err != nil {
		t.Fatalf("second write: %v", err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(again), `"acme":`) != 1 {
		t.Fatalf("second write duplicated the acme key:\n%s", again)
	}
}
