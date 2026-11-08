package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"fast-https/runtime/metrics"
)

func TestRuntimeProcessRestartE2E(t *testing.T) {
	if os.Getenv("FASTHTTPS_E2E_RUNTIME") != "1" {
		t.Skip("set FASTHTTPS_E2E_RUNTIME=1 to run process restart / stop e2e")
	}

	if runtimeBusy() {
		t.Fatal("127.0.0.1:9088 is already in use; stop the other Fast-Https instance before this e2e")
	}

	repo, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	for _, dir := range []string{"config/cert", "config/conf.d", "httpdoc/root", "logs"} {
		if err := os.MkdirAll(filepath.Join(work, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyFile(filepath.Join(repo, "config", "mime.json"), filepath.Join(work, "config", "mime.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "httpdoc", "root", "index.html"), []byte("runtime-e2e"), 0o644); err != nil {
		t.Fatal(err)
	}

	port := reserveTCPPort(t)
	writeRuntimeSite(t, work, port)

	bin := filepath.Join(work, "fast-https-runtime-e2e.exe")
	build := exec.Command("go", "build", "-o", bin, "fast-https.go")
	build.Dir = repo
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	first, firstLog := startRuntimeServer(t, bin, work)
	t.Cleanup(func() { stopRuntimeWork(bin, work, first) })

	site := fmt.Sprintf("http://127.0.0.1:%d/", port)
	if err := waitPortState(fmt.Sprintf("127.0.0.1:%d", port), true, 30*time.Second); err != nil {
		t.Fatalf("first process site: %v\n%s", err, firstLog.String())
	}
	if err := waitRuntimeReady(15 * time.Second); err != nil {
		t.Fatalf("first process /runtime: %v\n%s", err, firstLog.String())
	}
	if err := httpGetResponds(site); err != nil {
		t.Fatalf("first process GET site: %v\n%s", err, firstLog.String())
	}

	snap1 := getRuntimeSnapshot(t)
	if snap1.Server.PID == 0 || snap1.Server.StartedAt == "" {
		t.Fatalf("first snapshot missing pid/started_at: %+v", snap1.Server)
	}
	if snap1.Requests.Total == 0 {
		t.Fatal("first process requests.total = 0 after a successful GET")
	}

	if err := runStop(bin, work); err != nil {
		t.Fatalf("stop: %v\n%s", err, firstLog.String())
	}
	_ = first.Wait()

	if err := waitPortState("127.0.0.1:9088", false, 10*time.Second); err != nil {
		t.Fatalf("after stop /runtime still reachable: %v", err)
	}
	pidFile := filepath.Join(work, "fast-https.pid")
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatalf("pid file still present after stop: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Log("Windows stop uses Kill; not asserting stopping/SIGTERM")
	}

	second, secondLog := startRuntimeServer(t, bin, work)
	t.Cleanup(func() { stopRuntimeWork(bin, work, second) })
	if err := waitPortState(fmt.Sprintf("127.0.0.1:%d", port), true, 30*time.Second); err != nil {
		t.Fatalf("second process site: %v\n%s", err, secondLog.String())
	}
	if err := waitRuntimeReady(15 * time.Second); err != nil {
		t.Fatalf("second process /runtime: %v\n%s", err, secondLog.String())
	}

	snap2 := getRuntimeSnapshot(t)
	if snap2.Server.PID == 0 || snap2.Server.PID == snap1.Server.PID {
		t.Fatalf("second pid = %d, first pid = %d, want a new pid", snap2.Server.PID, snap1.Server.PID)
	}
	if snap2.Server.StartedAt == "" || snap2.Server.StartedAt == snap1.Server.StartedAt {
		t.Fatalf("second started_at = %q, first = %q, want a new started_at", snap2.Server.StartedAt, snap1.Server.StartedAt)
	}
	if snap2.Requests.Total != 0 {
		t.Fatalf("second process requests.total = %d, want 0", snap2.Requests.Total)
	}

	if err := runStop(bin, work); err != nil {
		t.Fatalf("second stop: %v\n%s", err, secondLog.String())
	}
}

func startRuntimeServer(t *testing.T, bin, work string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	cmd := exec.Command(bin, "start")
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "FASTHTTPS_FOREGROUND=1")
	setReloadProcessGroup(cmd)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd, &out
}

func runStop(bin, work string) error {
	cmd := exec.Command(bin, "stop")
	cmd.Dir = work
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w; output: %s", err, out)
	}
	return nil
}

func stopRuntimeWork(bin, work string, cmd *exec.Cmd) {
	_ = runStop(bin, work)
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

func writeRuntimeSite(t *testing.T, work string, port int) {
	t.Helper()
	main := fmt.Sprintf(`{
  "http": {
    "server": [{
      "listen": "%d",
      "server_name": "127.0.0.1",
      "location": [{
        "url": "/",
        "type": "local",
        "root": "./httpdoc/root",
        "index": ["index.html"]
      }]
    }],
    "include": []
  }
}
`, port)
	if err := os.WriteFile(filepath.Join(work, "config", "fast-https.json"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runtimeBusy() bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:9088", 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func waitRuntimeReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	var last error
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://127.0.0.1:9088/runtime")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	return last
}

func getRuntimeSnapshot(t *testing.T) metrics.Snapshot {
	t.Helper()
	resp, err := http.Get("http://127.0.0.1:9088/runtime")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var snap metrics.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	return snap
}
