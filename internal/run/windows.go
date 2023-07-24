package run

import (
	"fast-https/output"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

func StartWindows() {
	onExit := func() {
		now := time.Now()
		ioutil.WriteFile("logs/monitor.log", []byte(now.String()+" System Exit.\n"), 0644)
	}

	systray.Run(onReady, onExit)
}

func startServer() (err error) {
	dir, err := os.Getwd()
	command := exec.Command(filepath.Join(dir, "fast-https"), "start")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err = command.Run()
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func stopServer() (err error) {
	dir, err := os.Getwd()
	command := exec.Command(filepath.Join(dir, "fast-https"), "stop")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err = command.Run()
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func reloadServer() (err error) {
	dir, err := os.Getwd()
	command := exec.Command(filepath.Join(dir, "fast-https"), "reload")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err = command.Run()
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func onReady() {
	systray.SetTemplateIcon(output.Logo, output.Logo)
	systray.SetTitle("Fast Https")
	systray.SetTooltip("Lantern")
	systray.SetTemplateIcon(output.Logo, output.Logo)
	systray.SetTitle("Fast Https")
	systray.SetTooltip("Fast Https")
	mStart := systray.AddMenuItem("Start", "Start server")
	mStop := systray.AddMenuItem("Stop", "Stop server")
	mReload := systray.AddMenuItem("Reload", "Reload server")
	systray.AddSeparator()
	// We can manipulate the systray in other goroutines
	go func() {

		for {
			select {
			case <-mStart.ClickedCh:
				log.Println("Fast-Https Starting...")
				err := startServer()
				mStart.Disable()
				if err != nil {
					log.Fatal(err)
				}
			case <-mStop.ClickedCh:
				err := stopServer()
				if err != nil {
					log.Fatal(err)
				} else {
					log.Println("Fast-Https Stop")
					mStart.Enable()
				}
			case <-mReload.ClickedCh:
				err := reloadServer()
				if err != nil {
					log.Fatal(err)
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
		log.Println("Fast-Https Exit.")
	}()
}
