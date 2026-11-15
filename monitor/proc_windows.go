//go:build windows

package main

import "golang.org/x/sys/windows"

// stillActive 即 STILL_ACTIVE。进程已退出但 PID 还没被回收时，
// 光看 OpenProcess 能不能打开是不够的，必须再查退出码。
const stillActive = 259

// processRunning 判断 pid 对应的进程是否真的还在跑。
// 不能用 os.Process.Signal(syscall.Signal(0))：Windows 上它恒返回错误，
// 会把活着的进程判成「没在跑」。
func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}
