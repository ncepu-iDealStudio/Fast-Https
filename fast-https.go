// Command fast-https 启动 HTTP/HTTPS Web 服务器与反向代理。
package main

import (
	"fast-https/cmd"
	"os"
)

// main 是程序的入口函数。
//
// 控制台日志级别由 cmd 在子命令中按运行模式调整（dev 全量、其它为 info）。
func main() {
	command := cmd.RootCmd()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
