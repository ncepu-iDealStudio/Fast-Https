package cfgver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fast-https/management/state"
)

func TestSnapshotExcludesCerts(t *testing.T) {
	main, dir := writeTree(t)
	if err := os.WriteFile(filepath.Join(dir, "conf.d", "secret.pem"), []byte("PRIVATE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "site.key"), []byte("KEY"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := state.Dir{Root: filepath.Join(dir, "state")}
	man, err := Snapshot(st, main, "init", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(st.VersionsDir(), man.Version)
	err = filepath.WalkDir(snap, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".pem" || ext == ".key" || ext == ".crt" {
			t.Errorf("快照里出现了证书文件 %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestActiveByHash(t *testing.T) {
	main, dir := writeTree(t)
	st := state.Dir{Root: filepath.Join(dir, "state")}
	if _, err := Snapshot(st, main, "one", time.Now()); err != nil {
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
	if _, err := Snapshot(st, main, "one", time.Now()); err != nil {
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
	st := state.Dir{Root: filepath.Join(dir, "state")}
	man, err := Snapshot(st, main, "one", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(st.VersionsDir(), man.Version)
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

func jsonQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
