package run

import (
	"fast-https/cmd"
	"fast-https/output"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/getlantern/systray"
	"github.com/skratchdot/open-golang/open"
)

func StartWindows() {
	onExit := func() {
		now := time.Now()
		ioutil.WriteFile(fmt.Sprintf(`on_exit_%d.txt`, now.UnixNano()), []byte(now.String()), 0644)
	}

	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTemplateIcon(output.Logo, output.Logo)
	systray.SetTitle("Fast Https")
	systray.SetTooltip("Lantern")

	// We can manipulate the systray in other goroutines
	go func() {
		systray.SetTemplateIcon(output.Logo, output.Logo)
		systray.SetTitle("Fast Https")
		systray.SetTooltip("Fast Https")
		mStart := systray.AddMenuItem("Start", "Start server")
		mStop := systray.AddMenuItem("Stop", "Stop server")
		mReload := systray.AddMenuItem("Reload", "Reload server")
		mUrl := systray.AddMenuItem("Github", "Project Source code")

		systray.AddSeparator()

		for {
			select {
			case <-mStart.ClickedCh:
				log.Println("Fast-Https Starting...")
				err := cmd.StartHandler()
				if err != nil {
					log.Fatal(err)
				}
			case <-mStop.ClickedCh:
				err := cmd.StopHandler()
				if err != nil {
					log.Fatal(err)
				} else {
					log.Println("Fast-Https Stop")
				}
			case <-mReload.ClickedCh:
				err := cmd.ReloadHandler()
				if err != nil {
					log.Fatal(err)
				} else {
					log.Println("Fast-Https Reload")
				}
			case <-mUrl.ClickedCh:
				open.Run("https://gitee.com/ncepu-bj/fast-https")
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
