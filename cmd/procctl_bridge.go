package cmd

import (
	"time"

	"fast-https/cmd/internal/procctl"
)

// pid 文件与进程状态的实现已迁到 cmd/internal/procctl。这里保留 cmd 包内
// 的旧名字，让调用点不必与迁移同步改动；迁移收尾时整体删除。

// ProcessState 是读 pid 文件并检查对应进程后的结论（别名）。
type ProcessState = procctl.ProcessState

const (
	ProcessAbsent  = procctl.ProcessAbsent
	ProcessStale   = procctl.ProcessStale
	ProcessRunning = procctl.ProcessRunning
)

// WritePid 把当前进程的 pid 写入指定文件。
func WritePid(path string) error { return procctl.WritePid(path) }

func writePidTo(path string, pid int) error { return procctl.WritePidTo(path, pid) }

func readPid(path string) (int, error) { return procctl.ReadPid(path) }

// InspectProcess 报告 pidFile 指向的进程状态。
func InspectProcess(pidFile string) (ProcessState, int, error) {
	return procctl.InspectProcess(pidFile)
}

func processRunning(pid int) bool { return procctl.Running(pid) }

func waitProcessGone(pid int, timeout time.Duration) bool {
	return procctl.WaitGone(pid, timeout)
}
