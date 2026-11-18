//go:build windows

package cli

import (
	"os/exec"
	"syscall"
)

// createNoWindow 对应 CREATE_NO_WINDOW：不为子进程分配控制台窗口。
const createNoWindow = 0x08000000

// hideConsole 让子进程不要新建控制台窗口。
//
// 主程序是控制台子系统（这样 cmd / PowerShell 才会等待它退出），桌面端
// 作为 GUI 进程拉起它时如果不隐藏，每次 CLI 调用都会闪一下黑窗；而
// 设置页、证书页、站点页都会周期性调用 CLI，表现为「不停频闪」。
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindow
	cmd.SysProcAttr.HideWindow = true
}
