//go:build !windows

package procctl

import (
	"os"
	"syscall"
)

// Running 用 signal 0 探测进程是否存在。保留 !windows 而不是 unix：
// 与迁移前的构建面一致，plan9/js 等目标也能编译。
func Running(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
