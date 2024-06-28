package test

import (
	"fast-https/cmd"
	"fast-https/config"
	initialiaztion "fast-https/init"
	"os"
	"path/filepath"
	"testing"
)

/*
服务器初始化测试:
1. 写入进程pid
2. 系统消息初始化
3. 读取配置文件，初始化全局配置
4. 初始化系统日志模块
5. 自签名证书初始化
6. 读取静态文件
7. 启动服务开始监听
*/
func TestServerInit(t *testing.T) {
	currentDir, err := os.Getwd()
	if err != nil {
		t.Error(err)
	}
	currentDir = filepath.Dir(currentDir)
	err = os.Chdir(currentDir)
	if err != nil {
		t.Error(err)
	}
	t.Log("current system path: ", currentDir)

	// 1. write pid into file and command
	cmd.WritePid(config.PID_FILE)
	t.Log("step1: fast-https.pid: ", os.Getpid())

	// 2. init system message
	initialiaztion.MessageInit(config.GConfig.LogRoot)
	t.Log("step2: system message initialization")

	// 3. read config into memory
	config.Init()
	t.Log("step3: read config")

	// 5. self-signed certification initialization
	initialiaztion.CertInit()
	t.Log("step5: init self-signed certification")

	// 6. load cache

	// 7. listen on ports and start server
	t.Log("step6: run server")
}
