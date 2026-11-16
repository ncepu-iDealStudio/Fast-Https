//go:build windows

package servicecmd

import (
	"fast-https/infra/logging"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	// daemonEnv 标记当前进程已经是后台子进程，避免再次 exec 自己。
	daemonEnv = "FASTHTTPS_DAEMON"
	// daemonReadyEnv 记录父进程留给子进程的就绪文件路径。
	daemonReadyEnv = "FASTHTTPS_DAEMON_READY_FILE"

	// Windows 进程创建标志：脱离控制台，并单独成组，避免 Ctrl+C 传播。
	daemonDetachedProcess    = 0x00000008
	daemonCreateNewProcGroup = 0x00000200
)

// daemonReadyPath 是父进程等待子进程报告就绪的文件。
var daemonReadyPath string

// daemonChildExited 在子进程退出时关闭，用于区分「启动失败」和「启动很慢」。
var daemonChildExited chan struct{}

// daemonChildReadyFile 是子进程侧的就绪文件路径。
var daemonChildReadyFile string

// Daemonize 用「重新 exec 自己」的方式把服务转到后台。
//
// Windows 没有 fork，也不能像 Unix 那样用 ExtraFiles 传管道，因此用
// DETACHED_PROCESS 拉起一个完全脱离当前控制台的子进程，就绪状态通过
// 临时文件回传：子进程监听端口后写入 ready，父进程轮询到就返回。
func Daemonize() (pid int, isParent bool, err error) {
	if os.Getenv(daemonEnv) == "1" {
		// 子进程：接管父进程留下的就绪文件路径。
		daemonChildReadyFile = os.Getenv(daemonReadyEnv)
		// 标准输出已经指向 logs/daemon.log，四路文件日志不必再回显一遍。
		logging.SetConsoleEcho(false)
		return 0, false, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return 0, false, err
	}

	ready, err := os.CreateTemp("", "fast-https-ready-*")
	if err != nil {
		return 0, false, err
	}
	readyPath := ready.Name()
	_ = ready.Close()
	_ = os.Remove(readyPath)

	// 标准输出/错误不能留在终端：控制台关闭后写日志会失败。
	// 也不能扔进 nul：启动期的 Fatal 会随进程一起消失，父进程只能看到
	// 「pid 文件写了、进程没了」。落到 daemon.log，失败原因就能捞出来。
	logFile, logErr := OpenDaemonLog()
	if logErr != nil {
		logging.ConsoleLog.Warn().Msgf("open %s failed: %v, daemon output is discarded", DaemonLogPath(), logErr)
	}
	devNull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if devNull != nil {
		defer devNull.Close()
	}
	defer func() {
		if logFile != nil {
			_ = logFile.Close()
		}
	}()

	start := func(flags uint32) (*exec.Cmd, error) {
		command := exec.Command(exe, os.Args[1:]...)
		command.Env = append(os.Environ(), daemonEnv+"=1", daemonReadyEnv+"="+readyPath)
		command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
		if devNull != nil {
			command.Stdin = devNull
		}
		if logFile != nil {
			command.Stdout = logFile
			command.Stderr = logFile
		}
		return command, command.Start()
	}

	command, err := start(daemonDetachedProcess | daemonCreateNewProcGroup)
	if err != nil {
		return 0, false, err
	}

	daemonReadyPath = readyPath
	exited := make(chan struct{})
	daemonChildExited = exited
	child := command.Process
	// 父进程在等待就绪期间需要知道子进程是否已经退出；不等就绪就退出的
	// 情况只有一种——启动失败，此时把 daemon.log 的尾部打出来。
	go func() {
		_, _ = child.Wait()
		close(exited)
	}()

	// pid 文件不由父进程写：写早了子进程自己的 PreCheckHandler 会看到
	// 「已经有一个实例在跑」而直接退出，写自己的 pid 又会变成死 pid。
	// 子进程启动后第一件事就是写自己的 pid，父进程在就绪文件外面等它。
	return child.Pid, true, nil
}

// NotifyDaemonReady 在监听端口建立之后通知父进程启动成功。
func NotifyDaemonReady() {
	if daemonChildReadyFile == "" {
		return
	}
	if err := os.WriteFile(daemonChildReadyFile, []byte(daemonReadyMsg+"\n"), 0o644); err != nil {
		logging.ConsoleLog.Warn().Msgf("write daemon ready file %s failed: %v", daemonChildReadyFile, err)
	}
	daemonChildReadyFile = ""
}

// WaitDaemonStarted 等后台子进程报告就绪。
//
// 三种结果：读到 ready 表示端口已监听；子进程先退出表示启动失败，把
// daemon.log 的尾部打出来；超时（子进程还活着但很慢）只告警，保持旧行为。
func WaitDaemonStarted(pid int, timeout time.Duration) error {
	path := daemonReadyPath
	daemonReadyPath = ""
	exited := daemonChildExited
	daemonChildExited = nil
	if path == "" {
		return nil
	}
	defer os.Remove(path)

	isReady := func() bool {
		data, err := os.ReadFile(path)
		return err == nil && len(data) > 0
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if isReady() {
				return nil
			}
		case <-exited:
			if isReady() {
				return nil
			}
			return fmt.Errorf("fast-https failed to start (pid %d exited during startup), see %s:\n%s",
				pid, DaemonLogPath(), TailFile(DaemonLogPath(), 20))
		case <-deadline.C:
			logging.ConsoleLog.Warn().Msgf("fast-https (pid %d) did not report ready in %s, check %s",
				pid, timeout, DaemonLogPath())
			return nil
		}
	}
}

// SignalStopProcess 让服务优雅停机：Windows 没有信号语义，按原有行为直接终止进程。
func SignalStopProcess(process *os.Process) error {
	return process.Kill()
}
