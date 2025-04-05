package cmd

import (
	"encoding/json"
	"fast-https/config"
	initialization "fast-https/init"
	"fast-https/modules/core/server"
	"fast-https/output"
	"fast-https/utils/logger"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/kardianos/service"
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
	logger.Info("fast https (p *program) Start ...")
	return nil
}

func (p *program) Stop(s service.Service) error {
	logger.Info("fast https (p *program) Stop ...")
	return nil
}

// RootCmd 返回根命令对象，处理命令行参数和标志
func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "fast-https",
		Short:        "short log",
		Long:         "long log",
		SilenceUsage: true, //用于在发生错误时静音使用情况。
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(args)
		},
	}
	return cmd
}

func usage() {
	usageStr := `Unkown command, please check command.`
	logger.Info("%s\n", usageStr)
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
		logger.Warn("%v", err) //print the error if obtained
		return err
	}

	srvConfig.WorkingDirectory = directory

	s, err := service.New(prg, srvConfig)
	if err != nil {
		logger.Warn("%v", err)
		return err
	}

	err = s.Install()
	if err != nil {
		logger.Warn("service install failed: %s", err.Error())
		return err
	} else {
		logger.Info("fast-https service installed at %s", directory)
	}

	return nil
}

// ServiceUnInstallHandler 处理服务卸载
func ServiceUnInstallHandler() error {

	s, err := service.New(prg, srvConfig)
	if err != nil {
		logger.Warn("%v", err)
		return err
	}

	err = s.Uninstall()
	if err != nil {
		logger.Warn("service uninstall failed: %s", err.Error())
		return err
	} else {
		logger.Info("fast-https uninstalled")
	}

	return nil
}

// DevStartHandler 仅用于开发模式的启动处理
func DevStartHandler() error {

	go func() {
		logger.Info("%v", http.ListenAndServe("0.0.0.0:10000", nil))
	}()

	logger.Level(6)

	// pre-check before server start
	PreCheckHandler()

	// output logo, make initialization and start server
	output.PrintLogo()
	if runtime.GOOS == "windows" {
		WritePid(config.PID_FILE)
	}

	output.PrintInitialStart()
	initialization.InitSystem()
	output.PrintInitialEnd()

	server := server.ServerInit()
	server.Run()

	return nil
}

// StartHandler start server
// StartHandler 启动服务器处理函数
// 返回: error 错误信息
func StartHandler() error {
	// pre-check before server start
	PreCheckHandler()

	// output logo, make initialization and start server
	output.PrintLogo()
	if runtime.GOOS == "windows" {
		WritePid(config.PID_FILE)
	}

	output.PrintInitialStart()
	initialization.InitSystem()
	output.PrintInitialEnd()

	if runtime.GOOS != "windows" {
		_ = Daemon(0, 0) // this func will write pid
	}
	server := server.ServerInit()
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
		logger.Fatal("read pid failed")
	}

	// var cmd *exec.Cmd
	// if runtime.GOOS == "windows" {
	// 	cmd = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
	// } else {
	// 	cmd = exec.Command("sudo", "kill", "-9", strconv.Itoa(pid))
	// }
	process, err := os.FindProcess(pid)
	if err != nil {
		logger.Fatal("fast-https find process failed: %v", err)
	}

	err = process.Signal(os.Kill)

	if err != nil {
		logger.Fatal("fast-https stop failed: %v", err)
	}

	os.Remove(config.PID_FILE)
	return nil
}

// ReloadHandler reload server
// ReloadHandler 函数用于重新加载进程。
//
// 返回:
// - error: 如果发生错误，则返回错误信息；否则返回 nil。
func ReloadHandler() error {

	pid, err := readPid(config.PID_FILE)
	if err != nil {
		logger.Fatal("read pid failed")
	}

	// TODO: Windows
	// if runtime.GOOS == "windows" {
	// if err := sendCtrlC(pid); err != nil {
	// 	logger.Debug("gid: %d, send ctrl c sig failed %v", pid, err)
	// }
	// } else {
	// 	cmd := exec.Command("sudo", "kill", strconv.Itoa(pid), "-2")
	// 	err = cmd.Run()
	// 	if err != nil {
	// 		logger.Fatal("fast-https reload failed: %v", err)
	// 	}
	// }
	process, err := os.FindProcess(pid)
	if err != nil {
		logger.Fatal("fast-https find process failed: %v", err)
	}

	err = process.Signal(syscall.SIGINT)

	if err != nil {
		logger.Fatal("fast-https stop failed: %v", err)
	}
	return nil
}

func statusHandler() error {
	return nil
}

// PreCheckHandler 在服务器启动前执行以下检查：
// 1. 检查配置文件是否有效
// 2. 检查端口是否被占用
// 3. 检查是否已有实例在运行
func PreCheckHandler() {
	// check config
	err := config.CheckConfig()
	if err != nil {
		logger.Fatal("Start server failed. An error occurred for the following reason: %v", err)
	}

	// check ports
	err = server.ScanPorts()
	if err != nil {
		logger.Fatal("Port has been used, An error occurred for the following reason: %v", err)
	}

	// 检查 PID 文件
	pid, err := readPid(config.PID_FILE)
	if err != nil {
		if err.Error() == "error reading file" {
			// PID 文件不存在，说明没有运行的实例，可以继续
			logger.Debug("No existing process found")
			return
		}
		// 其他错误
		logger.Fatal("Error checking process: %v", err)
		return
	}

	// 只有在成功读取到 PID 时才检查进程
	process, err := os.FindProcess(pid)
	if err != nil {
		// 进程不存在，可以继续
		logger.Debug("Process %d not found, starting new instance", pid)
		return
	}

	// 检查进程是否真的在运行
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		// 进程正在运行
		logger.Fatal("fast-https is already running")
	}
	// 进程不存在，可以继续
}

// WritePid 将当前进程的PID写入指定文件
// 使用JSON格式存储PID信息，便于后续进程管理
func WritePid(filepath string) error {
	// Get current PID and GID
	pid := os.Getpid()

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
		return 0, fmt.Errorf("error reading file")
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

// 这个功能后面的版本再去做
// watchConfigChanges 监控配置文件变化
// 定期检查配置文件是否发生变化，如有变化则自动重载
func watchConfigChanges() {
	logger.Info("Config file watch started")
	for {
		// TODO: 实现配置文件监控逻辑
		time.Sleep(time.Second * 5)
	}
}
