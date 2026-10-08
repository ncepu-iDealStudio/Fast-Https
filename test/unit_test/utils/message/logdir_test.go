package message_test

import (
	"fast-https/config"
	"fast-https/utils/message"
	"os"
	"path/filepath"
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
