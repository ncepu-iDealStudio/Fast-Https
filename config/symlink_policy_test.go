package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func symlinkPolicyConfig(root, policy string) string {
	policyLine := ""
	if policy != "" {
		policyLine = `, "symlink_policy": "` + policy + `"`
	}
	return `{
  "http": {
    "server": [{
      "listen": 18081,
      "server_name": "localhost",
      "location": [{
        "url": "/", "type": "local",
        "root": "` + filepath.ToSlash(root) + `"` + policyLine + `,
        "index": ["index.html"]
      }]
    }]
  }
}`
}

func writeSymlinkPolicyConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// rootWithSymlink 返回一个内部含符号链接的站点目录。
func rootWithSymlink(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows 上创建符号链接需要开发者模式或管理员权限")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("创建符号链接失败：%v", err)
	}
	return root
}

// TestCheckAtLoadRejectsSymlink 默认策略在加载配置时拒绝含符号链接的 root。
func TestCheckAtLoadRejectsSymlink(t *testing.T) {
	root := rootWithSymlink(t)
	err := ValidateConfigFile(writeSymlinkPolicyConfig(t, symlinkPolicyConfig(root, "")))
	if err == nil {
		t.Fatal("默认 check-at-load 应拒绝含符号链接的 root")
	}
	if !strings.Contains(err.Error(), "symlink found under root") {
		t.Fatalf("错误信息应说明符号链接，得到：%v", err)
	}
}

// TestSymlinkPolicyAllowSkipsLoadCheck allow 策略保持旧行为。
func TestSymlinkPolicyAllowSkipsLoadCheck(t *testing.T) {
	root := rootWithSymlink(t)
	if err := ValidateConfigFile(writeSymlinkPolicyConfig(t, symlinkPolicyConfig(root, SYMLINK_ALLOW))); err != nil {
		t.Fatalf("allow 不应因符号链接拒绝加载：%v", err)
	}
	if err := ValidateConfigFile(writeSymlinkPolicyConfig(t, symlinkPolicyConfig(root, SYMLINK_REJECT))); err != nil {
		t.Fatalf("reject 交给运行期处理，加载不应失败：%v", err)
	}
}

// TestSymlinkPolicyInvalid 未知取值直接报错，避免拼错后静默退回默认。
func TestSymlinkPolicyInvalid(t *testing.T) {
	root := t.TempDir()
	err := ValidateConfigFile(writeSymlinkPolicyConfig(t, symlinkPolicyConfig(root, "deny")))
	if err == nil {
		t.Fatal("非法 symlink_policy 应被拒绝")
	}
	if !strings.Contains(err.Error(), "symlink_policy is invalid") {
		t.Fatalf("错误信息应指出策略非法，得到：%v", err)
	}
}

// TestSymlinkPolicyDefaultNormalized 空值按 check-at-load 转换。
func TestSymlinkPolicyDefaultNormalized(t *testing.T) {
	if got := normalizeSymlinkPolicy(" "); got != SYMLINK_CHECK_AT_LOAD {
		t.Fatalf("空值应归一化为 %s，得到 %s", SYMLINK_CHECK_AT_LOAD, got)
	}
	if got := normalizeSymlinkPolicy(" ALLOW "); got != SYMLINK_ALLOW {
		t.Fatalf("大小写与空白应被归一化，得到 %s", got)
	}
}
