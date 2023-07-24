package run

import (
	"bufio"
	"fast-https/output"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

var logFile *os.File

// StartWindows start the taskBox window
func StartWindows() {
	logFile, _ = os.OpenFile("logs/monitor.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
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
	command := exec.Command(filepath.Join(dir, "fast-https"), "start")
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
	cmd := exec.Command("tasklist", "/FI", "\"PID eq "+processId+"\"")
	out, err := cmd.Output()
	if err != nil {
		log.Println("Error while checking process status")
		log.Println(err)
		return false
	}
	if len(out) > 0 {
		return true
	}
	return false
}

func getPid() string {
	file, _ := os.OpenFile("fast-https.pid", os.O_RDONLY, 0666)
	defer file.Close()
	var pid string
	reader := bufio.NewReader(file)
	line, err := reader.ReadString('\n')
	if err == io.EOF {
		pid = line
	}
	return pid
}

func onReady() {
	isRunning := isProcessRunning(getPid())

	systray.SetTitle("Fast-Https")
	systray.SetTooltip("Fast-Https")
	mStart := systray.AddMenuItem("Start", "Start server")
	mStop := systray.AddMenuItem("Stop", "Stop server")
	mReload := systray.AddMenuItem("Reload", "Reload server")
	systray.AddSeparator()
	mQuitOrig := systray.AddMenuItem("Quit", "Quit the whole app")

	if isRunning {
		systray.SetTemplateIcon(output.LogoExecuting, output.LogoExecuting)
		mStart.Disable()

	} else {
		systray.SetTemplateIcon(output.LogoStopping, output.LogoStopping)
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
				mStart.SetIcon(output.IconStart)
				mStart.Uncheck()
				mStart.Show()
				systray.SetTemplateIcon(output.LogoExecuting, output.LogoExecuting)
				startServer()

			// stop
			case <-mStop.ClickedCh:
				if mStop.Checked() {
					mStop.Uncheck()
					mStop.Show()
				}
				mStart.Enable()
				mStart.SetIcon(output.IconStop)
				mStart.Show()
				log.Println("Fast-Https Stop...")
				systray.SetTemplateIcon(output.LogoStopping, output.LogoStopping)
				stopServer()

			// reload
			case <-mReload.ClickedCh:
				if mReload.Checked() {
					mReload.Uncheck()
					mReload.Show()
				}
				log.Println("Fast-Https Monitor Reload")
				reloadServer()
			}
		}

	}()
}
