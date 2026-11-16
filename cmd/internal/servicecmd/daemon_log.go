package servicecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fast-https/config"
)

// daemonLogName 是后台进程控制台输出的落点文件名。
const daemonLogName = "daemon.log"

// daemonReadyMsg 是子进程报告「已经监听端口」的标记。
const daemonReadyMsg = "ready"

// DaemonLogPath 返回后台进程控制台输出的路径，与四路文件日志同目录。
// 配置还没加载成功时退回默认日志目录。
func DaemonLogPath() string {
	root := strings.TrimSpace(config.Current().LogRoot)
	if root == "" {
		root = config.DEFAULT_LOG_ROOT
	}
	return filepath.Join(root, daemonLogName)
}

// OpenDaemonLog 打开（并清空）后台进程控制台日志。
// 每次 start 覆盖，避免上一次启动的残留把这次的错误埋掉。
func OpenDaemonLog() (*os.File, error) {
	path := DaemonLogPath()
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
}

// TailFile 取文件末尾最多 maxLines 行，用于把子进程的启动错误回显给调用者。
func TailFile(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(read %s failed: %v)", path, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}
