package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/management/cfgver"
)

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
