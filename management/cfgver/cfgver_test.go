package cfgver

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"fast-https/management/state"
)

func TestSnapshotIncludesCertsAndSkipsACME(t *testing.T) {
	main, dir := writeTree(t)
	certDir := filepath.Join(dir, "certs")
	if err := os.MkdirAll(filepath.Join(certDir, "acme"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(certDir, "example.pem"), "CERT")
	writeFile(t, filepath.Join(certDir, "example-key.pem"), "KEY")
	writeFile(t, filepath.Join(certDir, "acme", "account.key"), "ACCOUNT")

	st := state.Dir{Root: filepath.Join(dir, "state")}
	man, err := SnapshotWithCerts(st, main, certDir, "init", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(st.SnapshotsDir(), man.Version)
	for _, rel := range []string{"certs/example.pem", "certs/example-key.pem"} {
		if _, err := os.Stat(filepath.Join(snap, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s should be snapshot: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(snap, "certs", "acme", "account.key")); !os.IsNotExist(err) {
		t.Fatal("acme storage must not be snapshot")
	}
	if len(man.Certs) != 2 {
		t.Fatalf("certs = %+v", man.Certs)
	}
}

func TestSnapshotReusesSameTree(t *testing.T) {
	main, dir := writeTree(t)
	certDir := filepath.Join(dir, "certs")
	st := state.Dir{Root: filepath.Join(dir, "state")}
	a, err := SnapshotWithCerts(st, main, certDir, "one", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, err := SnapshotWithCerts(st, main, certDir, "two", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if a.Version != b.Version {
		t.Fatalf("same tree should reuse version: %s vs %s", a.Version, b.Version)
	}
	if n := countSnapshots(t, st); n != 1 {
		t.Fatalf("snapshots = %d, want 1", n)
	}
}

func TestRestoreCerts(t *testing.T) {
	main, dir := writeTree(t)
	certDir := filepath.Join(dir, "certs")
	writeFile(t, filepath.Join(certDir, "example.pem"), "OLD-CERT")
	writeFile(t, filepath.Join(certDir, "example-key.pem"), "OLD-KEY")

	st := state.Dir{Root: filepath.Join(dir, "state")}
	man, err := SnapshotWithCerts(st, main, certDir, "init", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(certDir, "example.pem"), "NEW-CERT")
	writeFile(t, filepath.Join(certDir, "example-key.pem"), "NEW-KEY")

	restored, err := RestoreCerts(filepath.Join(st.SnapshotsDir(), man.Version), certDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored) != 2 {
		t.Fatalf("restored = %v", restored)
	}
	if got := readFile(t, filepath.Join(certDir, "example.pem")); got != "OLD-CERT" {
		t.Fatalf("cert = %q", got)
	}
	if got := readFile(t, filepath.Join(certDir, "example-key.pem")); got != "OLD-KEY" {
		t.Fatalf("key = %q", got)
	}
}

func TestBackupAndRestoreCerts(t *testing.T) {
	root := t.TempDir()
	certDir := filepath.Join(root, "cert")
	backup := filepath.Join(root, "backup")
	writeFile(t, filepath.Join(certDir, "a.pem"), "A")
	if err := BackupCerts(certDir, backup); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(certDir, "a.pem"), "B")
	if err := RestoreCertBackup(backup, certDir); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(certDir, "a.pem")); got != "A" {
		t.Fatalf("cert = %q", got)
	}
}

func TestVersionIDFormat(t *testing.T) {
	main, dir := writeTree(t)
	st := state.Dir{Root: filepath.Join(dir, "state")}
	certDir := filepath.Join(dir, "certs")
	now := time.Date(2026, 11, 18, 14, 32, 0, 0, time.Local)
	a, err := SnapshotWithCerts(st, main, certDir, "one", now)
	if err != nil {
		t.Fatal(err)
	}
	if a.Version != "v2026.11.18.1" {
		t.Fatalf("version = %s, want v2026.11.18.1", a.Version)
	}
	if err := os.WriteFile(main, []byte(`{"http":{"include":[`+jsonQuote(filepath.Join(dir, "conf.d"))+`]},"x":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := SnapshotWithCerts(st, main, certDir, "two", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != "v2026.11.18.2" {
		t.Fatalf("version = %s, want v2026.11.18.2", b.Version)
	}
}

func TestDeleteVersion(t *testing.T) {
	main, dir := writeTree(t)
	st := state.Dir{Root: filepath.Join(dir, "state")}
	certDir := filepath.Join(dir, "certs")
	first, err := SnapshotWithCerts(st, main, certDir, "one", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte(`{"http":{"include":[`+jsonQuote(filepath.Join(dir, "conf.d"))+`]},"x":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := SnapshotWithCerts(st, main, certDir, "two", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := Delete(st, first.Version, main); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.SnapshotsDir(), first.Version)); !os.IsNotExist(err) {
		t.Fatal("删除后目录应不存在")
	}
	if err := Delete(st, second.Version, main); !errors.Is(err, ErrActiveVersion) {
		t.Fatalf("删除 active 版本应报错，得到 %v", err)
	}
	if err := Delete(st, "not-a-version", main); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("不存在的版本应报错，得到 %v", err)
	}
}

func TestActiveByHash(t *testing.T) {
	main, dir := writeTree(t)
	st := state.Dir{Root: filepath.Join(dir, "state")}
	if _, err := SnapshotWithCerts(st, main, filepath.Join(dir, "certs"), "one", time.Now()); err != nil {
		t.Fatal(err)
	}
	list, err := List(st, main)
	if err != nil {
		t.Fatal(err)
	}
	if list.Drift || len(list.Versions) != 1 || !list.Versions[0].Active {
		t.Fatalf("list = %+v", list)
	}
}

func TestDriftDetected(t *testing.T) {
	main, dir := writeTree(t)
	st := state.Dir{Root: filepath.Join(dir, "state")}
	if _, err := SnapshotWithCerts(st, main, filepath.Join(dir, "certs"), "one", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte(`{"http":{"include":[`+jsonQuote(filepath.Join(dir, "conf.d"))+`]},"x":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := List(st, main)
	if err != nil {
		t.Fatal(err)
	}
	if !list.Drift || list.Versions[0].Active {
		t.Fatalf("手工改文件后应 drift：%+v", list)
	}
}

func TestSnapshotPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 没有 POSIX 权限位")
	}
	main, dir := writeTree(t)
	certDir := filepath.Join(dir, "certs")
	writeFile(t, filepath.Join(certDir, "example.pem"), "CERT")
	st := state.Dir{Root: filepath.Join(dir, "state")}
	man, err := SnapshotWithCerts(st, main, certDir, "one", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(st.SnapshotsDir(), man.Version)
	info, err := os.Stat(snap)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("快照目录权限 %o", info.Mode().Perm())
	}
	manInfo, err := os.Stat(filepath.Join(snap, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if manInfo.Mode().Perm() != 0o600 {
		t.Fatalf("manifest 权限 %o", manInfo.Mode().Perm())
	}
	certInfo, err := os.Stat(filepath.Join(snap, "certs", "example.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if certInfo.Mode().Perm() != 0o600 {
		t.Fatalf("证书快照权限 %o", certInfo.Mode().Perm())
	}
}

func writeTree(t *testing.T) (main, dir string) {
	t.Helper()
	dir = t.TempDir()
	confd := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	site := `{"listen":[8080],"server_name":"demo.example","location":[{"url":"/","type":"local","root":"."}]}`
	if err := os.WriteFile(filepath.Join(confd, "demo.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}
	main = filepath.Join(dir, "fast-https.json")
	body := `{"http":{"include":[` + jsonQuote(confd) + `]}}`
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return main, dir
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func countSnapshots(t *testing.T, dir state.Dir) int {
	t.Helper()
	entries, err := os.ReadDir(dir.SnapshotsDir())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && IsVersionName(e.Name()) {
			n++
		}
	}
	return n
}

func jsonQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
