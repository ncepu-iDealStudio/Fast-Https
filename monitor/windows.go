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

	"fyne.io/systray"
)

var logFile *os.File

// exeDir returns the directory that contains this monitor executable, so the
// main program and the log directory are always found next to it, no matter
// what the current working directory is.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(exe)
}

// StartWindows start the taskBox window
func StartWindows() {
	// The log directory lives next to the executable. Create it first, otherwise
	// OpenFile fails (monitor has no console, so the file is the only log sink).
	logDir := filepath.Join(exeDir(), config.DEFAULT_LOG_ROOT)
	_ = os.MkdirAll(logDir, 0o755)
	var err error
	logFile, err = os.OpenFile(filepath.Join(logDir, config.MONIITOR_LOG_FILE_PATH), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		logFile = nil
	}
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
	command := exec.Command(filepath.Join(exeDir(), "fast-https.exe"))
	command.Dir = exeDir()
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		log.Println("start fast-https failed:", err)
	}
}

func stopServer() {
	command := exec.Command(filepath.Join(exeDir(), "fast-https.exe"), "stop")
	command.Dir = exeDir()
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Run(); err != nil {
		log.Println("stop fast-https failed:", err)
	}
}

func reloadServer() {
	command := exec.Command(filepath.Join(exeDir(), "fast-https.exe"), "reload")
	command.Dir = exeDir()
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Run(); err != nil {
		log.Println("reload fast-https failed:", err)
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
