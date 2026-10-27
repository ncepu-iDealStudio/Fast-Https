//go:build !windows

package cmd

import (
	"os"
	"os/exec"
	"syscall"
)

// daemonEnv 标记当前进程已经是后台子进程，避免再次 exec 自己。
const daemonEnv = "FASTHTTPS_DAEMON"

// daemonize 用「重新 exec 自己」的方式把服务转到后台。
//
// 不用 fork 的原因：fork 出来的子进程只带着当前这一个线程，父进程此时
// 已经有 Go 运行时的多个线程与锁，子进程继续跑 Go 代码可能直接卡死
// （启动日志停在一半，端口也没监听）；而且 fork 后的父进程无法可靠地
// 得知子进程 pid，写进 pid 文件的是它自己的 pid，一旦退出就成了死 pid。
//
// 返回值：pid 是后台进程的 pid；isParent 为 true 表示当前是父进程，
// pid 文件已经写好，调用方应当退出。
func daemonize() (pid int, isParent bool, err error) {
	if os.Getenv(daemonEnv) == "1" {
		return 0, false, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return 0, false, err
	}

	command := exec.Command(exe, os.Args[1:]...)
	command.Env = append(os.Environ(), daemonEnv+"=1")
	// Setsid 让子进程脱离当前会话：终端关闭不会把服务带走
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	// 标准输出/错误交给 /dev/null：终端关闭后再往已失效的 fd 写日志
	// 会收到 SIGPIPE 直接退出。真正需要的日志在 ./logs 下。
	if devNull, openErr := os.OpenFile(os.DevNull, os.O_RDWR, 0); openErr == nil {
		defer devNull.Close()
		command.Stdin = devNull
		command.Stdout = devNull
		command.Stderr = devNull
	}

	if err := command.Start(); err != nil {
		return 0, false, err
	}
	// Release 之后 Process.Pid 会被置为 -1，先取出来。
	childPid := command.Process.Pid
	_ = command.Process.Release()

	// pid 文件不由父进程写：写早了子进程自己的 PreCheckHandler 会看到
	// 「已经有一个实例在跑」而直接退出，写自己的 pid 又会变成死 pid。
	// 子进程启动后第一件事就是写自己的 pid，父进程在外面等它写好。
	return childPid, true, nil
}

// signalStopProcess 让服务优雅停机：Unix 下服务端收到 SIGTERM 会关闭监听、
// 等在途请求处理完（见 runtime/server 的 GracefulStopTimeout）。
func signalStopProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
