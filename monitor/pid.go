package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// PidState 把「PID 文件在不在」与「进程还在不在」合成一个判断，
// 用来区分从未启动、已崩溃和启动中。
type PidState int

const (
	PidUnknown PidState = iota
	PidAbsent           // PID 文件不存在：从未启动
	PidStale            // PID 文件在，进程已退出：崩溃或没清理
	PidAlive            // 进程还在：启动中，或运行中但 /runtime 不响应
)

func (s PidState) String() string {
	switch s {
	case PidAbsent:
		return "absent"
	case PidStale:
		return "stale"
	case PidAlive:
		return "alive"
	}
	return "unknown"
}

// ReadPidFile 读 {"pid":N} 形式的 PID 文件，格式与组合根的 WritePid 一致。
// 文件不存在不算错误：它正是「从未启动」的证据。
func ReadPidFile(path string) (int, PidState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, PidAbsent, nil
		}
		return 0, PidUnknown, fmt.Errorf("read pid file %s: %w", path, err)
	}
	var body struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return 0, PidUnknown, fmt.Errorf("parse pid file %s: %w", path, err)
	}
	if body.PID <= 0 || !processRunning(body.PID) {
		return body.PID, PidStale, nil
	}
	return body.PID, PidAlive, nil
}
