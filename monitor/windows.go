//go:build windows && amd64

package main

import (
	"bufio"
	"fast-https/config"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

var logFile *os.File

// StartWindows start the taskBox window
func StartWindows() {
	logFile, _ = os.OpenFile(filepath.Join(config.DEFAULT_LOG_ROOT, config.MONIITOR_LOG_FILE_PATH), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	log.SetOutput(logFile)
	defer logFile.Close()

	onExit := func() {
		now := time.Now()
		log.Println(now.String() + " Monitor System Exit.")
		os.Exit(0)
	}

	systray.Run(onReady, onExit)
}

func startServer() {
	dir, err := os.Getwd()
	command := exec.Command(filepath.Join(dir, "fast-https"))
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Stdout = logFile
	err = command.Start()
	if err != nil {
		log.Println(err)
	}
}

func stopServer() {
	dir, err := os.Getwd()
	command := exec.Command(filepath.Join(dir, "fast-https"), "stop")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Stdout = logFile
	err = command.Run()
	if err != nil {
		log.Println(err)
	}
}

func reloadServer() {
	dir, err := os.Getwd()
	command := exec.Command(filepath.Join(dir, "fast-https"), "reload")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Stdout = logFile
	err = command.Run()
	if err != nil {
		log.Println(err)
	}
}

func isProcessRunning(processId string) bool {
	if len(processId) == 0 {
		return false
	}
	atoi, _ := strconv.Atoi(processId)
	_, err := os.FindProcess(atoi)
	if err != nil {
		return false
	} else {
		return true
	}
}

func getPid() string {
	file, err := os.OpenFile(config.PID_FILE, os.O_RDONLY, 0666)
	if err != nil {
		return ""
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	pid, _, err := reader.ReadLine()
	if err != nil {
		return ""
	}
	return string(pid)
}

func onReady() {
	// setup taskBar window
	systray.SetTitle("Fast-Https")
	systray.SetTooltip("Fast-Https")
	mStart := systray.AddMenuItem("Start", "Start server")
	mStop := systray.AddMenuItem("Stop", "Stop server")
	systray.AddSeparator()
	mQuitOrig := systray.AddMenuItem("Quit", "Quit the whole app")

	// check the fast-https server status
	pid := getPid()
	log.Println("pid:", pid)
	isRunning := isProcessRunning(pid)
	if isRunning {
		systray.SetTemplateIcon(LogoExecuting, LogoExecuting)
		mStart.Disable()
		mStart.SetIcon(IconStart)
		mStart.Uncheck()
		mStart.Show()
	} else {
		systray.SetTemplateIcon(LogoStopping, LogoStopping)
	}

	// quit
	go func() {
		<-mQuitOrig.ClickedCh
		systray.Quit()
	}()

	// start stop and reload control
	go func() {
		for {
			select {
			// start
			case <-mStart.ClickedCh:
				log.Println("Fast-Https Starting...")
				if mStart.Checked() {
					mStart.Uncheck()
				}

				mStart.Disable()
				mStart.SetIcon(IconStart)
				mStart.Uncheck()
				mStart.Show()
				systray.SetTemplateIcon(LogoExecuting, LogoExecuting)
				startServer()

			// stop
			case <-mStop.ClickedCh:
				if mStop.Checked() {
					mStop.Uncheck()
					mStop.Show()
				}
				mStart.Enable()
				mStart.SetIcon(IconStop)
				mStart.Show()
				log.Println("Fast-Https Stop...")
				systray.SetTemplateIcon(LogoStopping, LogoStopping)
				stopServer()
			}
		}
	}()
}
