package cmd

import "fast-https/cmd/internal/cli"

// Run 执行根命令并返回进程退出码，供 main 使用。
func Run() int { return cli.Run(RootCmd) }
