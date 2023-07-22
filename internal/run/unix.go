package run

import (
	"fast-https/cmd"
	"fmt"
	"github.com/sevlyar/go-daemon"
	"os"
	"os/signal"
	"syscall"
)

func RunUnix() {
	cntxt := &daemon.Context{
		PidFileName: "fast-https",
		PidFilePerm: 0644,
		LogFileName: "syslog",
		LogFilePerm: 0640,
		WorkDir:     "./",
		Umask:       027,
	}

	// send signal to stop daemon
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// daemonize the process
	child, err := cntxt.Reborn()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if child != nil {
		// parent process
		fmt.Println("Daemon started with PID:", child.Pid)
		// wait for stop signal
		<-stop
		fmt.Println("Daemon stopped")
		return
	}
	defer cntxt.Release()

	// child process
	fmt.Println("Child process started")
	// do something here
	rootcmd := cmd.RootCmd()
	rootcmd.Execute()
	// wait for stop signal
	<-stop
	fmt.Println("Child process stopped")
}
