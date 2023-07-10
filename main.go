package main

import (
	"fast-https/cmd"
	initialization "fast-https/init"
	"fast-https/utils/message"
)

func main() {
	waitGroup := initialization.Init()

	// 执行cmd
	cmd.Execute()

	// 退出系统
	waitGroup.Wait()
	message.Exit()
}
