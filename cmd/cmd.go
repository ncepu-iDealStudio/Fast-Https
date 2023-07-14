package cmd

import (
	"bufio"
	"fast-https/modules/core/server"
	"fmt"
	"os"

	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var string_1 string
var string_2 string
var string_3 string
var string_4 string
var count int = 0

var rootcmd = &cobra.Command{
	Use:   "go",
	Short: "this is a short command",
	Long:  "this is a helping log",
	Run:   Startfunc,
}

func init() {
	// -h help帮助文档
	rootcmd.Flags().StringVarP(&string_1, "reoad", "d", "", "进行读写判断")
	rootcmd.Flags().StringVarP(&string_2, "start", "t", "", "进行读写判断")
	rootcmd.Flags().StringVarP(&string_3, "stop", "p", "", "进行读写判断")
	rootcmd.Flags().StringVarP(&string_4, "status", "s", "", "进行读写判断")
}

func Execute() {
	rootcmd.Execute()
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
			return
		case 4:
			return
		}
	}
}

func Reoad_func() {
	Kill()
	Start()
}

func Kill() {
	file, _ := os.OpenFile("/home/pzc/Project/fast-https/fasthttps.pid", os.O_RDWR|os.O_APPEND, 0666)
	defer file.Close()
	reader1 := bufio.NewReader(file)
	str, _ := reader1.ReadString('\n')
	mtg := strings.Trim(str, "\r\n")
	ax, _ := strconv.Atoi(mtg)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(ax))
	} else {
		cmd = exec.Command("kill", "-9", strconv.Itoa(ax))
	}

	fmt.Println(ax)
	err := cmd.Run()
	if err != nil {
		fmt.Println("关闭进程失败:", err)
		return
	}

	fmt.Println("进程已关闭")
}

func Start() {
	Start_test()
}

func Start_test() {
	x_pid := os.Getpid()

	file, _ := os.OpenFile("/home/pzc/Project/fast-https/fasthttps.pid", os.O_RDWR|os.O_APPEND, 0666)
	defer file.Close()
	writer1 := bufio.NewWriter(file)
	writer1.WriteString(strconv.Itoa(x_pid))
	writer1.Flush()

	server.Run()
}

func Choose() {
	if string_1 != "" {

		count = 1
	} else if string_2 != "" {
		count = 2
	} else if string_3 != "" {
		count = 3
	} else if string_4 != "" {
		count = 4
	}
}

func Hot_Reoad_func() {
	for {
		file, err := os.OpenFile("C:/Users/Lenovo/Desktop/P/a.txt", os.O_RDWR|os.O_APPEND, 0666)

		defer file.Close()

		if err != nil {
			fmt.Println("文件寻找失败")
			continue
		}

		reader1 := bufio.NewReader(file)
		writer1 := bufio.NewWriter(file)

		str_1, err := reader1.ReadString('\n')

		if err != nil {
			fmt.Println("文件读取失败")
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
			fmt.Println("输出结束")
			break
		} else {
			fmt.Println("已经有数据")
			break
		}
	}
}
