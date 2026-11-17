package static

import (
	"fast-https/config"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSymlinkPolicyRejectEscapingLink reject 策略下，链接指向 root 之外按 404 处理。
func TestSymlinkPolicyRejectEscapingLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 上创建符号链接需要开发者模式或管理员权限")
	}
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

// TestSymlinkPolicyRejectKeepsInnerLink root 内部的链接不受 reject 影响。
func TestSymlinkPolicyRejectKeepsInnerLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 上创建符号链接需要开发者模式或管理员权限")
	}
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

	if symlinkBlocked(config.SYMLINK_REJECT, root, filepath.Join(link, "index.html")) {
		t.Error("reject 只拦截逃出 root 的链接，内部链接应放行")
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
