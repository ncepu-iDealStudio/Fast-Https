//go:build windows

package cli

import (
	"os/exec"
	"syscall"
	"testing"
)

// TestHideConsole 桌面端调 CLI 时必须隐藏控制台，否则轮询时窗口频闪。
func TestHideConsole(t *testing.T) {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x1}
	hideConsole(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("hideConsole 不应清空已有的 SysProcAttr")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Error("CreationFlags 应包含 CREATE_NO_WINDOW")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow 应为 true")
	}
}
