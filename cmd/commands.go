// Package cmd 实现 fast-https 命令行接口：服务控制、配置检查与服务器启动。
package cmd

import (
	"encoding/json"
	"errors"
	"fast-https/config"
	"fast-https/infra/banner"
	"fast-https/infra/logging"
	initialization "fast-https/init"
	"fast-https/runtime/server"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/kardianos/service"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// Command Structure Definition
// 命令结构体定义
type command struct {
	name        string       // 命令名称
	description string       // 命令描述
	handler     func() error // 命令处理函数
}

var (
	// Command structure initialization
	// 命令结构体初始化
	commands = []command{
		{
			name:        "install",
			description: "to install fast-https service", // 安装fast-https服务
			handler:     ServiceInstallHandler,
		},
		{
			name:        "uninstall",
			description: "to uninstall fast-https service", // 卸载fast-https服务
			handler:     ServiceUnInstallHandler,
		},
		{
			name:        "start",
			description: "to start web server", // 启动Web服务器
			handler:     StartHandler,
		},
		{
			name:        "dev",
			description: "to start web server in dev mode", // 以debug模式启动Web服务器
			handler:     DevStartHandler,
		},
		{
			name:        "stop",
			description: "to Stop web server", // 停止Web服务器
			handler:     StopHandler,
		},
		{
			name:        "reload",
			description: "to reload config", // 重新加载配置
			handler:     ReloadHandler,
		},
		{
			name:        "status",
			description: "to check web server status", // 检查Web服务器状态
			handler:     statusHandler,
		},
		{
			name:        "check",
			description: "to validate config without listening",
			handler:     checkHandler,
		},
	}

	srvConfig = &service.Config{
		Name:        "fast-https",
		DisplayName: "Fast-https Web Server",
		Description: "A high preformance web server and proxy server",
	}

	prg = &program{}
)

type program struct{}

func (p *program) Start(s service.Service) error {
	logging.ConsoleLog.Info().Msg("fast https (p *program) Start ...")
	return nil
}

func (p *program) Stop(s service.Service) error {
	logging.ConsoleLog.Info().Msg("fast https (p *program) Stop ...")
	return nil
}

// RootCmd 返回根命令对象，处理命令行参数和标志
func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fast-https",
		Short: "Fast-https is a high performance web server and proxy server",
		Long: "Fast-https is a high performance web server and proxy server.\n\n" +
			"Run it without any command to start the server.",
		SilenceUsage: true, //用于在发生错误时静音使用情况。
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(args)
		},
	}

	// 把 commands 注册为 cobra 子命令，使 -h/--help 能列出全部命令及说明
	for i := range commands {
		sub := commands[i]
		cmd.AddCommand(&cobra.Command{
			Use:          sub.name,
			Short:        sub.description,
			SilenceUsage: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return sub.handler()
			},
		})
	}

	return cmd
}

func usage() {
	usageStr := `Unkown command, please check command.`
	logging.ConsoleLog.Info().Msgf("%s\n", usageStr)
}

// runCommand 执行具体的命令处理函数
func runCommand(args []string) error {
	// 如果没有参数，也能启动服务器
	if len(args) == 0 {
		StartHandler()
		return nil
	}

	for _, c := range commands {
		if args[0] != c.name {
			continue
		}
		if err := c.handler(); err != nil {
			return err
		} else {
			return nil
		}
	}

	// 没有匹配到任何命令，输出帮助信息
	usage()
	return nil
}

// ServiceInstallHandler 处理服务安装
// 获取当前工作目录并安装系统服务
func ServiceInstallHandler() error {

	directory, err := os.Getwd() //get the current directory using the built-in function
	if err != nil {
		logging.ConsoleLog.Warn().Msgf("%v", err) //print the error if obtained
		return err
	}

	srvConfig.WorkingDirectory = directory

	s, err := service.New(prg, srvConfig)
	if err != nil {
		logging.ConsoleLog.Warn().Msgf("%v", err)
		return err
	}

	err = s.Install()
	if err != nil {
		logging.ConsoleLog.Warn().Msgf("service install failed: %s", err.Error())
		return err
	} else {
		logging.ConsoleLog.Info().Msgf("fast-https service installed at %s", directory)
	}

	return nil
}

