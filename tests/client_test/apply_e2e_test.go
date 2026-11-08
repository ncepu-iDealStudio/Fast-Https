package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestApplyRollbackE2E(t *testing.T) {
	if os.Getenv("FASTHTTPS_E2E_APPLY") != "1" {
		t.Skip("set FASTHTTPS_E2E_APPLY=1 to run config apply/rollback e2e test")
	}
	if runtime.GOOS == "windows" {
		t.Log("Windows: skip killing the CLI mid-apply (plan step 5)")
	}

	repo, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	cfgDir := filepath.Join(work, "config")
	docOld := filepath.Join(work, "httpdoc", "root")
	docNew := filepath.Join(work, "httpdoc", "alt")
	for _, d := range []string{filepath.Join(cfgDir, "conf.d"), docOld, docNew} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyFile(filepath.Join(repo, "config", "mime.json"), filepath.Join(cfgDir, "mime.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docOld, "index.html"), []byte("OLD_PAGE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docNew, "index.html"), []byte("NEW_PAGE"), 0o644); err != nil {
		t.Fatal(err)
	}

	port := 19180
	writeE2EMain(t, cfgDir, port, docOld)
	cand := filepath.Join(work, "cand")
	if err := copyDir(cfgDir, cand); err != nil {
		t.Fatal(err)
	}
	writeE2EMain(t, cand, port, docNew)

	bin := filepath.Join(work, "fast-https-apply-e2e.exe")
	build := exec.Command("go", "build", "-o", bin, "fast-https.go")
	build.Dir = repo
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "start")
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "FASTHTTPS_FOREGROUND=1")
	setReloadProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command(bin, "stop").Run()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if err := waitPortState(addr, true, 25*time.Second); err != nil {
		t.Fatalf("server start: %v", err)
	}
	if body := httpBody(t, "http://"+addr+"/"); body != "OLD_PAGE" {
		t.Fatalf("initial body = %q", body)
	}

	apply := exec.Command(bin, "config", "apply", "--candidate", cand, "--yes", "--request-id", "e2e-1", "--json")
	apply.Dir = work
	out, err := apply.CombinedOutput()
	if err != nil {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	if err := waitHTTPBody("http://"+addr+"/", "NEW_PAGE", 20*time.Second); err != nil {
		t.Fatal(err)
	}

	verOut, err := exec.Command(bin, "config", "versions", "--json").CombinedOutput()
	if err != nil {
		ver := exec.Command(bin, "config", "versions", "--json")
		ver.Dir = work
		verOut, err = ver.CombinedOutput()
		if err != nil {
			t.Fatalf("versions: %v\n%s", err, verOut)
		}
	}

	var env struct {
		Data struct {
			Versions []struct{ Version string } `json:"versions"`
		} `json:"data"`
	}
	_ = json.Unmarshal(verOut, &env)
	if len(env.Data.Versions) < 2 {
		t.Fatalf("want at least 2 versions, got %s", verOut)
	}
	restore := env.Data.Versions[len(env.Data.Versions)-1].Version
	for _, v := range env.Data.Versions {
		if v.Version < restore {
			restore = v.Version
		}
	}

	busy, err := net.Listen("tcp", "127.0.0.1:19181")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	bad := filepath.Join(work, "cand-bad")
	if err := copyDir(cand, bad); err != nil {
		t.Fatal(err)
	}
	writeE2EMain(t, bad, 19181, docNew)
	deny := exec.Command(bin, "config", "apply", "--candidate", bad, "--yes", "--json")
	deny.Dir = work
	if out, err := deny.CombinedOutput(); err == nil {
		t.Fatalf("occupied port should be rejected\n%s", out)
	}
	if body := httpBody(t, "http://"+addr+"/"); body != "NEW_PAGE" {
		t.Fatalf("after rejected apply body = %q", body)
	}

	rb := exec.Command(bin, "config", "rollback", "--version", restore, "--yes", "--json")
	rb.Dir = work
	if out, err := rb.CombinedOutput(); err != nil {
		t.Fatalf("rollback: %v\n%s", err, out)
	}
	if err := waitHTTPBody("http://"+addr+"/", "OLD_PAGE", 20*time.Second); err != nil {
		t.Fatal(err)
	}
}

func writeE2EMain(t *testing.T, cfgDir string, port int, root string) {
	t.Helper()
	main := fmt.Sprintf(`{
  "http": {
    "include": ["./conf.d"],
    "server": []
  }
}`)
	site := fmt.Sprintf(`{
  "listen": [%d],
  "server_name": "localhost",
  "location": [{"url": "/", "type": "local", "root": %q, "index": ["index.html"]}]
}`, port, root)
	if err := os.WriteFile(filepath.Join(cfgDir, "fast-https.json"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "conf.d", "site.json"), []byte(site), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o644)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		return copyFile(path, to)
	})
}

func httpBody(t *testing.T, url string) string {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func waitHTTPBody(url, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(url)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			last = string(b)
			if last == want {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("body want %q, last %q", want, last)
}
