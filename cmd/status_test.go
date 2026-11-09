package cmd

import (
	"fast-https/cmd/internal/procctl"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writePidTo 写入的是调用方给的 pid（后台化时父进程写子进程的 pid），
// 不能像旧实现那样永远写自己的 pid。
func TestWritePidToRecordsGivenPid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fast-https.pid")
	if err := procctl.WritePidTo(path, 4321); err != nil {
		t.Fatal(err)
	}
	pid, err := procctl.ReadPid(path)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 4321 {
		t.Fatalf("pid = %d, want 4321", pid)
	}
}

// waitProcessGone 对活着的进程必须等满超时后返回 false。
func TestWaitProcessGoneTimesOut(t *testing.T) {
	start := time.Now()
	if procctl.WaitGone(os.Getpid(), 200*time.Millisecond) {
		t.Fatal("current process is alive, waitProcessGone must not report gone")
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("returned after %v, before the timeout", elapsed)
	}
}

func TestInspectProcessAbsent(t *testing.T) {
	state, pid, err := procctl.InspectProcess(filepath.Join(t.TempDir(), "missing.pid"))
	if err != nil {
		t.Fatal(err)
	}
	if state != procctl.ProcessAbsent || pid != 0 {
		t.Fatalf("state=%v pid=%d", state, pid)
	}
}

func TestInspectProcessStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fast-https.pid")
	if err := os.WriteFile(path, []byte(`{"pid":2147483646}`), 0o644); err != nil {
		t.Fatal(err)
	}
	state, pid, err := procctl.InspectProcess(path)
	if err != nil {
		t.Fatal(err)
	}
	if procctl.Running(2147483646) {
		t.Skip("pid 2147483646 is in use on this machine")
	}
	if state != procctl.ProcessStale || pid != 2147483646 {
		t.Fatalf("state=%v pid=%d", state, pid)
	}
}

func TestInspectProcessRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fast-https.pid")
	if err := procctl.WritePid(path); err != nil {
		t.Fatal(err)
	}
	state, pid, err := procctl.InspectProcess(path)
	if err != nil {
		t.Fatal(err)
	}
	if state != procctl.ProcessRunning || pid != os.Getpid() {
		t.Fatalf("state=%v pid=%d, want running %d", state, pid, os.Getpid())
	}
}