// ServiceUnInstallHandler 处理服务卸载
func ServiceUnInstallHandler() error {

	s, err := service.New(prg, srvConfig)
	if err != nil {
		logging.ConsoleLog.Warn().Msgf("%v", err)
		return err
	}

	err = s.Uninstall()
	if err != nil {
		logging.ConsoleLog.Warn().Msgf("service uninstall failed: %s", err.Error())
		return err
	} else {
		logging.ConsoleLog.Info().Msg("fast-https uninstalled")
	}

	return nil
}

// DevStartHandler 仅用于开发模式的启动处理
func DevStartHandler() error {

	go func() {
		logging.ConsoleLog.Info().Msgf("%v", http.ListenAndServe("0.0.0.0:10000", nil))
	}()

	logging.SetLevel(zerolog.TraceLevel)

	// pre-check before server start
	PreCheckHandler()

	// output logo, make initialization and start server
	banner.PrintLogo(config.CURRENT_VERSION)
	// dev stays in the foreground on every platform, so the pid file is this process.
	// reload reads that file and sends SIGINT.
	WritePid(config.PID_FILE)

	banner.PrintInitialStart()
	initialization.InitSystem()
	banner.PrintInitialEnd()

	RegisterHandlers()
	RegisterServices()
	// dev 前台模式：SIGINT（Ctrl+C）优雅停机，而不是重载配置
	server.SetForegroundDev(true)
	server := server.ServerInit()
	banner.PrintPortsListenerStart()
	server.Run()

	return nil
}

// StartHandler start server
// StartHandler 启动服务器处理函数
// 返回: error 错误信息
func StartHandler() error {
	// pre-check before server start
	PreCheckHandler()

	// 后台化：重新 exec 自己，而不是 fork 之后继续跑 Go 代码。
	// fork 出来的子进程继承了父进程的多线程与运行时锁状态，可能直接卡在
	// 启动阶段；而且 fork 出来的父进程只能写下自己的 pid，一退出就变成
	// 死 pid，stop / reload / status 都找不到真正在服务的进程。
	// 容器与 systemd 托管时保持前台（见 runInForeground）。
	if runtime.GOOS != "windows" && !runInForeground() {
		pid, isParent, err := daemonize()
		if err != nil {
			logging.ConsoleLog.Warn().Msgf("run in background failed: %v, staying in foreground", err)
		} else if isParent {
			// 等子进程写好 pid 文件再返回，避免「命令刚返回，status 就说没在跑」
			if waitPidFile(pid, 3*time.Second) {
				logging.ConsoleLog.Info().Msgf("fast-https started in background, pid %d, logs in ./logs", pid)
			} else {
				logging.ConsoleLog.Warn().Msgf("fast-https (pid %d) did not take over in time, check ./logs", pid)
			}
			return nil
		}
	}

	// 由真正提供服务的进程写自己的 pid，写完再做初始化。
	_ = WritePid(config.PID_FILE)

	// output logo, make initialization and start server
	banner.PrintLogo(config.CURRENT_VERSION)

	banner.PrintInitialStart()
	initialization.InitSystem()
	banner.PrintInitialEnd()

	RegisterHandlers()
	RegisterServices()
	server := server.ServerInit()
	banner.PrintPortsListenerStart()
	server.Run()

	// server will clog here
	return nil
}

