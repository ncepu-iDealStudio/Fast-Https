package view

// Windows 进程枚举与强杀（纯 syscall，无第三方依赖）。
// 只服务 StopServer 的兜底路径：stop 找不到 pid 时按 exe 强停。

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

type ProcessInfo struct {
	Pid  uint32
	Path string
}

var (
	modkernel32          = syscall.NewLazyDLL("kernel32.dll")
	procCreateToolhelp32 = modkernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32First   = modkernel32.NewProc("Process32FirstW")
	procProcess32Next    = modkernel32.NewProc("Process32NextW")
	procOpenProcess      = modkernel32.NewProc("OpenProcess")
	procTerminateProcess = modkernel32.NewProc("TerminateProcess")
	procCloseHandle      = modkernel32.NewProc("CloseHandle")
	procQueryFullImage   = modkernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	th32csSnapProcess = 0x2
	processTerminate  = 0x0001
	processQueryInfo  = 0x0400
	maxPath           = 260
)

type processEntry struct {
	Size            uint32
	Usage           uint32
	ProcessID       uint32
	Heaps           uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriClassTEB     int32
	Flags           uint32
	ExeFile         [maxPath]uint16
}

// GetProcessesByName 枚举系统中与 name.exe 匹配的进程（name 不含 .exe 时自动补）。
func GetProcessesByName(name string) []ProcessInfo {
	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name += ".exe"
	}
	var out []ProcessInfo
	snap, _, _ := procCreateToolhelp32.Call(th32csSnapProcess, 0)
	if snap == uintptr(syscall.InvalidHandle) {
		return out
	}
	defer procCloseHandle.Call(snap)
	var entry processEntry
	entry.Size = uint32(unsafe.Sizeof(entry))
	ret, _, _ := procProcess32First.Call(snap, uintptr(unsafe.Pointer(&entry)))
	for ret != 0 {
		exe := syscall.UTF16ToString(entry.ExeFile[:])
		if strings.EqualFold(exe, name) {
			out = append(out, ProcessInfo{Pid: entry.ProcessID, Path: processImagePath(entry.ProcessID)})
		}
		ret, _, _ = procProcess32Next.Call(snap, uintptr(unsafe.Pointer(&entry)))
	}
	return out
}

// processImagePath 读进程的完整映像路径；读不到（权限不足）返回空串。
func processImagePath(pid uint32) string {
	h, _, _ := procOpenProcess.Call(processQueryInfo, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer procCloseHandle.Call(h)
	buf := make([]uint16, maxPath)
	size := uint32(len(buf))
	ret, _, _ := procQueryFullImage.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

// KillProcess 强杀指定进程（TerminateProcess）。
func KillProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	h, _, _ := procOpenProcess.Call(processTerminate, 0, uintptr(pid))
	if h == 0 {
		return fmt.Errorf("open process %d failed", pid)
	}
	defer procCloseHandle.Call(h)
	ret, _, err := procTerminateProcess.Call(h, 1)
	if ret == 0 {
		return fmt.Errorf("terminate process %d failed: %v", pid, err)
	}
	_ = os.ErrProcessDone // 占位引用，保持 import 最小
	return nil
}
