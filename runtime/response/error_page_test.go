package response

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUseFileKeepsDefaultWhenMissing(t *testing.T) {
	res := DefaultNotFound()
	original := string(res.Body)
	UseFile(res, filepath.Join(t.TempDir(), "missing.html"))
	if string(res.Body) != original {
		t.Fatal("missing file replaced the default body")
	}
}

func TestUseFileReplacesBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "404.html")
	if err := os.WriteFile(path, []byte("custom-page"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := DefaultServerError()
	UseFile(res, path)
	if string(res.Body) != "custom-page" {
		t.Fatalf("body %q", res.Body)
	}
	if res.Headers["Content-Length"] != "11" {
		t.Fatalf("length %s", res.Headers["Content-Length"])
	}
}
