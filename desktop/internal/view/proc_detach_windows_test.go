//go:build windows

package view

import (
	"os/exec"
	"syscall"
	"testing"
)

// TestSetDetachedFlags 锁住服务进程的分离标志。
//
// 主程序在 Windows 上不做 daemonize（`start` 就是前台服务进程本身），
// 所以桌面端启动它时必须彻底分离：独立进程组 + 无控制台 + DETACHED。
// 少了任何一项，桌面端退出或作业对象回收时会把服务连带终止——
// 用户看到的现象就是「重载/重启经常失败、服务状态反复」。
func TestSetDetachedFlags(t *testing.T) {
	cmd := exec.Command("cmd")
	setDetached(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("setDetached 未设置 SysProcAttr")
	}
	flags := cmd.SysProcAttr.CreationFlags
	for _, want := range []struct {
		name string
		bit  uint32
	}{
		{"CREATE_NEW_PROCESS_GROUP", createNewProcessGroup},
		{"CREATE_NO_WINDOW", createNoWindow},
		{"DETACHED_PROCESS", detachedProcess},
	} {
		if flags&want.bit == 0 {
			t.Errorf("CreationFlags 缺少 %s (0x%X)，实际 0x%X", want.name, want.bit, flags)
		}
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow 应为 true，避免闪控制台")
	}
}

// TestSetDetachedKeepsExistingAttr 已有 SysProcAttr 时不应被覆盖成 nil。
func TestSetDetachedKeepsExistingAttr(t *testing.T) {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	setDetached(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("setDetached 不应清空已有的 SysProcAttr")
	}
	if cmd.SysProcAttr.CreationFlags == 0 {
		t.Error("应写入 CreationFlags")
	}
}
