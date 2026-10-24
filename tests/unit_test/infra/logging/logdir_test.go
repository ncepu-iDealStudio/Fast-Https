package logging_test

import (
	"fast-https/config"
	"fast-https/infra/logging"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenWritesFourFiles(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)

	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		config.SYSTEM_LOG_NAME,
		config.ACCESS_LOG_NAME,
		config.ERROR_LOG_NAME,
		config.SAFE_LOG_NAME,
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected %s in %s: %v", name, dir, err)
		}
	}
}

func TestOpenSwitchesDirectory(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)

	if err := logging.Open(first); err != nil {
		t.Fatal(err)
	}
	logging.PrintInfo("first-line")
	if err := logging.Open(second); err != nil {
		t.Fatal(err)
	}
	logging.PrintInfo("second-line")

	oldLog := readLog(t, filepath.Join(first, config.SYSTEM_LOG_NAME))
	newLog := readLog(t, filepath.Join(second, config.SYSTEM_LOG_NAME))
	if !strings.Contains(oldLog, "first-line") || strings.Contains(oldLog, "second-line") {
		t.Fatalf("old log = %q", oldLog)
	}
	if !strings.Contains(newLog, "second-line") {
		t.Fatalf("new log = %q", newLog)
	}
}

func TestOpenKeepsOldFilesWhenNewDirFails(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(logging.CloseLogFiles)

	if err := logging.Open(dir); err != nil {
		t.Fatal(err)
	}
	logging.PrintInfo("kept")

	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := logging.Open(filepath.Join(blocker, "logs")); err == nil {
		t.Fatal("expected Open to fail when the new directory cannot be created")
	}
	logging.PrintInfo("still")

	body := readLog(t, filepath.Join(dir, config.SYSTEM_LOG_NAME))
	if !strings.Contains(body, "kept") || !strings.Contains(body, "still") {
		t.Fatalf("old log should stay writable, got %q", body)
	}
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
