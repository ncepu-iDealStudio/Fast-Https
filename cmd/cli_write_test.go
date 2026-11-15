package cmd

import (
	"fast-https/cmd/internal/cli"
	"fast-https/management/cfgver"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===== merged from cli_apply_test.go =====
func TestReloadLegacyOutputUnchanged(t *testing.T) {
	err := reload(writeTestConfig(t, invalidListenConfig), filepath.Join(t.TempDir(), "no.pid"))
	if err == nil || !strings.Contains(err.Error(), "已保留运行中的配置") {
		t.Fatalf("无效配置时应保留原文案：%v", err)
	}
	pidPath := filepath.Join(t.TempDir(), "no.pid")
	if err := reload(writeTestConfig(t, validConfig), pidPath); err != nil {
		t.Fatalf("没在跑时旧 reload 应成功：%v", err)
	}
}

func TestServiceReloadNotRunning(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	path := writeTestConfig(t, validConfig)
	_, _, code := runCLI(t, "service", "reload", "--config", path, "--json")
	if code != 4 {
		t.Fatalf("exit = %d, want 4", code)
	}
	_, _, code = runCLI(t, "service", "reload", "--config", path, "--dry-run", "--json")
	if code != 4 {
		t.Fatalf("dry-run 没在跑 exit = %d, want 4", code)
	}
}

func TestConfigApplyRequiresYes(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	_, _, code := runCLI(t, "config", "apply", "--config", f.main, "--candidate", f.dir, "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestConfigApplyDryRunWritesNothing(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	before, err := cfgver.TreeHash(f.main)
	if err != nil {
		t.Fatal(err)
	}
	out, _, code := runCLI(t, "config", "apply", "--config", f.main, "--candidate", f.dir, "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	data := doc["data"].(map[string]any)
	if data["dry_run"] != true {
		t.Fatalf("data = %v", data)
	}
	if _, has := data["operation_id"]; has {
		t.Fatal("dry-run 默认不应创建 operation")
	}
	after, err := cfgver.TreeHash(f.main)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("dry-run 改变了线上哈希")
	}
}

func TestConfigApplyValidationFails(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	if err := os.WriteFile(f.site, []byte(`{"listen":[1],"server_name":"","location":[{"url":"/","type":"local","root":"."}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cand := f.dir
	live := writeFixture(t, defaultAcmeJSON)
	_, _, code := runCLI(t, "config", "apply", "--config", live.main, "--candidate", cand, "--yes", "--json")
	if code != 5 {
		t.Fatalf("exit = %d, want 5", code)
	}
}

func TestCandidatePathTraversalApply(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	_, _, code := runCLI(t, "config", "apply", "--config", f.main, "--candidate", filepath.Join("..", "x"), "--yes", "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestConfigRollbackMissingVersion(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	_, _, code := runCLI(t, "config", "rollback", "--config", f.main, "--version", "v999", "--yes", "--json")
	if code != 9 {
		t.Fatalf("exit = %d, want 9", code)
	}
}

func TestReloadPortPrecheckCLI(t *testing.T) {
	// 端口预检在 svcctl 单测覆盖；这里确认旧 reload 在配置无效时仍不走到预检。
	TestReloadStopsOnInvalidConfig(t)
}

// ===== merged from cli_candidate_test.go =====
func TestCandidatePathTraversal(t *testing.T) {
	out, _, code := runCLI(t, "config", "validate", "--candidate", "../x", "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	body := doc["error"].(map[string]any)
	if body["code"] != cli.CodeUsage {
		t.Fatalf("error = %v", body)
	}

	cand := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(cand, "fast-https.json"), []byte(`{"http":{"include":["conf.d"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(cand, "conf.d")); err != nil {
		t.Skipf("无法创建符号链接：%v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "site.json"), []byte(`{"listen":[8080],"server_name":"x","location":[{"url":"/","root":"."}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code = runCLI(t, "config", "validate", "--candidate", cand, "--json")
	if code != 2 {
		t.Fatalf("symlink escape exit = %d, want 2\n%s", code, out)
	}
}

func TestConfigValidateCandidateTreeInvalid(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	if err := os.WriteFile(f.site, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCLI(t, "config", "validate", "--candidate", f.dir, "--json")
	if code != 5 {
		t.Fatalf("exit = %d, want 5\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	body := doc["error"].(map[string]any)
	if body["code"] != cli.CodeConfigInvalid {
		t.Fatalf("error = %v", body)
	}
	if !strings.Contains(body["message"].(string), f.site) {
		t.Fatalf("错误应指向候选文件：%v", body["message"])
	}
}

func TestConfigDiffCandidateDir(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	cand := t.TempDir()
	if err := copyTree(f.dir, cand); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(cand, "conf.d", "demo.json")
	raw, err := os.ReadFile(site)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), `"server_name": "demo.example"`, `"server_name": "cand.example"`, 1)
	if err := os.WriteFile(site, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCLI(t, "config", "diff", "--current", f.dir, "--candidate", cand, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["identical"] == true {
		t.Fatalf("目录树应有差异：%v", data)
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	})
}

// ===== merged from cli_site_write_test.go =====
func TestSiteAddDryRunCLI(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	root := t.TempDir()
	out, errOut, code := runCLI(t, "site", "add", "--config", f.main, "--domain", "new.example",
		"--root", root, "--listen", "8081", "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s\n%s", code, out, errOut)
	}
	doc := decodeOneDocument(t, out)
	data := doc["data"].(map[string]any)
	if data["dry_run"] != true {
		t.Fatalf("data = %v", data)
	}
	if _, err := os.Stat(filepath.Join(f.confd, "new.example.json")); !os.IsNotExist(err) {
		t.Fatal("dry-run 不应写出线上站点文件")
	}
}

func TestSiteDeleteRequiresYes(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	_, _, code := runCLI(t, "site", "delete", "demo", "--config", f.main, "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestSiteAddDuplicateCLI(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	root := t.TempDir()
	_, _, code := runCLI(t, "site", "add", "--config", f.main, "--domain", "demo.example",
		"--root", root, "--yes", "--json")
	if code != 10 {
		t.Fatalf("同 server_name exit = %d, want 10", code)
	}
}

func TestSiteAddInvalidDomainOrListen(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	_, _, code := runCLI(t, "site", "add", "--config", f.main, "--domain", "../x",
		"--root", t.TempDir(), "--yes", "--json")
	if code != 2 {
		t.Fatalf("非法域名 exit = %d, want 2", code)
	}
	_, _, code = runCLI(t, "site", "add", "--config", f.main, "--domain", "ok.example",
		"--root", t.TempDir(), "--listen", "bad-port", "--yes", "--json")
	if code != 5 {
		t.Fatalf("非法 listen exit = %d, want 5", code)
	}
}

func TestSiteRootPathTraversalCLI(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	_, _, code := runCLI(t, "site", "add", "--config", f.main, "--domain", "x.example",
		"--root", filepath.Join("..", "..", "etc"), "--yes", "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	var out2, errOut string
	out2, errOut, code = runCLI(t, "site", "add", "--config", f.main, "--domain", "y.example",
		"--root", f.dir, "--yes", "--json")
	if code != 2 {
		t.Fatalf("指向配置目录 exit = %d, want 2, stdout=%s stderr=%s", code, out2, errOut)
	}
	stateDir := os.Getenv("FASTHTTPS_STATE_DIR")
	_, _, code = runCLI(t, "site", "add", "--config", f.main, "--domain", "z.example",
		"--root", stateDir, "--yes", "--json")
	if code != 2 {
		t.Fatalf("指向状态目录 exit = %d, want 2", code)
	}
}

func TestSiteUpdateWhitelist(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	f := writeFixture(t, defaultAcmeJSON)
	_, _, code := runCLI(t, "site", "update", "demo", "--config", f.main, "--type", "proxy", "--yes", "--json")
	if code != 2 {
		t.Fatalf("未列出字段 exit = %d, want 2", code)
	}
}

func TestSiteUpdateInlineNotSupportedCLI(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	dir := t.TempDir()
	confd := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "fast-https.json")
	body := `{
  "http": {
    "include": ["conf.d"],
    "server": [{"listen":[80],"server_name":"inline.example","location":[{"url":"/","type":"local","root":"."}]}]
  }
}`
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	_, _, code := runCLI(t, "site", "update", "inline.example", "--config", main, "--server-name", "x.example", "--yes", "--json")
	if code != 11 {
		t.Fatalf("内联站点 exit = %d, want 11", code)
	}
}

func TestLegacySiteInitRemoveRemoved(t *testing.T) {
	mainPath, _ := siteTestEnv(t)
	for _, sub := range []string{"init", "remove"} {
		_, errOut, code := runCLI(t, "site", sub, "blog", "--config", mainPath)
		if code == 0 {
			t.Fatalf("site %s 应已移除，却执行成功", sub)
		}
		if errOut == "" {
			t.Fatalf("site %s 失败但没有错误输出", sub)
		}
	}
}

func TestSiteListJSONHasEnabled(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	out, _, code := runCLI(t, "site", "list", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	row := decodeOneDocument(t, out)["data"].(map[string]any)["sites"].([]any)[0].(map[string]any)
	if row["enabled"] != true {
		t.Fatalf("enabled = %v", row["enabled"])
	}
}
