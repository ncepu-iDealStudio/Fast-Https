package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"fast-https/modules/core/listener"
)

func TestDialFailureKeepsDefaultWithoutPage(t *testing.T) {
	res := DialFailureResponse(nil)
	if res.FirstLine == "" || len(res.Body) == 0 {
		t.Fatal("expected the default server error")
	}
}

func TestDialFailureUsesConfiguredPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy.html")
	if err := os.WriteFile(path, []byte("upstream-down"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := DialFailureResponse(&listener.ListenCfg{ErrorProxy: path})
	if string(res.Body) != "upstream-down" {
		t.Fatalf("body %q", res.Body)
	}
}
