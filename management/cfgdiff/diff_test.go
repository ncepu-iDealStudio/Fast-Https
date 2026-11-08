package cfgdiff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffClassifiesChanges(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	write(t, a, `{"keep":1,"gone":2,"n":3}`)
	write(t, b, `{"keep":1,"added":4,"n":9}`)
	res, err := Files(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Identical {
		t.Fatal("应有差异")
	}
	ops := map[string]string{}
	for _, c := range res.Changes {
		ops[c.Path] = c.Op
	}
	if ops["added"] != OpAdded || ops["gone"] != OpRemoved || ops["n"] != OpModified {
		t.Fatalf("changes = %+v", res.Changes)
	}
}

func TestDiffNeverShowsSecrets(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	write(t, a, `{"auth":{"user":"admin","pswd":"OLD-SECRET"}}`)
	write(t, b, `{"auth":{"user":"admin","pswd":"NEW-SECRET"}}`)
	res, err := Files(a, b)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(res)
	text := string(raw)
	if strings.Contains(text, "OLD-SECRET") || strings.Contains(text, "NEW-SECRET") {
		t.Fatalf("差异里出现了口令：%s", text)
	}
	if len(res.Changes) != 1 || !res.Changes[0].Changed || res.Changes[0].Op != OpModified {
		t.Fatalf("changes = %+v", res.Changes)
	}
}

func TestTreesDiffsConfdFiles(t *testing.T) {
	current := t.TempDir()
	candidate := t.TempDir()
	writeTreeJSON(t, current, `{"n":1}`, `{"server_name":"a"}`)
	writeTreeJSON(t, candidate, `{"n":1}`, `{"server_name":"b"}`)
	res, err := Trees(current, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if res.Identical {
		t.Fatal("conf.d 不同应有差异")
	}
	found := false
	for _, c := range res.Changes {
		if strings.Contains(c.Path, "conf.d/") && c.Op == OpModified {
			found = true
		}
	}
	if !found {
		t.Fatalf("changes = %+v", res.Changes)
	}
}

func writeTreeJSON(t *testing.T, root, main, site string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "fast-https.json"), main)
	write(t, filepath.Join(root, "conf.d", "site.json"), site)
	write(t, filepath.Join(root, "manifest.json"), `{"version":"v001"}`)
}

func TestDiffIgnoresComments(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	write(t, a, `{"n":1}`)
	write(t, b, "// only a comment\n{\"n\":1}")
	res, err := Files(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Identical || len(res.Changes) != 0 {
		t.Fatalf("只改注释应 identical：%+v", res)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
