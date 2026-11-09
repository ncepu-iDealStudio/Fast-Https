package cmd

import (
	"fast-https/cmd/internal/cli"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCandidatePathTraversal(t *testing.T) {
	out, _, code := runCLI(t, "config", "validate", "--candidate", "../x", "--json")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	body := doc["error"].(map[string]any)
	if body["code"] != cli.CodeUsage {
		t.Fatalf("error = %v", body)
	}

	cand := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(cand, "fast-https.json"), []byte(`{"http":{"include":["conf.d"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(cand, "conf.d")); err != nil {
		t.Skipf("无法创建符号链接：%v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "site.json"), []byte(`{"listen":[8080],"server_name":"x","location":[{"url":"/","root":"."}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code = runCLI(t, "config", "validate", "--candidate", cand, "--json")
	if code != 2 {
		t.Fatalf("symlink escape exit = %d, want 2\n%s", code, out)
	}
}

func TestConfigValidateCandidateTreeInvalid(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	if err := os.WriteFile(f.site, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCLI(t, "config", "validate", "--candidate", f.dir, "--json")
	if code != 5 {
		t.Fatalf("exit = %d, want 5\n%s", code, out)
	}
	doc := decodeOneDocument(t, out)
	body := doc["error"].(map[string]any)
	if body["code"] != cli.CodeConfigInvalid {
		t.Fatalf("error = %v", body)
	}
	if !strings.Contains(body["message"].(string), f.site) {
		t.Fatalf("错误应指向候选文件：%v", body["message"])
	}
}

func TestConfigDiffCandidateDir(t *testing.T) {
	f := writeFixture(t, defaultAcmeJSON)
	cand := t.TempDir()
	if err := copyTree(f.dir, cand); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(cand, "conf.d", "demo.json")
	raw, err := os.ReadFile(site)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), `"server_name": "demo.example"`, `"server_name": "cand.example"`, 1)
	if err := os.WriteFile(site, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCLI(t, "config", "diff", "--current", f.dir, "--candidate", cand, "--json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	data := decodeOneDocument(t, out)["data"].(map[string]any)
	if data["identical"] == true {
		t.Fatalf("目录树应有差异：%v", data)
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	})
}
