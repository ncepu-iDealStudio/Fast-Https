package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplacedLogIsReadFromStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, eventsFile)
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	offset, prefix, lines, err := readNewLines(dir, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "old" || offset != 4 || prefix == "" {
		t.Fatalf("first read offset=%d prefix=%s lines=%v", offset, prefix, lines)
	}
	replacement := strings.Repeat("x", 40) + "\nFIRST\nSECOND\n"
	if int64(len(replacement)) <= offset {
		t.Fatal("replacement must be larger than the old offset")
	}
	if err := os.WriteFile(path, []byte(replacement), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, lines, err = readNewLines(dir, offset, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) < 2 || !strings.Contains(lines[0], "x") || lines[len(lines)-1] != "SECOND" {
		t.Fatalf("rotated read = %v", lines)
	}
}
