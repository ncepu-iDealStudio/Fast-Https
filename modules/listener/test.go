package main

import (
	"fmt"
	"os"
	"time"
)

func main() {

	fmt.Println("当前进程ID：", os.Getpid())

	procAttr := &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}
	process, err := os.StartProcess("/bin/echo", []string{"", "hello,world!"}, procAttr)
	if err != nil {
		fmt.Println("进程启动失败:", err)
		os.Exit(2)
	} else {
		time.Sleep(time.Second * 100)
		fmt.Println("子进程ID：", process.Pid)

	}

	time.Sleep(time.Second * 100)

}
