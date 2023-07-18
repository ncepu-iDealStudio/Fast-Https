package cmd

import (
	"bufio"
	initialization "fast-https/init"
	"fast-https/modules/core/server"
	"fmt"
	"io/ioutil"
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
	reloadFlag string
	startFlag  string
	stopFlag   string
	statusFlag string
	commands   = []command{
		{
			name:        "reload",
			description: "Switching Processes",
			handler:     reloadHandler,
		},
		{
			name:        "start",
			description: "start process",
			handler:     startHandler,
		},
		{
			name:        "stop",
			description: "Stop process",
			handler:     stopHandler,
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
		fmt.Println(color.RedString("Please re-enter"))
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
		fmt.Println(color.YellowString("Please re-enter"))
		return nil
	}

	return nil
}

// reloadHandler reload server
func reloadHandler() error {
	stopHandler()
	startHandler()
	return nil
}

// stopHandler stop server
func stopHandler() error {
	file, err := os.OpenFile("fast-https.pid", os.O_RDWR|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf(color.BlueString("output error"))
	} else {
		str_1 := color.RedString("There is a process running, do you need to continue the operation (y/n):")
		fmt.Println(str_1)
	}
	var scan byte
	fmt.Scanf("%c", &scan)
	if scan == 'y' {

		reader1 := bufio.NewReader(file)
		str, _ := reader1.ReadString('\n')
		msg := strings.Trim(str, "\r\n")
		ax, _ := strconv.Atoi(msg)
		// ax := 21980

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(ax))
		} else {
			cmd = exec.Command("kill", "-9", strconv.Itoa(ax))
		}

		err = cmd.Run()
		if err != nil {
			fmt.Println("Shutdown process failed:", err)
			return nil
		}

		fmt.Println("Process closed")
		file.Close()

		ioutil.WriteFile("fast-https.pid", []byte{}, 0666)
	} else {
		fmt.Println("End operation")
	}
	return nil
}

// startHandler start server
func startHandler() error {
	Write_fast_https_pid()
	initialization.Init()
	server.Run()
	return nil
}

func Write_fast_https_pid() {
	// Obtain the pid and store it
	x_pid := os.Getpid()

	file, _ := os.OpenFile("fast-https.pid", os.O_WRONLY|os.O_APPEND, 0666)

	defer file.Close()
	writer1 := bufio.NewWriter(file)
	writer1.WriteString(strconv.Itoa(x_pid))
	writer1.WriteString("\n")
	writer1.Flush()
	fmt.Println(color.RedString("Fast-Https running [PID]:"), x_pid)
}

func statusHandler() error {
	return nil
}