// StopHandler stop server
// StopHandler 函数用于停止运行中的进程。
//
// 参数：
// - 无
//
// 返回值：
// - error：返回可能发生的错误
func StopHandler() error {

	pid, err := readPid(config.PID_FILE)
	if err != nil {
		// 没有 pid 文件（已经停过）或文件损坏：按「没在跑」处理，stop 幂等
		if errors.Is(err, os.ErrNotExist) {
			logging.ConsoleLog.Info().Msg("fast-https is not running")
		} else {
			logging.ConsoleLog.Warn().Msgf("read pid file failed: %v", err)
		}
		return nil
	}

	// pid 文件可能是上一次异常退出留下的：进程已经不在时按「本来就没在跑」
	// 处理，顺手清掉残留文件，别让 stop 报一个看不懂的错。
	if !processRunning(pid) {
		_ = os.Remove(config.PID_FILE)
		logging.ConsoleLog.Info().Msgf("fast-https is not running, removed stale pid file (pid %d)", pid)
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		logging.ConsoleLog.Fatal().Msgf("fast-https find process failed: %v", err)
	}

	// Unix 下发 SIGTERM 走优雅停机（关闭监听、等在途请求）；
	// Windows 没有信号语义，落回 TerminateProcess。
	if err := signalStopProcess(process); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			_ = os.Remove(config.PID_FILE)
			logging.ConsoleLog.Info().Msgf("fast-https is not running (pid %d)", pid)
			return nil
		}
		logging.ConsoleLog.Fatal().Msgf("fast-https stop failed: %v", err)
	}

	if !waitProcessGone(pid, server.GracefulStopTimeout+2*time.Second) {
		logging.ConsoleLog.Fatal().Msgf("fast-https (pid %d) did not exit in time", pid)
	}

	_ = os.Remove(config.PID_FILE)
	logging.ConsoleLog.Info().Msg("fast-https stopped")
	return nil
}

// waitPidFile 等后台进程把自己的 pid 写进文件，超时返回 false。
func waitPidFile(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if current, err := readPid(config.PID_FILE); err == nil && current == pid {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitProcessGone 轮询等待进程退出。
func waitProcessGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !processRunning(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// ReloadHandler reload server
// ReloadHandler 函数用于重新加载进程。
//
// 返回:
// - error: 如果发生错误，则返回错误信息；否则返回 nil。
func ReloadHandler() error {

	pid, err := readPid(config.PID_FILE)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logging.ConsoleLog.Info().Msg("fast-https is not running, nothing to reload")
		} else {
			logging.ConsoleLog.Warn().Msgf("read pid file failed: %v", err)
		}
		return nil
	}

	// 残留 pid 文件（进程已退出）时提示「没在跑」并清理，不要对着死 pid 发信号。
	if !processRunning(pid) {
		_ = os.Remove(config.PID_FILE)
		logging.ConsoleLog.Info().Msgf("fast-https is not running, nothing to reload (stale pid %d)", pid)
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		logging.ConsoleLog.Fatal().Msgf("fast-https find process failed: %v", err)
	}

	err = signalReloadProcess(process, pid)

	if err != nil {
		logging.ConsoleLog.Fatal().Msgf("fast-https reload failed: %v", err)
	}
	return nil
}

// CheckFile validates one config file and its includes. It does not listen or write a pid file.
func CheckFile(path string) error {
	if err := config.ValidateConfigFile(path); err != nil {
		return err
	}
	fmt.Println("config ok")
	return nil
}

func checkHandler() error {
	return CheckFile(config.CONFIG_FILE_PATH)
}

func statusHandler() error {
	state, pid, err := InspectProcess(config.PID_FILE)
	if err != nil {
		return err
	}
	switch state {
	case ProcessRunning:
		fmt.Printf("fast-https is running, pid %d\n", pid)
		return nil
	case ProcessStale:
		return fmt.Errorf("fast-https is not running, stale pid %d", pid)
	default:
		return fmt.Errorf("fast-https is not running, no pid file")
	}
}

// ProcessState is the result of reading the pid file and checking that process.
type ProcessState int

const (
	ProcessAbsent ProcessState = iota
	ProcessStale
	ProcessRunning
)

// InspectProcess reports whether pidFile names a live process.
func InspectProcess(pidFile string) (ProcessState, int, error) {
	pid, err := readPid(pidFile)
	if err != nil {
		// 用 errors.Is 判断，别再比对错误字符串（readPid 换文案就失效）
		if errors.Is(err, os.ErrNotExist) {
			return ProcessAbsent, 0, nil
		}
		return ProcessAbsent, 0, err
	}
	if !processRunning(pid) {
		return ProcessStale, pid, nil
	}
	return ProcessRunning, pid, nil
}

// PreCheckHandler 在服务器启动前执行以下检查：
// 1. 检查配置文件是否有效
// 2. 检查端口是否被占用
// 3. 检查是否已有实例在运行
func PreCheckHandler() {
	// Load config and create missing self-signed certs before the file check.
	// config/cert/*.pem is not in the repository; CertInit writes them on first start.
	if err := config.Init(); err != nil {
		logging.ConsoleLog.Fatal().Msgf("Start server failed. An error occurred for the following reason: %v", err)
	}
	initialization.CertInit()

	// check config
	err := config.CheckConfig()
	if err != nil {
		logging.ConsoleLog.Fatal().Msgf("Start server failed. An error occurred for the following reason: %v", err)
	}

	// check ports
	err = server.ScanPorts()
	if err != nil {
		logging.ConsoleLog.Fatal().Msgf("Port has been used, An error occurred for the following reason: %v", err)
	}

	// 检查 PID 文件
	pid, err := readPid(config.PID_FILE)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// PID 文件不存在，说明没有运行的实例，可以继续
			logging.ConsoleLog.Debug().Msg("No existing process found")
			return
		}
		// 其他错误
		logging.ConsoleLog.Fatal().Msgf("Error checking process: %v", err)
		return
	}

	// 只有在成功读取到 PID 时才检查进程。
	// 存活判定必须走平台实现：Windows 上 os.Process.Signal(syscall.Signal(0))
	// 恒返回 EWINDOWS，会把正在跑的实例判成「没在跑」，重复启动就漏过去了。
	if processRunning(pid) {
		logging.ConsoleLog.Fatal().Msg("fast-https is already running")
	}
	// 进程不存在，可以继续
	logging.ConsoleLog.Debug().Msgf("Process %d not found, starting new instance", pid)
}

