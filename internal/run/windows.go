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

func StartWindows() {
	logFile, _ := os.OpenFile("logs/monitor.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	onExit := func() {
		now := time.Now()
		log.Println(now.String() + " System Exit.")
	}

	systray.Run(onReady, onExit)
}

func startServer() (err error) {
	dir, err := os.Getwd()
	command := exec.Command(filepath.Join(dir, "fast-https"), "start")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
	err = command.Run()
	if err != nil {
		log.Println(err)
	}
	return nil
}

func stopServer() (err error) {
	dir, err := os.Getwd()
	command := exec.Command(filepath.Join(dir, "fast-https"), "stop")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err = command.Run()
	if err != nil {
		log.Println(err)
	}
	return nil
}

func reloadServer() (err error) {
	dir, err := os.Getwd()
	command := exec.Command(filepath.Join(dir, "fast-https"), "reload")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err = command.Run()
	if err != nil {
		log.Println(err)
	}
	return nil
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
	systray.SetTooltip("Lantern")
	mStart := systray.AddMenuItem("Start", "Start server")
	mStop := systray.AddMenuItem("Stop", "Stop server")
	mReload := systray.AddMenuItem("Reload", "Reload server")
	systray.AddSeparator()

	if isRunning {
		systray.SetTemplateIcon(output.LogoExecuting, output.LogoExecuting)
		mStart.Disable()

	} else {
		systray.SetTemplateIcon(output.LogoStopping, output.LogoStopping)
	}
	// We can manipulate the systray in other goroutines
	go func() {

		for {
			select {
			case <-mStart.ClickedCh:
				log.Println("Fast-Https Starting...")
				err := startServer()
				systray.SetTemplateIcon(output.LogoExecuting, output.LogoExecuting)
				mStart.Disable()
				if err != nil {
					log.Println(err)
				}
			case <-mStop.ClickedCh:
				err := stopServer()
				if err != nil {
					log.Println(err)
				} else {
					systray.SetTemplateIcon(output.LogoStopping, output.LogoStopping)
					log.Println("Fast-Https Stop")
					mStart.Enable()
				}
			case <-mReload.ClickedCh:
				err := reloadServer()
				if err != nil {
					log.Println(err)
				} else {
					log.Println("Fast-Https Monitor Reload")
				}
			}
		}
	}()

	mQuitOrig := systray.AddMenuItem("Quit", "Quit the whole app")
	go func() {
		<-mQuitOrig.ClickedCh
		systray.Quit()
		os.Exit(0)
	}()
}
