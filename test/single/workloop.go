package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 创建一个通道用于接收操作系统的信号
	sigChan := make(chan os.Signal, 1)

	// 指定要监听的信号，这里监听SIGINT
	signal.Notify(sigChan, syscall.SIGINT)

	// 启动一个 goroutine 等待信号到来
	go func() {
		// 等待信号
		sig := <-sigChan
		fmt.Printf("接收到信号 %v\n", sig)

		// 在接收到信号后执行需要的处理逻辑，这里简单打印一条消息
		fmt.Println("收到 SIGINT 信号，程序即将退出...")

		// 模拟清理或处理工作
		time.Sleep(time.Second)

		// 退出程序
		os.Exit(0)
	}()

	// 主程序继续执行其他逻辑
	fmt.Println("程序正在运行，请按 Ctrl+C 发送 SIGINT 信号")

	// 保持程序运行状态
	select {}
}
