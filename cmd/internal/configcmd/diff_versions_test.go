package configcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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
