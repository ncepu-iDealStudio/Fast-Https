//go:build !windows

package cmd

import (
	"os"

	"fast-https/cmd/internal/servicecmd"
)

// 后台化日志相关的辅助只在非 Windows 上存在（Windows 前台运行，不写 daemon 日志）。

func daemonLogPath() string { return servicecmd.DaemonLogPath() }

func openDaemonLog() (*os.File, error) { return servicecmd.OpenDaemonLog() }

func tailFile(path string, maxLines int) string { return servicecmd.TailFile(path, maxLines) }
