// Command fast-https 启动 HTTP/HTTPS Web 服务器与反向代理。
package main

import (
	"fast-https/cmd"
	"os"
)

// main 是程序的入口函数。
//
// 控制台日志级别由 infra/logging 决定，默认 info。
// 退出码由 cmd.Run 按错误码映射，旧命令的一般错误仍是 1。
func main() {
	if code := cmd.Run(); code != 0 {
		os.Exit(code)
	}
}
