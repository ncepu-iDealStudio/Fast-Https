package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectProcessAbsent(t *testing.T) {
	state, pid, err := InspectProcess(filepath.Join(t.TempDir(), "missing.pid"))
	if err != nil {
		t.Fatal(err)
	}
	if state != ProcessAbsent || pid != 0 {
		t.Fatalf("state=%v pid=%d", state, pid)
	}
}

func TestInspectProcessStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fast-https.pid")
	if err := os.WriteFile(path, []byte(`{"pid":2147483646}`), 0o644); err != nil {
		t.Fatal(err)
	}
	state, pid, err := InspectProcess(path)
	if err != nil {
		t.Fatal(err)
	}
	if processRunning(2147483646) {
		t.Skip("pid 2147483646 is in use on this machine")
	}
	if state != ProcessStale || pid != 2147483646 {
		t.Fatalf("state=%v pid=%d", state, pid)
	}
}

func TestInspectProcessRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fast-https.pid")
	if err := WritePid(path); err != nil {
		t.Fatal(err)
	}
	state, pid, err := InspectProcess(path)
	if err != nil {
		t.Fatal(err)
	}
	if state != ProcessRunning || pid != os.Getpid() {
		t.Fatalf("state=%v pid=%d, want running %d", state, pid, os.Getpid())
	}
}
