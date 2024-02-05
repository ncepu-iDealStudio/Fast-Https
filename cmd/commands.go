package cmd

import (
	"bufio"
	"fast-https/config"
	initialization "fast-https/init"
	"fast-https/modules/core/server"
	"fast-https/output"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var data []string

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
			name:        "reload",
			description: "Switching Processes",
			handler:     ReloadHandler,
		},
		{
			name:        "start",
			description: "start process",
			handler:     StartHandler,
		},
		{
			name:        "stop",
			description: "Stop process",
			handler:     StopHandler,
		},
		{
			name:        "status",
			description: "Check process status",
			handler:     statusHandler,
		},
	}
)

// Root command parameters are methods
func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          color.HiYellowString("go"),
		Short:        "A command-line tool",
		Long:         color.RedString("This is a help log"),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read terminal input
			data = os.Args
			return runCommand(data)
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
	if len(data) == 1 {
		fmt.Println(color.RedString("Input is missing a parameter"))
		fmt.Println(color.RedString("Please try again"))
		return nil
	}

	// Correct command usage
	var found bool
	for _, c := range commands {
		if data[1] == c.name {
			found = true
			if err := c.handler(); err != nil {
				return err
			}
			break
		}
	}

	// Irregular commands
	if !found {
		fmt.Println(color.YellowString("The input command is not a valid command"))
		fmt.Println(color.YellowString("Please try again"))
		return nil
	}

	return nil
}

// ReloadHandler reload server
func ReloadHandler() error {
	// StopHandler()
	// time.Sleep(time.Second)
	// StartHandler()
	return nil
}

// StopHandler stop server
func StopHandler() error {
	file, err := os.OpenFile(config.PID_FILE, os.O_RDWR|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	readerBuf := bufio.NewReader(file)
	str, _ := readerBuf.ReadString('\n')
	msg := strings.Trim(str, "\r\n")
	ax, _ := strconv.Atoi(msg)
	// ax := 21980

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(ax))
	} else {
		cmd = exec.Command("kill", strconv.Itoa(ax))
	}

	err = cmd.Run()
	if err != nil {
		fmt.Println("Shutdown process failed:", err)
		return nil
	}

	file.Close()

	os.Remove(config.PID_FILE)

	return nil
}

// this start handler only when develop
func DevStartHandler() error {
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:10000", nil))
	}()
	// pre check before server start
	PreCheckHandler()

	// output logo, make initialization and start server
	output.PrintLogo()
	WritePid()

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
	WritePid()

	output.PrintInitialStart()
	initialization.Init()
	output.PrintInitialEnd()

	server := server.ServerInit()
	server.Run()

	// server will clog here
	return nil
}

func PreCheckHandler() {
	// check config
	err := config.CheckConfig()
	if err != nil {
		log.Println("Start server failed. An error occurred for the following reason:")
		log.Fatalln(err)
	}

	// check ports
	err = server.ScanPorts()
	if err != nil {
		log.Println("Port has been used, An error occurred for the following reason:")
		log.Fatalln(err)
	}

	config.ClearConfig()
}

func WritePid() {
	// Obtain the pid and store it
	x_pid := os.Getpid()

	file, err := os.OpenFile(config.PID_FILE, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatal("Open pid file error", err)
	}
	defer file.Close()

	file.WriteString(strconv.Itoa(x_pid) + "\n")
	fmt.Println("Fast-Https running [PID]:", x_pid)
}

func statusHandler() error {
	return nil
}