// runInForeground keeps the process as PID 1. Containers are detected from
// their well-known marker files. FASTHTTPS_FOREGROUND=1 forces the same mode.
func runInForeground() bool {
	if os.Getenv("FASTHTTPS_FOREGROUND") == "1" {
		return true
	}
	// systemd 由 unit 托管进程：再转后台会让 unit 认为服务已经退出。
	// INVOCATION_ID 只由 systemd 注入，手工执行时不会有。
	if os.Getenv("INVOCATION_ID") != "" {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	return false
}

// WritePid 将当前进程的PID写入指定文件
// 使用JSON格式存储PID信息，便于后续进程管理
func WritePid(filepath string) error {
	return writePidTo(filepath, os.Getpid())
}

// writePidTo 把指定进程的 pid 写进文件。后台化时由父进程写子进程的 pid，
// 这样 stop / reload / status 才能找到真正在服务的进程。
func writePidTo(filepath string, pid int) error {
	// Create a map to hold PID and GID
	pidGidMap := map[string]int{
		"pid": pid,
	}

	// Marshal the map to JSON
	jsonData, err := json.Marshal(pidGidMap)
	if err != nil {
		return fmt.Errorf("error marshalling PID and GID to JSON: %v", err)
	}

	// Write JSON data to the specified file
	if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
		return fmt.Errorf("error writing JSON data to file: %v", err)
	}

	return nil
}

// readPid 从指定文件读取并解析PID信息
// 返回进程ID和可能的错误信息
func readPid(filepath string) (int, error) {
	// Read the file contents
	data, err := os.ReadFile(filepath)
	if err != nil {
		// 带上原始错误，调用方才能区分「文件不存在」和「读不动」
		return 0, fmt.Errorf("error reading file: %w", err)
	}

	// Create a map to hold the PID and GID
	pidGidMap := make(map[string]int)

	// Unmarshal the JSON data into the map
	if err := json.Unmarshal(data, &pidGidMap); err != nil {
		return 0, fmt.Errorf("error unmarshalling JSON data: %v", err)
	}

	// Get the PID and GID from the map
	pid, pidExists := pidGidMap["pid"]

	if !pidExists {
		return 0, fmt.Errorf("PID or GID not found in JSON data")
	}

	return pid, nil
}
