package pathutil

import (
	"path/filepath"
	"testing"
)

// Resolve 展开数据目录内的相对路径，绝对路径只做 Clean。
func TestResolve(t *testing.T) {
	base := t.TempDir()
	if got, want := Resolve(base, "./cache/edge"), filepath.Join(base, "cache", "edge"); got != want {
		t.Fatalf("relative: %q, want %q", got, want)
	}
	abs := filepath.Join(base, "cache", "edge")
	if got := Resolve(base, abs); got != abs {
		t.Fatalf("absolute: %q", got)
	}
	if got := Resolve(base, "  "); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

// Compact 收回数据目录内的绝对路径，目录外保留绝对写法。
func TestCompact(t *testing.T) {
	base := t.TempDir()
	if got := Compact(base, filepath.Join(base, "cache", "edge")); got != "./cache/edge" {
		t.Fatalf("inside: %q", got)
	}
	outside := filepath.Join(filepath.Dir(base), "outside", "site")
	if got := Compact(base, outside); got != outside {
		t.Fatalf("outside: %q", got)
	}
	if got := Compact(base, "./cache/edge"); got != "./cache/edge" {
		t.Fatalf("already relative: %q", got)
	}
	if got := Compact(base, ""); got != "" {
		t.Fatalf("empty: %q", got)
	}
}
