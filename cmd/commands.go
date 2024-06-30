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

	"github.com/fatih/color"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

// Command Structure Definition
type command struct {
	name        string
	description string
	handler     func() error
}

var (
	// Command structure initialization
	commands = []command{
		{
			name:        "install",
			description: "to install fast-https service",
			handler:     ServiceInstallHandler,
		},
		{
			name:        "uninstall",
			description: "to uninstall fast-https service",
			handler:     ServiceUnInstallHandler,
		},
		{
			name:        "start",
			description: "to start web server",
			handler:     StartHandler,
		},
		{
			name:        "stop",
			description: "to Stop web server",
			handler:     StopHandler,
		},
		{
			name:        "reload",
			description: "to reload config",
			handler:     ReloadHandler,
		},
		{
			name:        "status",
			description: "to check web server status",
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

// Root command parameters are methods
func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "fast-https",
		Short:        "short log",
		Long:         "long log",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read terminal input

			return runCommand(args)
		},
	}

	// Read terminal input
	for _, c := range commands {
		cmd.PersistentFlags().String(c.name, "", color.BlueString(c.description))
	}

	return cmd
}

// Read terminal input
func runCommand(args []string) error {
	// missing parameter
	if len(args) == 0 {
		logger.Info(color.RedString("This is Dev start mod ..."))
		DevStartHandler()
		return nil
	}

	for _, c := range commands {
		if args[0] == c.name {
			if err := c.handler(); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

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
		logger.Warn("安装服务失败: %s", err.Error())
		return err
	} else {
		logger.Info("fast-https服务在 %s 安装成功!", directory)
	}

	return nil
}

func ServiceUnInstallHandler() error {

	s, err := service.New(prg, srvConfig)
	if err != nil {
		logger.Warn("%v", err)
		return err
	}

	err = s.Uninstall()
	if err != nil {
		logger.Warn("卸载服务失败: %s", err.Error())
		return err
	} else {
		logger.Info("fast-https卸载服务成功")
	}

	return nil
}

// this start handler only when develop
func DevStartHandler() error {
	go func() {
		logger.Info("%v", http.ListenAndServe("0.0.0.0:10000", nil))
	}()
	// pre-check before server start
	PreCheckHandler()

	// output logo, make initialization and start server
	output.PrintLogo()
	WritePid(config.PID_FILE)

	output.PrintInitialStart()
	initialization.Init()
	output.PrintInitialEnd()

	server := server.ServerInit()
	server.Run()

	// server will clog here
	return nil
}

// StartHandler start server
func StartHandler() error {
	// pre-check before server start
	PreCheckHandler()

	// output logo, make initialization and start server
	output.PrintLogo()
	if runtime.GOOS == "windows" {
		WritePid(config.PID_FILE)
	}

	output.PrintInitialStart()
	initialization.Init()
	output.PrintInitialEnd()

	if runtime.GOOS != "windows" {
		Daemon(0, 0) // this func will write pid
	}
	server := server.ServerInit()
	server.Run()

	// server will clog here
	return nil
}

// StopHandler stop server
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

	//if failed, logger.Fatal...
	pid, err := readPid(config.PID_FILE)
	if err != nil && err.Error() == "error reading file" {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		logger.Fatal("fast-https find process failed: %v", err)
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		logger.Fatal("fast-https is already running")
	}
}

// WritePid writes the current PID to a given file in JSON format
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

// readPid read pid reads the PID and GID from a given file in JSON format and returns them.
func readPid(filepath string) (int, error) {
	// Read the file contents
	data, err := os.ReadFile(filepath)
	if err != nil {
		return 0, fmt.Errorf("error reading file: %v", err)
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
