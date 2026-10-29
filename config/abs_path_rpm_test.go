//go:build rpm

package config

import (
	"path/filepath"
	"testing"
)

// 装了包之后从任意目录执行 fast-https，配置里的相对路径都必须落在
// FAST_HTTPS_BASE_DIR 下，而不是跟着进程的工作目录走。
func TestAbsConfigPathIgnoresWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())

	base, err := filepath.Abs(FAST_HTTPS_BASE_DIR)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "config", "conf.d")
	if got := absConfigPath("./config/conf.d"); got != want {
		t.Fatalf("absConfigPath = %q, want %q", got, want)
	}
}
