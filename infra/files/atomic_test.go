package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicCreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conf", "fast-https.json")

	if err := WriteAtomic(path, []byte("first"), 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "first" {
		t.Fatalf("read = %q, %v", got, err)
	}

	if err := WriteAtomic(path, []byte("second"), 0o600); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "second" {
		t.Fatalf("read after replace = %q, %v", got, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file must be renamed away, got %v", err)
	}
}

// 写入失败不能留下 .tmp：读者按 .json 扫描目录时不能被临时文件带偏。
func TestWriteAtomicCleansTempOnFailure(t *testing.T) {
	dir := t.TempDir()
	// 目标路径是一个已存在的目录：rename 会失败。
	target := filepath.Join(dir, "occupied")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := WriteAtomic(target, []byte("x"), 0o644); err == nil {
		t.Fatal("writing onto a directory should fail")
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file must be removed on failure, got %v", err)
	}
}
