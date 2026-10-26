//go:build windows && amd64

package main

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestResolvePath(t *testing.T) {
	abs := `C:\fast-https\fast-https.pid`
	if got := resolvePath(abs); got != abs {
		t.Errorf("绝对路径应原样返回，got %q", got)
	}
	if got := resolvePath("./fast-https.pid"); got != filepath.Join(exeDir(), "fast-https.pid") {
		t.Errorf("相对路径应挂到程序目录，got %q", got)
	}
	if got := resolvePath(""); got != "" {
		t.Errorf("空路径应原样返回，got %q", got)
	}
}

// TestAcquireSingleInstance 验证第二个实例拿不到锁，
// 第一个实例释放后可以再次获取。
func TestAcquireSingleInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.lock")

	first, ok := acquireSingleInstance(path)
	if !ok {
		t.Fatal("首个实例应当获取成功")
	}
	if first == 0 {
		t.Fatal("首个实例应当返回有效句柄")
	}

	second, ok := acquireSingleInstance(path)
	if ok {
		t.Error("第二个实例不应获取成功")
	}
	if second != 0 {
		t.Error("失败的调用不应返回句柄")
	}

	if err := windows.CloseHandle(first); err != nil {
		t.Fatalf("关闭句柄失败: %v", err)
	}

	third, ok := acquireSingleInstance(path)
	if !ok {
		t.Fatal("前一个实例退出后应当可以重新获取")
	}
	if err := windows.CloseHandle(third); err != nil {
		t.Fatalf("关闭句柄失败: %v", err)
	}
}

func TestProcessAliveRejectsInvalidPid(t *testing.T) {
	for _, pid := range []string{"", "0", "-1", "not-a-pid"} {
		if processAlive(pid) {
			t.Errorf("pid %q 不应被判为存活", pid)
		}
	}
}
