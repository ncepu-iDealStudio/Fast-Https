//go:build unix

package procctl

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

// 普通用户对 root 进程执行 kill(pid, 0) 得到 EPERM：进程存在，只是没有
// 权限向它发信号。必须按「在跑」处理——否则非 root 的 stop 会把 root
// 启动实例的 pid 文件当成残留删掉，之后连 sudo stop 也找不到进程。
//
// pid 1 在 Unix 上总是存在且通常属于 root，用它构造这个场景。当前环境
// 构造成 EPERM（root 或 pid 1 同属当前用户）时跳过。
func TestRunningPermissionDeniedCountsAsAlive(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 可以给任何进程发信号，构造不出 EPERM")
	}
	p, err := os.FindProcess(1)
	if err != nil {
		t.Skipf("find pid 1: %v", err)
	}
	sigErr := p.Signal(syscall.Signal(0))
	if sigErr == nil {
		t.Skip("pid 1 belongs to the current user; no EPERM to test")
	}
	if !errors.Is(sigErr, os.ErrPermission) {
		t.Skipf("signal 0 on pid 1 returned %v, not a permission error", sigErr)
	}
	if !Running(1) {
		t.Fatal("EPERM proves the process exists; Running must not report it as gone")
	}
}
