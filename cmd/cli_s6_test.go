package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fast-https/management/cfgver"
	"fast-https/management/operation"
	"fast-https/management/state"
)

func TestConfigVersionsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FASTHTTPS_STATE_DIR", dir)
	f := writeFixture(t, defaultAcmeJSON)
	out, _, code := runCLI(t, "config", "versions", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	vers, _ := data["versions"].([]any)
	if len(vers) != 0 {
		t.Fatalf("首次应为空列表：%v", vers)
	}
}

func TestOperationListEmptyAndGetMissing(t *testing.T) {
	t.Setenv("FASTHTTPS_STATE_DIR", t.TempDir())
	out, _, code := runCLI(t, "operation", "list", "--json")
	if code != 0 {
		t.Fatalf("list exit = %d\n%s", code, out)
	}
	_, _, code = runCLI(t, "operation", "get", "op_nonexistent", "--json")
	if code != 9 {
		t.Fatalf("missing get exit = %d", code)
	}
}

func TestOperationReconcileCLI(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FASTHTTPS_STATE_DIR", root)
	ops := filepath.Join(root, "operations")
	if err := os.MkdirAll(ops, 0o700); err != nil {
		t.Fatal(err)
	}
	id := "op_20261108_001"
	body := `{"operation_id":"` + id + `","command":"config.apply","status":"running","created_at":"2026-11-08T12:00:00Z","owner_pid":999999,"owner_start_key":"dead","phase":"write","result":{}}`
	if err := os.WriteFile(filepath.Join(ops, id+".json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCLI(t, "operation", "get", id, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["status"] != "unknown" {
		t.Fatalf("status = %v", data["status"])
	}
	result, _ := data["result"].(map[string]any)
	if result["last_phase"] != "write" {
		t.Fatalf("result = %v", result)
	}
}

func TestRequestIDConflictExitCode(t *testing.T) {
	if got := ExitCode(fromStateError("operation.get", operation.ErrRequestIDConflict)); got != 10 {
		t.Fatalf("exit = %d, want 10", got)
	}
}

func TestConfigDiffCLI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FASTHTTPS_STATE_DIR", dir)
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte(`{"n":1,"pswd":"OLD"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`{"n":2,"pswd":"NEW"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCLI(t, "config", "diff", "--current", a, "--candidate", b, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if strings.Contains(out, "OLD") || strings.Contains(out, "NEW") {
		t.Fatalf("diff 输出了口令：%s", out)
	}
}

func TestConfigVersionsActiveAndDiffVersion(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	root := t.TempDir()
	t.Setenv("FASTHTTPS_STATE_DIR", root)
	st := state.Dir{Root: root}
	man, err := cfgver.Snapshot(st, f.main, "init", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	out, _, code := runCLI(t, "config", "versions", "--config", f.main, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["drift"] != false {
		t.Fatalf("drift = %v", data["drift"])
	}
	out, _, code = runCLI(t, "config", "diff", "--config", f.main, "--candidate", man.Version, "--json")
	if code != 0 {
		t.Fatalf("diff version exit = %d\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)["data"].(map[string]any)
	if doc["identical"] != true {
		t.Fatalf("与自身快照比较应 identical：%v", doc)
	}
}
