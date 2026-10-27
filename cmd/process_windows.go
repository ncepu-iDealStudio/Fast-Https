//go:build windows

package cmd

import "golang.org/x/sys/windows"

// 259 即 STILL_ACTIVE，进程已退出但 PID 尚未回收时也能区分出来。
const stillActive = 259

// processRunning 判断 pid 对应的进程是否真的还在跑。
// 只调 OpenProcess 是不够的：进程退出后只要还有别的进程持有它的句柄
// （例如托盘拉起的子进程），PID 就仍然可以被 OpenProcess 打开，
// 于是 stop 会以为服务还在、白等到超时。这里再查一次退出码。
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
