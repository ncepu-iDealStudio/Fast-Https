package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"fast-https/config"
)

// 归档布局被 PATH 调用时，工作目录里没有配置、安装目录里有，才切目录。
func TestPortableLayoutDir(t *testing.T) {
	if filepath.IsAbs(config.CONFIG_FILE_PATH) {
		t.Skip("包布局使用绝对路径，不适用归档锚定")
	}
	root := t.TempDir()
	cwd := filepath.Join(root, "elsewhere")
	exeDir := filepath.Join(root, "install")
	for _, dir := range []string{cwd, exeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if _, ok := portableLayoutDir(cwd, exeDir); ok {
		t.Fatal("两边都没有配置时不应切换目录")
	}

	writeConfig(t, exeDir)
	if dir, ok := portableLayoutDir(cwd, exeDir); !ok || dir != exeDir {
		t.Fatalf("安装目录有配置时应切到 %q，得到 %q ok=%v", exeDir, dir, ok)
	}

	writeConfig(t, cwd)
	if _, ok := portableLayoutDir(cwd, exeDir); ok {
		t.Fatal("当前目录已有配置时不应切换目录")
	}
}

func writeConfig(t *testing.T, base string) {
	t.Helper()
	path := filepath.Join(base, config.CONFIG_FILE_PATH)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}
