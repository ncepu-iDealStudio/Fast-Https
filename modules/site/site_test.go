package site

import (
	"fast-https/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validInput(name string) config.SiteInput {
	return config.SiteInput{
		Listens:    []string{"80"},
		ServerName: name + ".example",
		Locations: []config.SiteLocation{{
			URL:   "/",
			Type:  "local",
			Root:  "./httpdoc/root",
			Index: []string{"index.html"},
		}},
	}
}

func TestValidateName(t *testing.T) {
	ok := []string{"blog", "a1", "static-site", "v1.2"}
	for _, name := range ok {
		if err := ValidateName(name); err != nil {
			t.Fatalf("ValidateName(%q) = %v", name, err)
		}
	}
	bad := []string{"", " ", "../x", "a/b", ".", "..", `a\b`}
	for _, name := range bad {
		if err := ValidateName(name); err == nil {
			t.Fatalf("ValidateName(%q) should fail", name)
		}
	}
}

func TestCreateFindListRemove(t *testing.T) {
	dir := t.TempDir()
	path, err := Create([]string{dir}, "blog", validInput("blog"), false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if path != filepath.Join(dir, "blog.json") {
		t.Fatalf("path = %q", path)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file should be renamed away")
	}

	found, ok := Find([]string{dir}, "blog")
	if !ok || found != path {
		t.Fatalf("find = %q, %v", found, ok)
	}

	entries, err := List([]string{dir})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].ServerName != "blog.example" {
		t.Fatalf("entries = %+v", entries)
	}
	if len(entries[0].Types) != 1 || !strings.Contains(entries[0].Types[0], "local") {
		t.Fatalf("types = %v", entries[0].Types)
	}

	removed, err := Remove([]string{dir}, "blog")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed != path {
		t.Fatalf("removed = %q", removed)
	}
	if _, ok := Find([]string{dir}, "blog"); ok {
		t.Fatal("site should be gone")
	}
}

func TestCreateRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if _, err := Create([]string{dir}, "blog", validInput("blog"), false); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := Create([]string{dir}, "blog", validInput("blog"), false); err == nil {
		t.Fatal("existing file should be refused without --force")
	}
	if _, err := Create([]string{dir}, "blog", validInput("other"), true); err != nil {
		t.Fatalf("force create: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "blog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"other.example"`) {
		t.Fatalf("force should overwrite:\n%s", raw)
	}
}

// 校验失败时不能落盘，也不能留下 .tmp。
func TestCreateInvalidLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	bad := config.SiteInput{Listens: []string{"80"}, ServerName: "bad.example"} // 没有 location
	if _, err := Create([]string{dir}, "bad", bad, false); err == nil {
		t.Fatal("site without location should be rejected")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("dir should stay empty, got %d entries", len(entries))
	}
}

func TestCreateWithoutIncludeDir(t *testing.T) {
	if _, err := Create(nil, "blog", validInput("blog"), false); err == nil {
		t.Fatal("empty include dirs should be reported")
	}
}

func TestListReportsBrokenFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := List([]string{dir})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Invalid == "" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestRemoveUnknownSite(t *testing.T) {
	if _, err := Remove([]string{t.TempDir()}, "nope"); err == nil {
		t.Fatal("removing an unknown site should fail")
	}
}
