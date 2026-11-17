package static

import (
	"fast-https/config"
	"fast-https/runtime/listener"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSymlinkPolicyRejectEscapingLink reject 策略下，路径经过链接即按 404 处理。
func TestSymlinkPolicyRejectEscapingLink(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("top-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("创建符号链接失败：%v", err)
	}

	full := filepath.Join(link, "secret.txt")
	if !symlinkBlocked(config.SYMLINK_REJECT, root, full) {
		t.Error("reject 应拦截指向 root 之外的链接")
	}
	if symlinkBlocked(config.SYMLINK_ALLOW, root, full) {
		t.Error("allow 不应拦截链接")
	}
	if symlinkBlocked(config.SYMLINK_CHECK_AT_LOAD, root, full) {
		t.Error("check-at-load 只在加载期生效，请求期不应拦截")
	}
}

// TestSymlinkPolicyRejectBlocksInnerLink root 内部的链接同样被 reject 拦截：
// 该策略的语义是"静态根下不允许链接"，与 check-at-load 的严格度一致。
func TestSymlinkPolicyRejectBlocksInnerLink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "real", "index.html")
	if err := os.WriteFile(file, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(filepath.Join(root, "real"), link); err != nil {
		t.Skipf("创建符号链接失败：%v", err)
	}

	if !symlinkBlocked(config.SYMLINK_REJECT, root, filepath.Join(link, "index.html")) {
		t.Error("reject 应拦截 root 内部的链接")
	}
	if symlinkBlocked(config.SYMLINK_REJECT, root, file) {
		t.Error("真实路径不应被拦截")
	}
}

// TestSymlinkPolicyRejectBlocksJunction Windows junction 在 Go 1.27 上既不是
// ModeSymlink，EvalSymlinks 也不会解析，必须靠 ModeIrregular + Readlink 识别。
func TestSymlinkPolicyRejectBlocksJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 有 junction")
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("top-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	junc := filepath.Join(root, "junc")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", junc, outside).CombinedOutput(); err != nil {
		t.Skipf("创建 junction 失败：%v (%s)", err, out)
	}

	if !symlinkBlocked(config.SYMLINK_REJECT, root, filepath.Join(junc, "secret.txt")) {
		t.Error("reject 应拦截 junction")
	}
}

// TestSymlinkPolicyRejectPlainFile 普通文件不受影响。
func TestSymlinkPolicyRejectPlainFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "index.html")
	if err := os.WriteFile(file, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if symlinkBlocked(config.SYMLINK_REJECT, root, file) {
		t.Error("普通文件不应被 reject 拦截")
	}
}

// TestSymlinkPolicyRejectBlocksIndexLink 目录请求会退到索引文件，
// 索引文件是链接时也必须重新过策略，否则请求路径本身不越界就能读到外部文件。
func TestSymlinkPolicyRejectBlocksIndexLink(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.html")
	if err := os.WriteFile(secret, []byte("top-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(root, "index.html")); err != nil {
		t.Skipf("创建符号链接失败：%v", err)
	}
	try := &listener.Try{Files: []string{"index.html"}}

	found, file, _, _ := dofind(root, try, root, config.SYMLINK_REJECT)
	if found {
		file.Close()
		t.Fatal("reject 应拦截指向 root 之外的索引链接")
	}

	found, file, _, _ = dofind(root, try, root, config.SYMLINK_ALLOW)
	if !found {
		t.Fatal("allow 应能读到索引链接")
	}
	file.Close()
}

// TestSymlinkPolicyRejectBlocksTryNextLink try.Next 同样是请求路径的延伸。
func TestSymlinkPolicyRejectBlocksTryNextLink(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.html")
	if err := os.WriteFile(secret, []byte("top-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(root, "index.html")); err != nil {
		t.Skipf("创建符号链接失败：%v", err)
	}
	try := &listener.Try{Next: "index.html"}

	found, file, _, _ := dofind(root, try, root, config.SYMLINK_REJECT)
	if found {
		file.Close()
		t.Fatal("reject 应拦截指向 root 之外的 try.Next 链接")
	}

	found, file, _, _ = dofind(root, try, root, config.SYMLINK_ALLOW)
	if !found {
		t.Fatal("allow 应能读到 try.Next 链接")
	}
	file.Close()
}
