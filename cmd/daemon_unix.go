//go:build !windows

package cmd

import (
	"fast-https/config"
	"fast-https/infra/logging"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// daemonEnv 标记当前进程已经是后台子进程，避免再次 exec 自己。
	daemonEnv = "FASTHTTPS_DAEMON"
	// daemonReadyEnv 记录父进程留给子进程的就绪通道写端 fd。
	daemonReadyEnv = "FASTHTTPS_DAEMON_READY_FD"
	// daemonLogName 是后台进程控制台输出的落点文件名。
	daemonLogName = "daemon.log"
	// daemonReadyMsg 是子进程报告「已经监听端口」的标记。
	daemonReadyMsg = "ready"
)

// daemonReadyPipe 是父进程侧的就绪通道读端，子进程启动完成或退出时关闭。
var daemonReadyPipe *os.File

// daemonChildReady 是子进程侧的就绪通道写端。
var daemonChildReady *os.File

// daemonize 用「重新 exec 自己」的方式把服务转到后台。
//
// 不用 fork 的原因：fork 出来的子进程只带着当前这一个线程，父进程此时
// 已经有 Go 运行时的多个线程与锁，子进程继续跑 Go 代码可能直接卡死
// （启动日志停在一半，端口也没监听）；而且 fork 后的父进程无法可靠地
// 得知子进程 pid，写进 pid 文件的是它自己的 pid，一旦退出就成了死 pid。
//
// 返回值：pid 是后台进程的 pid；isParent 为 true 表示当前是父进程，
// 调用方应调用 waitDaemonStarted 等子进程报告就绪，再决定是否成功。
func daemonize() (pid int, isParent bool, err error) {
	if os.Getenv(daemonEnv) == "1" {
		// 子进程：接管父进程留下的就绪通道写端。
		if fd, convErr := strconv.Atoi(os.Getenv(daemonReadyEnv)); convErr == nil && fd > 0 {
			daemonChildReady = os.NewFile(uintptr(fd), "daemon-ready")
		}
		// 标准输出已经指向 logs/daemon.log，四路文件日志不必再回显一遍。
		logging.SetConsoleEcho(false)
		return 0, false, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return 0, false, err
	}

	readyR, readyW, err := os.Pipe()
	if err != nil {
		return 0, false, err
	}

	command := exec.Command(exe, os.Args[1:]...)
	command.Env = append(os.Environ(), daemonEnv+"=1", daemonReadyEnv+"=3")
	command.ExtraFiles = []*os.File{readyW}
	// Setsid 让子进程脱离当前会话：终端关闭不会把服务带走
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	// 标准输出/错误不能留在终端（终端关闭后写日志会收到 SIGPIPE 直接退出），
	// 也不能扔进 /dev/null：启动期任何 Fatal 都会随进程一起消失，父进程只能
	// 看到「pid 文件写了、进程没了」。落到 daemon.log，失败原因就能捞出来。
	logFile, logErr := openDaemonLog()
	if logErr != nil {
		logging.ConsoleLog.Warn().Msgf("open %s failed: %v, daemon output is discarded", daemonLogPath(), logErr)
	}
	devNull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if devNull != nil {
		defer devNull.Close()
		command.Stdin = devNull
	}
	if logFile != nil {
		command.Stdout = logFile
		command.Stderr = logFile
	} else if devNull != nil {
		command.Stdout = devNull
		command.Stderr = devNull
	}

	if err := command.Start(); err != nil {
		_ = readyR.Close()
		_ = readyW.Close()
		if logFile != nil {
			_ = logFile.Close()
		}
		return 0, false, err
	}

	// 父进程必须关掉写端：子进程异常退出后读端才能读到 EOF，
	// 否则只能靠超时判定，白白多等几秒。
	_ = readyW.Close()
	if logFile != nil {
		_ = logFile.Close()
	}
	daemonReadyPipe = readyR

	// Release 之后 Process.Pid 会被置为 -1，先取出来。
	childPid := command.Process.Pid
	_ = command.Process.Release()

	// pid 文件不由父进程写：写早了子进程自己的 PreCheckHandler 会看到
	// 「已经有一个实例在跑」而直接退出，写自己的 pid 又会变成死 pid。
	// 子进程启动后第一件事就是写自己的 pid，父进程在就绪通道外面等它。
	return childPid, true, nil
}

// daemonLogPath 返回后台进程控制台输出的路径，与四路文件日志同目录。
// 配置还没加载成功时退回默认日志目录。
func daemonLogPath() string {
	root := strings.TrimSpace(config.Current().LogRoot)
	if root == "" {
		root = config.DEFAULT_LOG_ROOT
	}
	return filepath.Join(root, daemonLogName)
}

// openDaemonLog 打开（并清空）后台进程控制台日志。
// 每次 start 覆盖，避免上一次启动的残留把这次的错误埋掉。
func openDaemonLog() (*os.File, error) {
	path := daemonLogPath()
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
}

// notifyDaemonReady 在监听端口建立之后通知父进程启动成功。
// 子进程在就绪之前退出时，写端随进程关闭，父进程读到 EOF 就知道启动失败，
// 不用再靠「pid 文件写了没有」猜。
func notifyDaemonReady() {
	if daemonChildReady == nil {
		return
	}
	_, _ = daemonChildReady.Write([]byte(daemonReadyMsg + "\n"))
	_ = daemonChildReady.Close()
	daemonChildReady = nil
}

// waitDaemonStarted 等后台子进程报告就绪。
//
// 三种结果：读到 ready 表示端口已监听；读到 EOF 表示子进程在就绪前退出，
// 此时把 daemon.log 的尾部打出来，父进程返回错误而不是假报成功；
// 超时（子进程还活着但很慢）只告警，保持旧行为，不打断正在启动的实例。
func waitDaemonStarted(pid int, timeout time.Duration) error {
	pipe := daemonReadyPipe
	daemonReadyPipe = nil
	if pipe == nil {
		return nil
	}
	defer pipe.Close()

	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 32)
		n, _ := pipe.Read(buf)
		got <- string(buf[:n])
	}()

	select {
	case msg := <-got:
		if strings.HasPrefix(strings.TrimSpace(msg), daemonReadyMsg) {
			return nil
		}
		return fmt.Errorf("fast-https failed to start (pid %d exited during startup), see %s:\n%s",
			pid, daemonLogPath(), tailFile(daemonLogPath(), 20))
	case <-time.After(timeout):
		logging.ConsoleLog.Warn().Msgf("fast-https (pid %d) did not report ready in %s, check %s",
			pid, timeout, daemonLogPath())
		return nil
	}
}

// tailFile 取文件末尾最多 maxLines 行，用于把子进程的启动错误回显给调用者。
func tailFile(path string, maxLines int) string {
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

// signalStopProcess 让服务优雅停机：Unix 下服务端收到 SIGTERM 会关闭监听、
// 等在途请求处理完（见 runtime/server 的 GracefulStopTimeout）。
func signalStopProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
