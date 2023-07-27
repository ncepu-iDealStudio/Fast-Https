package server

import (
	"fast-https/modules/core/events"
	"fast-https/modules/core/listener"
	"fast-https/output"
	"fast-https/service"
	"fast-https/utils/message"
	"net"
)

//func Daemon(nochdir, noclose int) int {
//	var ret, ret2 uintptr
//	var err syscall.Errno
//	darwin := runtime.GOOS == "darwin"
//	// already a daemon
//	if syscall.Getppid() == 1 {
//		return 0
//	}
//	// fork off the parent process
//	ret, ret2, err = syscall.RawSyscall(syscall.SYS_FORK, 0, 0, 0)
//	if err != 0 {
//		return -1
//	}
//	// failure
//	if ret2 < 0 {
//		os.Exit(-1)
//	}
//	// handle exception for darwin
//	if darwin && ret2 == 1 {
//		ret = 0
//	}
//	// if we got a good PID, then we call exit the parent process.
//	if ret > 0 {
//		os.Exit(0)
//	}
//	/* Change the file mode mask */
//	_ = syscall.Umask(0)
//
//	// create a new SID for the child process
//	s_ret, s_errno := syscall.Setsid()
//	if s_errno != nil {
//		log.Printf("Error: syscall.Setsid errno: %d", s_errno)
//	}
//	if s_ret < 0 {
//		return -1
//	}
//	if nochdir == 0 {
//		os.Chdir("/")
//	}
//	if noclose == 0 {
//		f, e := os.OpenFile("/dev/null", os.O_RDWR, 0)
//		if e != nil {
//			fmt.Println("jklk")
//			fd := f.Fd()
//			syscall.Dup2(int(fd), int(os.Stdin.Fd()))
//			syscall.Dup2(int(fd), int(os.Stdout.Fd()))
//			syscall.Dup2(int(fd), int(os.Stderr.Fd()))
//		}
//	}
//	return 0
//}

// listen and serve one port
func serve_one_port(listener listener.ListenInfo) {
	for {
		conn, err := listener.Lfd.Accept()

		each_event := events.Event{}
		each_event.Conn = conn
		each_event.Lis_info = listener
		each_event.Timer = nil

		if err != nil {
			message.PrintErr("Error accepting connection:", err)
			continue
		}
		go events.Handle_event(each_event)
	}
}

// ScanPorts scan ports to check whether they've been used
func ScanPorts() error {
	ports := listener.Process_ports()
	for _, port := range ports {
		conn, err := net.Listen("tcp", "0.0.0.0:"+port)
		if err != nil {
			listener.Lisinfos = []listener.ListenInfo{}
			return err
		}
		conn.Close()
	}
	listener.Lisinfos = []listener.ListenInfo{}
	return nil
}

func Run() {
	output.PrintPortsListenerStart()
	service.TestService("0.0.0.0:5000")
	listens := listener.Listen()
	for _, value := range listens {
		go serve_one_port(value)
	}
	select {}
}
