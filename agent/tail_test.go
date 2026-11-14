package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatedLogUsesNewFileIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, eventsFile)
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	offset, prefix, lines, err := readNewLines(dir, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "old" || !strings.HasPrefix(prefix, "id:") {
		t.Fatalf("first read offset=%d prefix=%s lines=%v", offset, prefix, lines)
	}
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	replacement := strings.Repeat("x", int(offset)+8) + "\nFIRST\nSECOND\n"
	if err := os.WriteFile(path, []byte(replacement), 0o644); err != nil {
		t.Fatal(err)
	}
	_, next, lines, err := readNewLines(dir, offset, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if next == prefix {
		t.Fatal("rotated file kept the previous identity")
	}
	if len(lines) < 2 || !strings.Contains(lines[0], "x") || lines[len(lines)-1] != "SECOND" {
		t.Fatalf("rotated read = %v", lines)
	}
}

func TestAppendKeepsFileIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, eventsFile)
	if err := os.WriteFile(path, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	offset, prefix, _, err := readNewLines(dir, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("two\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	_, next, lines, err := readNewLines(dir, offset, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if next != prefix {
		t.Fatalf("append changed identity %s -> %s", prefix, next)
	}
	if len(lines) != 1 || lines[0] != "two" {
		t.Fatalf("append read = %v", lines)
	}
}
