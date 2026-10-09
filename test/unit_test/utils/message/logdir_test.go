package message_test

import (
	"fast-https/config"
	"fast-https/utils/message"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveLogDirDefaultsToLogs(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	for _, input := range []string{"", ".", "./"} {
		got := message.ResolveLogDir(input)
		if filepath.Clean(got) != filepath.Clean(config.DEFAULT_LOG_ROOT) {
			t.Fatalf("ResolveLogDir(%q)=%q, want %q", input, got, config.DEFAULT_LOG_ROOT)
		}
		info, err := os.Stat(filepath.Join(tmp, "logs"))
		if err != nil {
			t.Fatalf("logs directory should be created: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("logs path should be a directory")
		}
	}
}

func TestMessageFormatWritesFourFiles(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(message.CloseLogFiles)
	message.MessageFormat(dir)
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

func TestReopenWritesNewDirectory(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	t.Cleanup(message.CloseLogFiles)
	if err := message.Reopen(first); err != nil {
		t.Fatal(err)
	}
	message.Glog.SystemLog.Info("first-line")
	if err := message.Reopen(second); err != nil {
		t.Fatal(err)
	}
	message.Glog.SystemLog.Info("second-line")

	oldLog := readLog(t, filepath.Join(first, config.SYSTEM_LOG_NAME))
	newLog := readLog(t, filepath.Join(second, config.SYSTEM_LOG_NAME))
	if !strings.Contains(oldLog, "first-line") || strings.Contains(oldLog, "second-line") {
		t.Fatalf("old log = %q", oldLog)
	}
	if !strings.Contains(newLog, "second-line") {
		t.Fatalf("new log = %q", newLog)
	}
}

func TestReopenKeepsOldFilesWhenNewDirFails(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(message.CloseLogFiles)
	if err := message.Reopen(dir); err != nil {
		t.Fatal(err)
	}
	message.Glog.SystemLog.Info("kept")

	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := message.Reopen(filepath.Join(blocker, "logs")); err == nil {
		t.Fatal("expected reopen to fail when the new directory cannot be created")
	}
	message.Glog.SystemLog.Info("still")

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
