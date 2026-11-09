// Package procctl 管理 pid 文件与进程存活判断。
//
// 它刻意不 import 任何业务包（config / management / modules）：所有函数
// 都以路径或 pid 为参数，这样服务端、CLI、monitor 三处可以共用同一份
// 实现，也不会因为业务包反向依赖而形成环。
package procctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ProcessState 是读 pid 文件并检查对应进程后的结论。
type ProcessState int

const (
	// ProcessAbsent 表示没有 pid 文件（从未启动或已被清理）。
	ProcessAbsent ProcessState = iota
	// ProcessStale 表示 pid 文件存在但进程已经不在。
	ProcessStale
	// ProcessRunning 表示 pid 文件指向的进程仍在运行。
	ProcessRunning
)

// WritePid 把当前进程的 pid 写入指定文件。
// 使用 JSON 格式存储，便于后续进程管理。
func WritePid(path string) error {
	return WritePidTo(path, os.Getpid())
}

// WritePidTo 把指定进程的 pid 写进文件。后台化时由父进程写子进程的 pid，
// 这样 stop / reload / status 才能找到真正在服务的进程。
//
// 父目录缺失时先创建：包布局的 pid 在 /run/fast-https 下，/run 是 tmpfs，
// 重启后目录不存在，靠 systemd 的 RuntimeDirectory 或这里兜住手工启动。
func WritePidTo(path string, pid int) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("error creating pid directory %s: %v", dir, err)
		}
	}
	data, err := json.Marshal(map[string]int{"pid": pid})
	if err != nil {
		return fmt.Errorf("error marshalling PID and GID to JSON: %v", err)
	}

	// Windows 上目标文件偶尔会被杀毒/索引服务短暂占用，一次失败就放弃
	// 会导致 pid 文件停留在上一次的旧值（表现为「服务在跑但 stop/status
	// 找不到进程」），就地重试几次。
	var writeErr error
	for attempt := 0; attempt < 3; attempt++ {
		if writeErr = os.WriteFile(path, data, 0o644); writeErr == nil {
			return nil
		}
		time.Sleep(80 * time.Millisecond)
	}
	return fmt.Errorf("error writing JSON data to file: %v", writeErr)
}

// ReadPid 从指定文件读取并解析 pid。
//
// 底层错误用 %w 包装：调用方要靠 errors.Is(err, os.ErrNotExist) 区分
// 「没有 pid 文件」和「读不动」。
func ReadPid(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("error reading file: %w", err)
	}

	var pidGidMap map[string]int
	if err := json.Unmarshal(data, &pidGidMap); err != nil {
		return 0, fmt.Errorf("error unmarshalling JSON data: %v", err)
	}
	pid, ok := pidGidMap["pid"]
	if !ok {
		return 0, fmt.Errorf("PID or GID not found in JSON data")
	}
	return pid, nil
}

// InspectProcess 报告 pidFile 指向的进程状态。
// 没有 pid 文件时返回 ProcessAbsent 与 nil 错误——「从未启动」不是异常。
func InspectProcess(pidFile string) (ProcessState, int, error) {
	pid, err := ReadPid(pidFile)
	if err != nil {
		// 用 errors.Is 判断，别再比对错误字符串（ReadPid 换文案就失效）
		if errors.Is(err, os.ErrNotExist) {
			return ProcessAbsent, 0, nil
		}
		return ProcessAbsent, 0, err
	}
	if !Running(pid) {
		return ProcessStale, pid, nil
	}
	return ProcessRunning, pid, nil
}

// WaitGone 轮询等待进程退出，超时返回 false。
func WaitGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !Running(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}
