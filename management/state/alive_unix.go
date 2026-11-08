//go:build unix

package state

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// processStartKey 在 Linux 上取 /proc/pid/stat 的启动 tick，用来识别 PID 复用。
// 其它 Unix 没有稳定的跨平台接口，返回空串，只凭进程是否还在。
func processStartKey(pid int) string {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	s := string(raw)
	i := strings.LastIndex(s, ") ")
	if i < 0 {
		return ""
	}
	fields := strings.Fields(s[i+2:])
	// /proc/pid/stat：comm 之后第 20 个字段（从 0 计）是 starttime。
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}
