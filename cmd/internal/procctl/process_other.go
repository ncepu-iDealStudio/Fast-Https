//go:build !windows

package procctl

import (
	"errors"
	"os"
	"syscall"
)

// Running 用 signal 0 探测进程是否存在。保留 !windows 而不是 unix：
// 与迁移前的构建面一致，plan9/js 等目标也能编译。
//
// EPERM 必须按「在跑」处理：服务由 root 启动、CLI 以普通用户运行时，
// kill(pid, 0) 返回的是 EPERM 而不是成功，但它证明进程确实存在。把它
// 当成「没在跑」会让 stop 走清理残留 pid 的分支删掉 pid 文件，之后
// 连 sudo stop 也找不到进程了。
func Running(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, os.ErrPermission)
}
