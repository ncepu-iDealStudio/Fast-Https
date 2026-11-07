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
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

// Command Structure Definition
// 命令结构体定义
type command struct {
	name        string                 // 命令名称
	description string                 // 命令描述（已按当前语言翻译）
	handler     func() error           // 命令处理函数
	register    func() *cobra.Command  // 需要子命令或 flags 时使用，与 handler 二选一
}

// commandList 每次建树时按当前语言生成描述。
// 不能做成包级变量：初始化发生在 -h 之前，那时还不知道要哪种语言。
func commandList() []command {
	return []command{
		{
			name:        "start",
			description: t("启动 Web 服务器", "to start web server"),
			handler:     StartHandler,
		},
		{
			name:        "stop",
			description: t("停止 Web 服务器", "to stop web server"),
			handler:     StopHandler,
		},
		{
			name:        "reload",
			description: t("重新加载配置", "to reload config"),
			handler:     ReloadHandler,
		},
		{
			name:        "status",
			description: t("查看 Web 服务器运行状态", "to check web server status"),
			handler:     statusHandler,
		},
		{
			name:        "version",
			description: t("打印版本号", "print the version"),
			register:    newVersionCmd,
		},
		{
			name:        "acme",
			description: t("配置 ACME 证书签发", "configure ACME certificate issuance"),
			register:    newAcmeCmd,
		},
		{
			name:        "config",
			description: t("查看解析后的运行时路径", "inspect resolved runtime paths"),
			register:    newConfigCmd,
		},
		{
			name:        "site",
			description: t("管理 http.include 目录下的站点文件", "manage site files under http.include"),
			register:    newSiteCmd,
		},
	}
}

// newVersionCmd 打印版本号。版本是编译期常量，与启动横幅、错误页脚、
// ACME 的 User-Agent 用的是同一个值，不另外维护一份。
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "version",
		Short:        t("打印版本号", "print the version"),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "fast-https %s\n", config.CURRENT_VERSION)
			return nil
		},
	}
}

// RootCmd 返回根命令对象。语言先按命令行与环境变量定下来，再建整棵树：
// cobra 的 Short/Long 是字符串字段，建树时给哪份就是哪份。
func RootCmd() *cobra.Command {
	return RootCmdForLang(resolveLang(os.Args[1:]))
}

// RootCmdForLang 按指定语言建树，供 --lang 之外的入口（测试、man 生成）使用。
func RootCmdForLang(lang Lang) *cobra.Command {
	if lang != LangEN {
		lang = LangZH
	}
	currentLang = lang

	cmd := &cobra.Command{
		Use:   "fast-https",
		Short: t("高性能 Web 服务器与反向代理服务器", "Fast-https is a high performance web server and proxy server"),
		Long: t(
			"Fast-https 是高性能 Web 服务器与反向代理服务器。\n\n不带子命令直接运行即启动服务。",
			"Fast-https is a high performance web server and proxy server.\n\n"+
				"Run it without any command to start the server."),
		SilenceUsage: true, //用于在发生错误时静音使用情况。
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(args)
		},
	}
	// 语言在解析参数之前就要定下来，所以这个 flag 只作为显式入口记录在帮助里：
	// 真正的取值发生在 RootCmd() 里扫 os.Args。默认值留空，避免 pflag 在
	// 中文帮助里补一句英文的 (default "zh")。
	cmd.PersistentFlags().String("lang", "",
		t("帮助信息的语言：zh | en（默认取 FAST_HTTPS_LANG，未设置时为 zh）",
			"language of the help output: zh | en (defaults to "+LangEnv+", zh when unset)"))

	// 把命令注册为 cobra 子命令，使 -h/--help 能列出全部命令及说明
	for _, sub := range commandList() {
		if sub.register != nil {
			// 带子命令或 flags 的命令自己构造完整命令树（acme、site、config）。
			cmd.AddCommand(sub.register())
			continue
		}
		handler := sub.handler
		cmd.AddCommand(&cobra.Command{
			Use:          sub.name,
			Short:        sub.description,
			SilenceUsage: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return handler()
			},
		})
	}

	// 不要 cobra 自带的 `completion` 子命令：它生成的是 shell 补全脚本，
	// 与本项目无关（站点路径的 Tab 补全在 site init 的输入行里，是另一套实现）。
	// 留着只会在 --help 里多一条看起来像是我们做的事。
	cmd.CompletionOptions.DisableDefaultCmd = true

	// help 平时由 cobra 在 Execute 时懒加载，这里提前建出来，
	// 两种语言的命令树才是同一棵（man 页不会缺项）；英文版沿用 cobra 的默认文案。
	cmd.InitDefaultHelpCmd()

	localizeCobra(cmd)
	return cmd
}

func usage() {
	usageStr := t("未知命令，请检查命令拼写。", "Unknown command, please check the command.")
	logging.ConsoleLog.Info().Msgf("%s\n", usageStr)
}

