package cmd

import (
	"bufio"
	"fast-https/modules/core/server"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var string_1 string
var string_2 string
var string_3 string
var string_4 string
var count int = 0

var rootcmd = &cobra.Command{
	Use:   color.HiYellowString("go"),
	Short: "this is a short command",
	Long:  color.RedString("this is a helping log"),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		data := os.Args
		comma := data[1]
		if comma == "start" {
			string_2 = "m"
		} else if comma == "reoad" {
			string_1 = "m"
		} else if comma == "stop" {
			string_3 = "m"
		} else if comma == "status" {
			string_4 = "m"
		}
	},
	Run: Startfunc,
}

func Execute() {
	rootcmd.Execute()
}

func init() {
	// -h help帮助文档
	rootcmd.PersistentFlags().String("reload", "", color.BlueString("Switching Processes"))
	rootcmd.PersistentFlags().String("start", "", color.BlueString("Start process"))
	rootcmd.PersistentFlags().String("stop", "", color.BlueString("Sop process"))
	rootcmd.PersistentFlags().String("status", "", color.BlueString("进行读写判断"))
}

func Startfunc(cmd *cobra.Command, args []string) {
	for {
		Choose()
		switch count {
		case 1:
			Reoad_func()
			return
		case 2:
			Start()
			return
		case 3:
			Kill()
			return
		case 4:
			return
		}
	}
}

func Reoad_func() {
	Kill()
	Start_test()
}

func Kill() {
	file, err := os.OpenFile("fasthttps.pid", os.O_RDWR|os.O_APPEND, 0666)
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
			return
		}

		fmt.Println("Process closed")
		file.Close()

		ioutil.WriteFile("fasthttps.pid", []byte{}, 0666)
	} else {
		fmt.Println("End operation")
	}
}

func Start() {
	Start_test()
	return
}

func Start_test() {
	x_pid := os.Getpid()

	file, _ := os.OpenFile("fasthttps.pid", os.O_WRONLY|os.O_APPEND, 0666)

	defer file.Close()
	writer1 := bufio.NewWriter(file)
	writer1.WriteString(strconv.Itoa(x_pid))
	writer1.WriteString("\n")
	writer1.Flush()
	fmt.Println(color.RedString("Fast-Https running [PID]:"), x_pid)
	// for {
	// 	y_pid := color.BlueString(strconv.Itoa(x_pid))
	// 	fmt.Println(y_pid)
	// 	time.Sleep(2 * time.Second)
	// }
	// server.Daemon(0, 1)
	server.Run()
}

func Choose() {
	if string_1 == "m" {
		count = 1
	} else if string_2 == "m" {
		count = 2
	} else if string_3 == "m" {
		count = 3
	} else if string_4 == "m" {
		count = 4
	}
}

func Hot_Reoad_func() {
	for {
		file, err := os.OpenFile("fasthttps.pid", os.O_RDWR|os.O_APPEND, 0666)
		defer file.Close()

		if err != nil {
			fmt.Println("File search failed")
			continue
		}

		reader1 := bufio.NewReader(file)
		writer1 := bufio.NewWriter(file)

		str_1, err := reader1.ReadString('\n')

		if err != nil {
			fmt.Println("File read failure")
			fmt.Println(err)
			break
		}

		msg := strings.Trim(str_1, "\r\n")
		fmt.Println(msg)

		if msg == "" {
			count := 0
			for {
				writer1.WriteString("reoad\n")
				time.Sleep(1 * time.Second)
				count++
				if count >= 5 {
					break
				}
			}
			writer1.Flush()
		}

		if msg == "reoad" {
			fmt.Println("Output End")
			break
		} else {
			fmt.Println("There is already data available")
			break
		}
	}
}
