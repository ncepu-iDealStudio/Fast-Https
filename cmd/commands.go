package cmd

import (
	"bufio"
	"fast-https/config"
	initialization "fast-https/init"
	"fast-https/modules/core/server"
	"fast-https/output"
	"fast-https/utils/logger"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

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

// need return
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
	// pre check before server start
	PreCheckHandler()

	// output logo, make initialization and start server
	output.PrintLogo()
	WritePid(os.Getpid())

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
	// pre check before server start
	PreCheckHandler()

	// output logo, make initialization and start server
	output.PrintLogo()
	if runtime.GOOS == "windows" {
		WritePid(os.Getpid())
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
	file, err := os.OpenFile(config.PID_FILE, os.O_RDWR|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	defer file.Close()
	readerBuf := bufio.NewReader(file)
	str, _ := readerBuf.ReadString('\n')
	msg := strings.Trim(str, "\r\n")
	ax, _ := strconv.Atoi(msg)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(ax))
	} else {
		cmd = exec.Command("sudo", "kill", strconv.Itoa(ax), "-9")
	}

	err = cmd.Run()
	if err != nil {
		logger.Fatal("fast-https stop failed: %v", err)
	}

	os.Remove(config.PID_FILE)
	return nil
}

// ReloadHandler reload server
func ReloadHandler() error {
	// StopHandler()
	// time.Sleep(time.Second)
	// StartHandler()
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

	//TODO: check if fast-https is already running...
	//if failed, logger.Fatal...

}

func WritePid(x_pid int) {
	// Obtain the pid and store it

	file, err := os.OpenFile(config.PID_FILE, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		logger.Fatal("Open pid file error %v", err)
	}
	defer file.Close()

	file.WriteString(strconv.Itoa(x_pid) + "\n")
	logger.Info("Fast-Https running [PID]: %v", x_pid)
}
