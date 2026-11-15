package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPidFileStates(t *testing.T) {
	dir := t.TempDir()

	t.Run("absent", func(t *testing.T) {
		pid, state, err := ReadPidFile(filepath.Join(dir, "none.json"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state != PidAbsent || pid != 0 {
			t.Fatalf("state/pid = %s/%d, want absent/0", state, pid)
		}
	})

	t.Run("alive", func(t *testing.T) {
		path := filepath.Join(dir, "alive.json")
		writePidFile(t, path, os.Getpid())
		pid, state, err := ReadPidFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state != PidAlive || pid != os.Getpid() {
			t.Fatalf("state/pid = %s/%d, want alive/%d", state, pid, os.Getpid())
		}
	})

	t.Run("stale", func(t *testing.T) {
		const deadPID = 999999
		if processRunning(deadPID) {
			t.Skipf("pid %d is in use on this machine", deadPID)
		}
		path := filepath.Join(dir, "stale.json")
		writePidFile(t, path, deadPID)
		pid, state, err := ReadPidFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state != PidStale || pid != deadPID {
			t.Fatalf("state/pid = %s/%d, want stale/%d", state, pid, deadPID)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		path := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, state, err := ReadPidFile(path)
		if err == nil {
			t.Fatal("want an error for a malformed pid file")
		}
		if state != PidUnknown {
			t.Fatalf("state = %s, want unknown", state)
		}
	})
}
