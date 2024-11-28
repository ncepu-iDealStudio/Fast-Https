package main

import (
	"fast-https/cmd"
	"fast-https/utils/logger"
)

func init() {
	logger.Level(4)
}

// main 是程序的入口函数
//
// 该函数首先通过调用 cmd.RootCmd() 获取根命令对象
// 然后调用该对象的 Execute 方法执行命令
func main() {
	rootcmd := cmd.RootCmd()
	rootcmd.Execute()
}
