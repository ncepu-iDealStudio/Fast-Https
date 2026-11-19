//go:build windows

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"fast-https/cmd/internal/procctl"
)

// restartService 停止并重新启动服务进程，让新配置生效（Windows 专用）。
//
// Windows 无法定向投递 reload 信号，重启是唯一可靠的应用方式。通过
// 子进程调用自身的 stop / start 子命令实现——不直接调父包的 Handler
// （会循环依赖），也复用了已有的停止/启动语义。
//
// 本文件被 tests/arch 的 os/exec 例外名单覆盖：这里只重执行自身，
// 参数是写死的子命令，不接受用户输入，不经过 Shell。
func restartService(pidPath string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("取程序路径失败：%w", err)
	}
	dir := filepath.Dir(pidPath)

	// 1. stop：等它真正退出，避免端口占用导致新进程起不来
	stopCmd := exec.Command(self, "stop")
	stopCmd.Dir = dir
	_ = stopCmd.Run() // stop 在服务未运行时返回非零，这里不视为失败
	for i := 0; i < 30; i++ {
		if st, _, err := procctl.InspectProcess(pidPath); err != nil || st != procctl.ProcessRunning {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}

	// 2. start：派生后台进程，不等待它退出
	startCmd := exec.Command(self, "start")
	startCmd.Dir = dir
	if err := startCmd.Start(); err != nil {
		return fmt.Errorf("启动服务失败：%w", err)
	}
	go func() { _ = startCmd.Wait() }()

	// 3. 确认新进程在跑（最多等 6 秒）
	for i := 0; i < 20; i++ {
		time.Sleep(300 * time.Millisecond)
		if st, _, err := procctl.InspectProcess(pidPath); err == nil && st == procctl.ProcessRunning {
			return nil
		}
	}
	return fmt.Errorf("服务启动超时")
}
