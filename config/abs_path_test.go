package config

import (
	"os"
	"path/filepath"
	"testing"
)

// 拆掉安装根下的 config 层之后，1.5.0 存量配置里的 "./config/xxx" 与
// "config/xxx" 都要回退到新布局；新写法一次命中，不触发回退。
func TestAbsPathInTrimsLegacyConfigPrefix(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "cert", "acme"), 0o755); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(base, "cert", "acme")
	for _, legacy := range []string{"./config/cert/acme", "config/cert/acme"} {
		if got := absPathIn(base, legacy); got != want {
			t.Fatalf("absPathIn(%q) = %q, want %q", legacy, got, want)
		}
	}
	if got := absPathIn(base, "./cert/acme"); got != want {
		t.Fatalf("absPathIn new layout = %q, want %q", got, want)
	}
}

// 主路径存在时必须直接用主路径，不能被回退规则改写；两者都不存在时返回
// 主路径，让调用方的报错信息指向新布局。
func TestAbsPathInPrefersPrimaryPath(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "config", "cert"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "cert"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got, want := absPathIn(base, "config/cert"), filepath.Join(base, "config", "cert"); got != want {
		t.Fatalf("absPathIn existing primary = %q, want %q", got, want)
	}
	if got, want := absPathIn(base, "./missing/x"), filepath.Join(base, "missing", "x"); got != want {
		t.Fatalf("absPathIn missing = %q, want %q", got, want)
	}
}

// 绝对路径与空串原样返回，不参与基址拼接。
func TestAbsPathInPassThrough(t *testing.T) {
	base := t.TempDir()
	abs := filepath.Join(base, "x.pem")
	if got := absPathIn(base, abs); got != abs {
		t.Fatalf("absPathIn(abs) = %q, want %q", got, abs)
	}
	if got := absPathIn(base, "   "); got != "" {
		t.Fatalf("absPathIn(blank) = %q, want empty", got)
	}
}
