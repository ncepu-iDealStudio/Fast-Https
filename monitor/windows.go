//go:build windows && amd64

package main

import (
	"bufio"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"fyne.io/systray"
	"golang.org/x/sys/windows"

	"fast-https/config"
)

const (
	// monitorLockName 是单实例锁，放在日志目录里，那个目录一定是可写的。
	monitorLockName = "monitor.lock"

	stateRefreshPeriod = 2 * time.Second
	stopTimeout        = 10 * time.Second

	// 259 即 STILL_ACTIVE，进程已退出但 PID 尚未回收时也能区分出来。
	stillActive = 259
)

var (
	logFile *os.File

	// serverOwned 记录 fast-https 是否由本托盘拉起。只有自己拉起的进程才在退出时回收，
	// 避免误杀由系统服务方式启动的实例。
	serverOwned atomic.Bool

	serverMu sync.Mutex
	stateMu  sync.Mutex
)

// exeDir returns the directory that contains this monitor executable, so the
// main program and the log directory are always found next to it, no matter
// what the current working directory is.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(exe)
}

// resolvePath 把配置里的相对路径固定到程序目录。
// 配置常量写作 "./xxx"，进程工作目录不同会指向不同文件，pid 文件尤其致命。
func resolvePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(exeDir(), path)
}

func pidFilePath() string { return resolvePath(config.PID_FILE) }

// acquireSingleInstance 独占打开锁文件，保证同时只有一个 monitor。
//
// 不用命名互斥体是因为 CreateMutex 把 GetLastError 的交给了调用方，
// 而 Go 的 syscall 封装不保证该值在调用返回后仍然有效，无法可靠区分
// 「新建」与「已存在」。共享模式传 0 的独占打开则是确定性的：
// 第二个进程会拿到 ERROR_SHARING_VIOLATION。
//
// 句柄随进程结束由内核释放，崩溃后残留的锁文件不影响下次启动。
// 返回值 ok 为 false 表示已有实例在跑。
func acquireSingleInstance(path string) (windows.Handle, bool) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, true
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, // 不共享：第二个进程打不开
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return 0, false
		}
		// 目录不可写之类的问题放行，不能因为查重失败导致托盘完全起不来。
		return 0, true
	}
	return handle, true
}

// StartWindows start the taskBox window
func StartWindows() {
	// The log directory lives next to the executable. Create it first, otherwise
	// OpenFile fails (monitor has no console, so the file is the only log sink).
	logDir := resolvePath(config.DEFAULT_LOG_ROOT)
	_ = os.MkdirAll(logDir, 0o755)
	var err error
	logFile, err = os.OpenFile(filepath.Join(logDir, config.MONIITOR_LOG_FILE_PATH), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o666)
	if err != nil {
		logFile = nil
	}
	if logFile != nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	// 单实例：重复点击会拉起多个托盘，各自启停同一个服务，状态互相覆盖。
	handle, ok := acquireSingleInstance(filepath.Join(logDir, monitorLockName))
	if !ok {
		log.Println(time.Now().String() + " Another monitor is already running, exit.")
		return
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	onExit := func() {
		log.Println(time.Now().String() + " Monitor System Exit.")
		shutdownServer()
	}

	systray.Run(onReady, onExit)
}

// shutdownServer 在退出托盘时回收本实例拉起的服务。
func shutdownServer() {
	if !serverOwned.Load() {
		if processAlive(getPid()) {
			log.Println("fast-https is running but was not started by this monitor, left untouched.")
		}
		return
	}

	log.Println("Stopping fast-https before exit...")
	stopServer()

	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		if !processAlive(getPid()) {
			serverOwned.Store(false)
			log.Println("fast-https stopped.")
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	log.Println("Warn: fast-https did not exit within", stopTimeout)
}

// setCmdOutput 让子进程沿用托盘日志；logFile 不可用时交给系统默认处理。
func setCmdOutput(command *exec.Cmd) {
	if logFile == nil {
		return
	}
	command.Stdout = logFile
	command.Stderr = logFile
}

func startServer() {
	serverMu.Lock()
	defer serverMu.Unlock()

	if processAlive(getPid()) {
		return
	}

	command := exec.Command(filepath.Join(exeDir(), "fast-https.exe"))
	command.Dir = exeDir()
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	setCmdOutput(command)
	if err := command.Start(); err != nil {
		log.Println("start fast-https failed:", err)
		return
	}
	serverOwned.Store(true)

	// 回收子进程句柄；服务被 stop 杀掉后这里自然返回。
	go func() {
		_ = command.Wait()
		serverOwned.Store(false)
	}()
}

func stopServer() {
	serverMu.Lock()
	defer serverMu.Unlock()
	runServerCommand("stop")
}

func reloadServer() {
	serverMu.Lock()
	defer serverMu.Unlock()
	runServerCommand("reload")
}

func runServerCommand(verb string) {
	command := exec.Command(filepath.Join(exeDir(), "fast-https.exe"), verb)
	command.Dir = exeDir()
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	setCmdOutput(command)
	if err := command.Run(); err != nil {
		log.Println(verb+" fast-https failed:", err)
	}
}

// processAlive 判断 pid 对应的进程是否真的还活着。
// 只调 OpenProcess 会把「已退出但句柄未回收」的进程也算成运行中，
// 因此再取一次退出码，只有仍在运行才算活。
func processAlive(pidStr string) bool {
	pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
	if err != nil || pid <= 0 {
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

func getPid() string {
	file, err := os.Open(pidFilePath())
	if err != nil {
		return ""
	}
	defer file.Close()
	line, _, err := bufio.NewReader(file).ReadLine()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(line))
}

func onReady() {
	// setup taskBar window
	systray.SetTitle("Fast-Https")
	systray.SetTooltip("Fast-Https")
	mStart := systray.AddMenuItem("Start", "Start server")
	mStop := systray.AddMenuItem("Stop", "Stop server")
	systray.AddSeparator()
	mQuitOrig := systray.AddMenuItem("Quit", "Quit the whole app")

	lastRunning := -1
	// refresh 按真实进程状态刷新图标与菜单。点击后由它统一驱动，
	// 服务自己退出时也能在下一个周期纠正过来。
	refresh := func() {
		stateMu.Lock()
		defer stateMu.Unlock()

		running := 1
		if !processAlive(getPid()) {
			running = 0
			serverOwned.Store(false)
		}
		if running == lastRunning {
			return
		}
		lastRunning = running

		if running == 1 {
			systray.SetTemplateIcon(LogoExecuting, LogoExecuting)
			mStart.Disable()
			mStop.Enable()
			return
		}
		systray.SetTemplateIcon(LogoStopping, LogoStopping)
		mStart.Enable()
		mStop.Disable()
	}

	refresh()

	// done 在退出时关闭，先停掉状态刷新，避免 systray 关闭后还在调它的接口。
	done := make(chan struct{})

	// quit
	go func() {
		<-mQuitOrig.ClickedCh
		close(done)
		systray.Quit()
	}()

	// start stop and reload control
	go func() {
		for {
			select {
			// start
			case <-mStart.ClickedCh:
				log.Println("Fast-Https Starting...")
				startServer()
				refresh()

			// stop
			case <-mStop.ClickedCh:
				log.Println("Fast-Https Stop...")
				stopServer()
				refresh()
			}
		}
	}()

	// 进程可能在托盘之外被停掉或崩溃，定时同步一次，避免菜单一直是旧状态。
	go func() {
		ticker := time.NewTicker(stateRefreshPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}
