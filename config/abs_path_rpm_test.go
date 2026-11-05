//go:build rpm

package config

import (
	"path/filepath"
	"testing"
)

// 装了包之后从任意目录执行 fast-https，配置里的相对路径都必须落在
// ETC 基址（/etc/fast-https）下，而不是跟着进程的工作目录走。
func TestAbsETCPathIgnoresWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())

	base, err := filepath.Abs(FAST_HTTPS_ETC_DIR)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "conf.d")
	if got := absETCPath("./conf.d"); got != want {
		t.Fatalf("absETCPath = %q, want %q", got, want)
	}
}
