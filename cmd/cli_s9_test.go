package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	_, _, code = runCLI(t, "site", "add", "--config", f.main, "--domain", "y.example",
		"--root", f.dir, "--yes", "--json")
	if code != 2 {
		t.Fatalf("指向配置目录 exit = %d, want 2", code)
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

func TestLegacySiteInitRemoveUnchanged(t *testing.T) {
	mainPath, includeDir := siteTestEnv(t)
	out, _, code := runCLI(t, "site", "init", "blog", "--config", mainPath,
		"--non-interactive", "--listen", "80", "--server-name", "blog.example", "--root", "./httpdoc/root")
	if code != 0 {
		t.Fatalf("init exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "站点文件已生成") || !strings.Contains(out, "fast-https reload") {
		t.Fatalf("旧 init 输出应变：%s", out)
	}
	if _, err := os.Stat(filepath.Join(includeDir, "blog.json")); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runCLI(t, "site", "remove", "blog", "--config", mainPath)
	if code != 1 {
		t.Fatalf("remove 无 --yes exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "加 --yes") {
		t.Fatalf("stderr = %q", errOut)
	}
	out, _, code = runCLI(t, "site", "remove", "blog", "--config", mainPath, "--yes")
	if code != 0 {
		t.Fatalf("remove --yes exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "已删除") || !strings.Contains(out, "fast-https reload") {
		t.Fatalf("旧 remove 输出应变：%s", out)
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
