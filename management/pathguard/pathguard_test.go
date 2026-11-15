package pathguard

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckRootRejectsDotDotAndBanned(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "www")
	if err := os.MkdirAll(doc, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := CheckRoot(".."+string(filepath.Separator)+"etc", nil); !errors.Is(err, ErrPath) {
		t.Fatalf(".. 应拒绝：%v", err)
	}
	if _, err := CheckRoot(cfg, []string{cfg}); !errors.Is(err, ErrPath) {
		t.Fatalf("配置目录应拒绝：%v", err)
	}
	got, err := CheckRoot(doc, []string{cfg})
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("应返回绝对路径")
	}
}