// runCommand 执行具体的命令处理函数
func runCommand(args []string) error {
	// 如果没有参数，也能启动服务器
	if len(args) == 0 {
		StartHandler()
		return nil
	}

	for _, c := range commandList() {
		if args[0] != c.name {
			continue
		}
		if c.register != nil {
			// 子命令组由 cobra 解析，这里只处理传进来的剩余参数。
			sub := c.register()
			sub.SetArgs(args[1:])
			return sub.Execute()
		}
		if err := c.handler(); err != nil {
			return err
		}
		return nil
	}

	// 没有匹配到任何命令，输出帮助信息
	usage()
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
			// 等子进程报告「端口已监听」再返回。只等 pid 文件是不够的：
			// 子进程写完 pid 后绑端口失败会立刻退出，父进程却报启动成功，
			// 调用方只看到一句 stale pid，真正的原因无处可查。
			if err := waitDaemonStarted(pid, daemonStartTimeout); err != nil {
				return err
			}
			logging.ConsoleLog.Info().Msgf("fast-https started in background, pid %d, logs in %s",
				pid, config.Current().LogRoot)
			return nil
		}
	}

	// 由真正提供服务的进程写自己的 pid，写完再做初始化。
	// 写失败不能静默：pid 是 stop / reload / status 的唯一入口，写不出来
	// 会表现为「服务在跑但停不掉」，必须留下能查的日志（例如安装目录
	// 不可写的情况）。
	if err := WritePid(config.PID_FILE); err != nil {
		logging.ConsoleLog.Error().Msgf(
			"write pid file %s failed: %v; stop / reload / status will not find this process",
			config.PID_FILE, err)
	}

	// output logo, make initialization and start server
	banner.PrintLogo(config.CURRENT_VERSION)

	banner.PrintInitialStart()
	initialization.InitSystem()
	banner.PrintInitialEnd()

	RegisterHandlers()
	RegisterServices()
	server := server.ServerInit()
	// 监听已经建立（端口全部绑定成功）：通知父进程启动成功。
	// 前台运行或 Windows 上没有父进程等待，这里是空操作。
	notifyDaemonReady()
	banner.PrintPortsListenerStart()
	server.Run()

	// server will clog here
	return nil
}

// daemonStartTimeout 是父进程等后台子进程报告就绪的上限。
// 首次启动要生成 CA 与自签证书（RSA 3072），给足时间但别让命令卡死。
const daemonStartTimeout = 10 * time.Second

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
	return reload(config.CONFIG_FILE_PATH, config.PID_FILE)
}

// reload 先校验配置再发信号。
//
// 校验放在客户端做有两点好处：配置写坏时不去惊动正在服务的进程，以及把原因
// 直接说到终端上——否则只有 daemon.log 里一行「reload kept previous config」，
// 用户看到的是 reload 成功退出却什么都没变。
func reload(configPath, pidPath string) error {
	if err := validateConfig(configPath); err != nil {
		return fmt.Errorf("配置未通过校验，已保留运行中的配置：%w", err)
	}

	pid, err := readPid(pidPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logging.ConsoleLog.Info().Msg("fast-https is not running, nothing to reload (config ok)")
		} else {
			logging.ConsoleLog.Warn().Msgf("read pid file failed: %v", err)
		}
		return nil
	}

	// 残留 pid 文件（进程已退出）时提示「没在跑」并清理，不要对着死 pid 发信号。
	if !processRunning(pid) {
		_ = os.Remove(pidPath)
		logging.ConsoleLog.Info().Msgf("fast-https is not running, nothing to reload (stale pid %d, config ok)", pid)
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

// validateConfig 用主配置同一套 schema 校验一个配置文件及其 include，
// 不监听端口、不写 pid，也不打印任何东西。
func validateConfig(path string) error {
	return config.ValidateConfigFile(path)
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
	if err := initialization.CertInit(); err != nil {
		logging.ConsoleLog.Fatal().Msgf("Start server failed. An error occurred for the following reason: %v (run as root, or start it with systemctl start fast-https)", err)
	}

	// check config
	err := config.CheckConfig()
	if err != nil {
		logging.ConsoleLog.Fatal().Msgf("Start server failed. An error occurred for the following reason: %v", err)
	}

	// 先判已有实例，再查端口：自己已经在跑的时候端口当然被占，
	// 先查端口会报成「port 443 is already in use」，让人以为被别的程序占了。
	checkRunningInstance()

	// check ports
	if err := server.ScanPorts(); err != nil {
		logging.ConsoleLog.Fatal().Msgf("Start server failed. An error occurred for the following reason: %v", err)
	}
}

// checkRunningInstance 读 pid 文件，发现同名实例仍在运行就退出。
// 残留 pid（进程已退出）只记一条 debug，不阻断启动。
func checkRunningInstance() {
	pid, err := readPid(config.PID_FILE)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logging.ConsoleLog.Debug().Msg("No existing process found")
			return
		}
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
// 父目录缺失时先创建：包布局的 pid 在 /run/fast-https 下，/run 是 tmpfs，
// 重启后目录不存在，靠 systemd 的 RuntimeDirectory 或这里兜住手工启动。
func writePidTo(path string, pid int) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("error creating pid directory %s: %v", dir, err)
		}
	}
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
	if err := os.WriteFile(path, jsonData, 0644); err != nil {
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
