//go:build windows && amd64

// Command monitor 是 Windows 系统托盘助手，用于启动、停止与重载 fast-https。
package main

// start taskBox in windows platform
func main() {
	StartWindows()
}
