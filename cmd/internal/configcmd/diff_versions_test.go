package configcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fast-https/cmd/internal/cli"
	"fast-https/management/cfgdiff"
	"fast-https/management/cfgver"
	"fast-https/management/operation"
	"fast-https/management/state"
)

// config diff --current v001 --candidate v002 要展示「从 v001 到 v002 改了什么」。
func TestConfigDiffBetweenVersions(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("FASTHTTPS_STATE_DIR", stateDir)
	main := filepath.Join(t.TempDir(), "fast-https.json")
	writeMain := func(body string) {
		if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st := state.Dir{Root: stateDir}
	certDir := filepath.Join(stateDir, "cert")

	writeMain(`{"cache":[{"name":"default_cache","path":"./cache","valid":["200:1h"],"max_size":0}]}`)
	before, err := cfgver.SnapshotWithCerts(st, main, certDir, "before delete", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	writeMain(`{"log_root":"./logs"}`)
	after, err := cfgver.SnapshotWithCerts(st, main, certDir, "after delete", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"diff", "--current", before.Version, "--candidate", after.Version, "--config", main})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("diff: %v\n%s", err, out.String())
	}

	var res cfgdiff.Result
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &res); err != nil {
		t.Fatalf("解析 diff 输出: %v\n%s", err, out.String())
	}
	found := false
	for _, c := range res.Changes {
		if c.Path == "fast-https.json:cache" && c.Op == cfgdiff.OpRemoved {
			found = true
		}
	}
	if !found {
		t.Fatalf("v001 → v002 应显示 cache 被删除：%+v", res.Changes)
	}
}

// versions delete --dry-run 只校验和预览，不删目录、不要求确认。
func TestConfigVersionsDeleteDryRunKeepsSnapshot(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("FASTHTTPS_STATE_DIR", stateDir)
	main := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(main, []byte(`{"log_root":"./logs"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st := state.Dir{Root: stateDir}
	man, err := cfgver.SnapshotWithCerts(st, main, filepath.Join(stateDir, "cert"), "one", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// 改动 live 配置，让该快照不再是 active。
	if err := os.WriteFile(main, []byte(`{"log_root":"./logs2"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"versions", "delete", man.Version, "--dry-run", "--config", main})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(cfgver.VersionDir(st, man.Version)); err != nil {
		t.Fatalf("dry-run 不应删除快照目录: %v", err)
	}
	if !strings.Contains(out.String(), "dry-run") {
		t.Fatalf("dry-run 输出应说明未改动：%s", out.String())
	}
}

// active 版本即使带 --dry-run 也必须报冲突，与真实删除保持同一套前置校验。
func TestConfigVersionsDeleteDryRunRejectsActive(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("FASTHTTPS_STATE_DIR", stateDir)
	main := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(main, []byte(`{"log_root":"./logs"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st := state.Dir{Root: stateDir}
	man, err := cfgver.SnapshotWithCerts(st, main, filepath.Join(stateDir, "cert"), "one", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"versions", "delete", man.Version, "--dry-run", "--config", main})
	err = cmd.Execute()
	var ce *cli.CLIError
	if !errors.As(err, &ce) || ce.Code != cli.CodeConflict {
		t.Fatalf("err = %v, want CONFLICT", err)
	}
	if _, statErr := os.Stat(cfgver.VersionDir(st, man.Version)); statErr != nil {
		t.Fatalf("冲突时不应删除快照目录: %v", statErr)
	}
}

// 非终端删除必须显式 --yes。
func TestConfigVersionsDeleteNonTerminalNeedsYes(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("FASTHTTPS_STATE_DIR", stateDir)
	main := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(main, []byte(`{"log_root":"./logs"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st := state.Dir{Root: stateDir}
	man, err := cfgver.SnapshotWithCerts(st, main, filepath.Join(stateDir, "cert"), "one", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte(`{"log_root":"./logs2"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"versions", "delete", man.Version, "--config", main})
	err = cmd.Execute()
	var ce *cli.CLIError
	if !errors.As(err, &ce) || ce.Code != cli.CodeUsage {
		t.Fatalf("err = %v, want USAGE_ERROR", err)
	}
	if _, statErr := os.Stat(cfgver.VersionDir(st, man.Version)); statErr != nil {
		t.Fatalf("拒绝后不应删除快照目录: %v", statErr)
	}
}

// 操作 ID 也能作为 diff/rollback 的引用，解析到该操作的结果版本目录。
func TestResolveDiffRefOperation(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("FASTHTTPS_STATE_DIR", stateDir)
	main := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(main, []byte(`{"log_root":"./logs"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st := state.Dir{Root: stateDir}
	man, err := cfgver.SnapshotWithCerts(st, main, filepath.Join(stateDir, "cert"), "one", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rec := operation.Record{
		ID: "op_20260102_001", Command: "config.apply", Status: operation.StatusSucceeded,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Result:    map[string]any{"version": man.Version},
	}
	store := operation.Store{Dir: st}
	if err := store.Save(rec); err != nil {
		t.Fatal(err)
	}
	got, err := resolveDiffRef("candidate", rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := cfgver.VersionDir(st, man.Version)
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("resolved = %s, want %s", got, want)
	}
}
