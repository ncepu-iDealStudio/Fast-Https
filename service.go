package main

import (
	"fmt"
	"os"
	"time"

	"github.com/kardianos/service"
	"github.com/shirou/gopsutil/process"
)

func main2() {
	srvConfig := &service.Config{
		Name:        "fast-https",
		DisplayName: "Fast-https Web Server",
		Description: "A high preformance web server and proxy server",
	}
	prg := &program{}
	s, err := service.New(prg, srvConfig)
	if err != nil {
		fmt.Println(err)
	}
	if len(os.Args) > 1 {
		serviceAction := os.Args[1]
		switch serviceAction {
		case "install":
			err := s.Install()
			if err != nil {
				fmt.Println("安装服务失败: ", err.Error())
			} else {
				fmt.Println("安装服务成功")
			}
			return
		case "uninstall":
			err := s.Uninstall()
			if err != nil {
				fmt.Println("卸载服务失败: ", err.Error())
			} else {
				fmt.Println("卸载服务成功")
			}
			return
		}
	}

	err = s.Run()
	if err != nil {
		fmt.Println(err)
	}
}

type program struct{}

func (p *program) Start(s service.Service) error {
	fmt.Println(" (p *program) Start 服务运行...")
	go Run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	fmt.Println(" (p *program) Stop 服务停止...")
	return nil
}

func Run() {
	// 具体的服务实现
	t := time.Now()
	fileName := fmt.Sprintf("process-info-%s.txt", t.Format("2006-01-02-15-04-05"))
	file, err := os.Create(fileName)
	//file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		fmt.Println("创建文件失败：", err)
		return
	}
	defer file.Close()

	tick := time.Tick(3 * time.Second)

	for {
		select {
		case <-tick:
			now := t.Format("2006-01-02 15:04:05")
			processes, _ := process.Processes()
			for _, p := range processes {
				name, _ := p.Name()
				pid := p.Pid
				memory, _ := p.MemoryInfo()
				cpuPercent, _ := p.CPUPercent()
				//fmt.Printf("Process name: %s,PID:%d,Memory: Usage:%v,CPU uage:%f\n", name, pid, memory, cpuPercent)
				_, err := file.WriteString(fmt.Sprintf("time：%s,Process name: %s,PID:%d,Memory: Usage:%v,CPU uage:%f\n", now, name, pid, memory, cpuPercent))
				if err != nil {
					fmt.Println("写入信息失败：", err)
					return
				}
			}
			fmt.Printf("%s 写入成功 \n", now)
		}
	}
}
