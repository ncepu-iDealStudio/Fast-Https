//go:build windows

package cmd

import "os"

// daemonize 在 Windows 上不做后台化：托盘 / 服务管理器负责拉起进程，
// 再 exec 一次只会多出一个脱离托管的孤儿进程。
func daemonize() (pid int, isParent bool, err error) {
	return 0, false, nil
}

// signalStopProcess 在 Windows 上没有信号语义，按原有行为直接终止进程。
func signalStopProcess(process *os.Process) error {
	return process.Kill()
}
