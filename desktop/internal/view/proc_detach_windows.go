//go:build windows

package view

import (
	"os/exec"
	"syscall"
)

// Windows 进程创建标志：让服务进程彻底脱离桌面端。
//
// CREATE_NEW_PROCESS_GROUP (0x00000200)：独立进程组，Ctrl 事件不会波及
// CREATE_NO_WINDOW         (0x08000000)：不分配控制台窗口
// DETACHED_PROCESS         (0x00000008)：不继承父进程控制台
//
// 主程序在 Windows 上不做 daemonize（start 就是前台服务进程），所以
// 桌面端必须把它分离出去，否则桌面端退出时服务被连带终止——用户看到的
// 就是「重启/重载经常失败、状态反复」。
const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
	detachedProcess       = 0x00000008
)

func setDetached(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = createNewProcessGroup | createNoWindow | detachedProcess
	cmd.SysProcAttr.HideWindow = true
}
