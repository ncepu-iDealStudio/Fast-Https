package paths

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// root 自身是链接时不算违规：部署方可以把整个站点目录挂到别处。
func TestFirstSymlinkUnderAllowsLinkedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 上创建符号链接需要开发者模式或管理员权限")
	}
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkRoot := filepath.Join(t.TempDir(), "site")
	if err := os.Symlink(real, linkRoot); err != nil {
		t.Skipf("创建符号链接失败：%v", err)
	}

	got, err := FirstSymlinkUnder(linkRoot)
	if err != nil {
		t.Fatalf("FirstSymlinkUnder: %v", err)
	}
	if got != "" {
		t.Fatalf("root 自身是链接不应违规，got %q", got)
	}
}

// junction 在 Go/Windows 上不是 ModeSymlink，EvalSymlinks 也不解析，
// 只能靠 ModeIrregular + Readlink 识别（见 isLink）。
func TestFirstSymlinkUnderDetectsJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 有 junction")
	}
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	junc := filepath.Join(root, "junc")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", junc, target).CombinedOutput(); err != nil {
		t.Skipf("创建 junction 失败：%v (%s)", err, out)
	}

	got, err := FirstSymlinkUnder(root)
	if err != nil {
		t.Fatalf("FirstSymlinkUnder: %v", err)
	}
	if got != junc {
		t.Fatalf("junction = %q, want %q", got, junc)
	}
}
