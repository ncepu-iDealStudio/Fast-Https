//go:build windows

package svcctl

import (
	"os"
	"strings"
	"testing"
)

// TestSignalReloadRefusesSelf 不允许向自身进程发 reload 信号。
//
// Windows 的 GenerateConsoleCtrlEvent 只能发给整个控制台进程组，发给自身
// 会打断当前进程（用户看到的退出码 0xc000013a）。这条断言锁住该保护。
func TestSignalReloadRefusesSelf(t *testing.T) {
	err := SignalReload(os.Getpid())
	if err == nil {
		t.Fatal("向自身进程发 reload 信号应被拒绝")
	}
	if !strings.Contains(err.Error(), "自身") {
		t.Errorf("错误信息应说明原因，得到：%v", err)
	}
}

// TestSignalReloadReturnsUnsupportedOnWindows 在 Windows 上必须返回
// not supported 错误，让上层降级成「重启服务」而不是硬发热重载信号。
//
// 背景：曾经这里会尝试 CTRL_BREAK / CTRL_C，结果打断服务进程——
// 用户点「禁用站点」后服务直接停了。现在约定：Windows 一律返回错误，
// 由 cmd/internal/cli 的 Reloader 捕获后重启服务。
func TestSignalReloadReturnsUnsupportedOnWindows(t *testing.T) {
	// 用一个几乎不可能存在的 pid，避开「自身进程」分支
	err := SignalReload(0x7FFFFFF0)
	if err == nil {
		t.Fatal("Windows 上 SignalReload 应返回错误（不支持定向投递）")
	}
	msg := err.Error()
	if !strings.Contains(msg, "not supported") && !strings.Contains(msg, "找不到") {
		t.Errorf("错误应表明 Windows 不支持或进程不存在，得到：%v", err)
	}
}

// TestSignalReloadRejectsInvalidPID 非正 pid 直接拒绝。
func TestSignalReloadRejectsInvalidPID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if err := SignalReload(pid); err == nil {
			t.Errorf("pid=%d 应被拒绝", pid)
		}
	}
}
