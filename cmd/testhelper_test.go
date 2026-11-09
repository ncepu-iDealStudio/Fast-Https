package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"text/template"
)

// renderTestdata 渲染 cmd/testdata 下的模板。固定输入放 testdata（编译器
// 不会把该目录当成包内容），需要按测试临时目录改路径的用 .tmpl 模板。
func renderTestdata(t *testing.T, name string, data any) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	tpl, err := template.New(name).Parse(string(raw))
	if err != nil {
		t.Fatalf("parse testdata/%s: %v", name, err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("render testdata/%s: %v", name, err)
	}
	return buf.String()
}

// siteTestEnv 建一个临时的主配置与 conf.d 目录，供 CLI 契约测试使用。
// site 命令组迁到 internal/sitecmd 后，cmd 侧的契约测试仍需要这个夹具。
func siteTestEnv(t *testing.T) (mainPath, includeDir string) {
	t.Helper()
	base := t.TempDir()
	includeDir = filepath.Join(base, "conf.d")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mainPath = filepath.Join(base, "fast-https.json")
	main := renderTestdata(t, "main-standalone.json.tmpl", map[string]any{"IncludeDir": includeDir})
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	return mainPath, includeDir
}
