//go:build windows && amd64

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

	// 259 即 STILL_ACTIVE，进程已退出但 PID 尚未回收时也能区分出来。
	stillActive = 259
)

// 退出时等待 stop 生效、以及兜底 kill 生效的时长。做成变量便于测试缩短等待。
// commandTimeout 是 stop/reload 子命令自身的执行上限，命令卡住时也要能退出来。
var (
	commandTimeout = 5 * time.Second
	stopTimeout    = 3 * time.Second
	killTimeout    = 3 * time.Second
)

var (
	logFile *os.File

	// child 是本托盘拉起的 fast-https 进程；nil 表示没有。
	// 只认这个句柄，不看 pid 文件：子进程写 pid 文件有延迟，
	// 拿 pid 文件判断归属会在启动瞬间误判成「不是我启的」。
	childMu sync.Mutex
	child   *os.Process

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

// pidFilePath 返回 pid 文件路径。做成变量是为了让测试把它重定向到临时目录，
// 不必往真实的安装目录里写 pid 文件。
var pidFilePath = func() string { return resolvePath(config.PID_FILE) }

// acquireSingleInstance 独占打开锁文件，保证同时只有一个 monitor。
//
// 不用命名互斥体是因为 CreateMutex 把 GetLastError 交给调用方，
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
	// 与菜单里的 Start/Stop 串行：退出清理期间若又拉起一个新进程，
	// 那个进程就成了没人管的孤儿。命令本身有超时，等锁不会无限挂住。
	serverMu.Lock()
	defer serverMu.Unlock()

	proc := ownedChild()
	if proc == nil {
		if processAlive(getPid()) {
			log.Println("fast-https is running but was not started by this monitor, left untouched.")
		}
		return
	}
	if !pidAlive(proc.Pid) {
		log.Println("fast-https already exited.")
		return
	}

	log.Printf("Stopping fast-https (pid %d) before exit...\n", proc.Pid)
	if err := runServerCommand("stop"); err != nil {
		// stop 都没跑起来（主程序缺失、pid 文件损坏等），不用等，直接杀。
		log.Println("stop command failed, terminating the child process directly.")
		killChild(proc)
		return
	}
	if waitPidGone(proc.Pid, stopTimeout) {
		log.Println("fast-https stopped.")
		return
	}

	// pid 文件缺失或 stop 没生效时兜底，直接终止自己拉起的子进程，别留孤儿。
	log.Println("Warn: stop did not take effect, terminating the child process directly.")
	killChild(proc)
}

func killChild(proc *os.Process) {
	if err := proc.Kill(); err != nil {
		log.Println("kill fast-https failed:", err)
	}
	if !waitPidGone(proc.Pid, killTimeout) {
		log.Printf("Warn: fast-https (pid %d) is still alive.\n", proc.Pid)
	}
}

func ownedChild() *os.Process {
	childMu.Lock()
	defer childMu.Unlock()
	return child
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

	// 子进程写 pid 文件有延迟，只看 pid 文件会在启动瞬间允许重复启动。
	if serverRunning() {
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

	proc := command.Process
	childMu.Lock()
	child = proc
	childMu.Unlock()

	// 回收子进程句柄；服务被 stop 杀掉后这里自然返回。
	go func() {
		_ = command.Wait()
		childMu.Lock()
		if child == proc {
			child = nil
		}
		childMu.Unlock()
	}()
}

func stopServer() {
	serverMu.Lock()
	defer serverMu.Unlock()

	// 先记下自己拉起的子进程：stop 可能停的是 pid 文件里的另一个实例，
	// 那样本实例的子进程还在跑，界面就会一直显示「运行中」。
	proc := ownedChild()
	err := runServerCommand("stop")

	if proc == nil || !pidAlive(proc.Pid) {
		return
	}
	if err == nil {
		if waitPidGone(proc.Pid, stopTimeout) {
			return
		}
		log.Println("Warn: stop did not take effect, terminating the child process directly.")
	} else {
		log.Println("stop command failed, terminating the child process directly.")
	}
	killChild(proc)
}

func reloadServer() {
	serverMu.Lock()
	defer serverMu.Unlock()
	runServerCommand("reload")
}

func runServerCommand(verb string) error {
	return runWithTimeout(filepath.Join(exeDir(), "fast-https.exe"), []string{verb}, commandTimeout)
}

// runWithTimeout 执行子命令并在超时后强制终止它。
//
// stopTimeout 只在 stop 命令返回之后才开始计时，所以命令本身卡住（等锁、
// 被杀软拦住、日志盘满）会让托盘退出无限期挂起。这里给命令本身也加一道
// 超时，超时即视为失败，由调用方走兜底流程。
func runWithTimeout(name string, args []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	command := exec.CommandContext(ctx, name, args...)
	command.Dir = exeDir()
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	setCmdOutput(command)

	err := command.Run()
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		log.Printf("%s timed out after %v, killed.\n", name, timeout)
		return err
	}
	log.Println(name, "failed:", err)
	return err
}

// serverRunning 汇总两条线索：pid 文件指向的进程，以及本托盘拉起的子进程。
func serverRunning() bool {
	if processAlive(getPid()) {
		return true
	}
	if proc := ownedChild(); proc != nil && pidAlive(proc.Pid) {
		return true
	}
	return false
}

// pidAlive 判断 pid 对应的进程是否真的还活着。
// 只调 OpenProcess 会把「已退出但句柄未回收」的进程也算成运行中，
// 因此再取一次退出码，只有仍在运行才算活。
func pidAlive(pid int) bool {
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

func processAlive(pidStr string) bool {
	pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
	if err != nil {
		return false
	}
	return pidAlive(pid)
}

func waitPidGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// pidFileRecord 与 cmd.WritePid 写出的 pid 文件结构一致。
type pidFileRecord struct {
	Pid int `json:"pid"`
}

// getPid 返回 pid 文件里记录的进程号，读不到时返回空串。
//
// pid 文件由主程序写成 JSON（{"pid":1234}），早期版本写的是纯数字，
// 两种格式都要认——之前按纯数字解析，JSON 内容永远解析失败，
// 托盘因此从不认 pid 文件里跑着的服务。
func getPid() string {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return ""
	}

	var record pidFileRecord
	if err := json.Unmarshal([]byte(text), &record); err == nil && record.Pid > 0 {
		return strconv.Itoa(record.Pid)
	}
	if _, err := strconv.Atoi(text); err == nil {
		return text
	}
	return ""
}

func onReady() {
	// setup taskBar window
	systray.SetTitle("Fast-Https")
	systray.SetTooltip("Fast-Https")
	mStart := systray.AddMenuItem("Start", "Start server")
	mStop := systray.AddMenuItem("Stop", "Stop server")
	systray.AddSeparator()
	mQuitOrig := systray.AddMenuItem("Quit", "Quit the whole app")

	// done 在退出时关闭，先停掉状态刷新，避免 systray 关闭后还在调它的接口。
	done := make(chan struct{})

	lastRunning := -1
	// refresh 只负责刷新界面，不参与归属判定：启动瞬间 pid 文件还没写出来，
	// 在这里根据 pid 文件改归属会把刚拉起的进程判成「不是我启的」。
	refresh := func() {
		// 退出流程开始后 systray 正在销毁，此时再调它的接口没有意义，
		// 还可能撞上已经关闭的通道。
		select {
		case <-done:
			return
		default:
		}

		stateMu.Lock()
		defer stateMu.Unlock()

		running := 0
		if serverRunning() {
			running = 1
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

			// 退出后菜单不再可用，别把这个 goroutine 挂在通道上。
			case <-done:
				return
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
