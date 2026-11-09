package cmd

import (
	"os"
	"path/filepath"
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
