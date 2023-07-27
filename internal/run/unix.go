package run

import (
	"fast-https/cmd"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sevlyar/go-daemon"
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

// func Daemon(nochdir, noclose int) int {
// 	var ret, ret2 uintptr
// 	var err syscall.Errno
// 	darwin := runtime.GOOS == "darwin"
// 	// already a daemon
// 	if syscall.Getppid() == 1 {
// 		return 0
// 	}
// 	// fork off the parent process
// 	ret, ret2, err = syscall.RawSyscall(syscall.SYS_FORK, 0, 0, 0)
// 	if err != 0 {
// 		return -1
// 	}
// 	// failure
// 	if ret2 < 0 {
// 		os.Exit(-1)
// 	}
// 	// handle exception for darwin
// 	if darwin && ret2 == 1 {
// 		ret = 0
// 	}
// 	// if we got a good PID, then we call exit the parent process.
// 	if ret > 0 {
// 		os.Exit(0)
// 	}
// 	/* Change the file mode mask */
// 	_ = syscall.Umask(0)

// 	// create a new SID for the child process
// 	s_ret, s_errno := syscall.Setsid()
// 	if s_errno != nil {
// 		log.Printf("Error: syscall.Setsid errno: %d", s_errno)
// 	}
// 	if s_ret < 0 {
// 		return -1
// 	}
// 	if nochdir == 0 {
// 		os.Chdir("/")
// 	}
// 	if noclose == 0 {
// 		f, e := os.OpenFile("/dev/null", os.O_RDWR, 0)
// 		if e != nil {

// 			fd := f.Fd()
// 			syscall.Dup2(int(fd), int(os.Stdin.Fd()))
// 			syscall.Dup2(int(fd), int(os.Stdout.Fd()))
// 			syscall.Dup2(int(fd), int(os.Stderr.Fd()))
// 		}
// 	}
// 	return 0
// }
