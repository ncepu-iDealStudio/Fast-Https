package server

import (
	"fast-https/modules/core"
	"fast-https/modules/core/events"
	"fast-https/modules/core/listener"
	"fast-https/modules/safe"
	"fast-https/output"
	"fast-https/service"
	"fast-https/utils/message"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Server struct {
	Shutdown bool
}

// init server
func Server_init() *Server {
	//  to do : ScanPorts
	return &Server{Shutdown: false}
}

// ScanPorts scan ports to check whether they've been used
func Scan_ports() error {
	ports := listener.Process_ports()
	for _, port := range ports {
		conn, err := net.Listen("tcp", "0.0.0.0:"+port)
		if err != nil {
			listener.Lisinfos = []listener.Listener{}
			return err
		}
		conn.Close()
	}
	listener.Lisinfos = []listener.Listener{}
	return nil
}

// register some signal handlers
func (s *Server) sig_handler(signal os.Signal) {
	if signal == syscall.SIGTERM {
		fmt.Println("Got kill signal. ")
		s.Shutdown = true

	} else if signal == syscall.SIGINT {
		fmt.Println("Got CTRL+C signal")
		s.Shutdown = true

	}
}

// set connection confgure
func (s *Server) set_conn_cfg(conn *net.Conn) {
	now := time.Now()
	(*conn).SetDeadline(now.Add(time.Second * 30))
}

// listen and serve one port
func (s *Server) serve_listener(listener listener.Listener) {
	// var wg sync.WaitGroup

	blacklist := safe.NewBlacklist()

	for !s.Shutdown {

		conn, err := listener.Lfd.Accept()
		if err != nil {
			message.PrintErr("Error accepting connection:", err)
			continue
		}
		s.set_conn_cfg(&conn)

		each_event := core.NewEvent(listener, conn)
		each_event.Conn = conn
		each_event.Lis_info = listener
		each_event.Timer = nil
		each_event.RR.Ev = each_event // include each other

		if !safe.Bucket(each_event) {
			continue
		}
		if blacklist.IsInBlacklist(each_event) {
			continue
		}
		events.Handle_event(each_event)

		// syncCalculateSum := func() {
		// 	events.Handle_event(each_event)
		// 	wg.Done()
		// }
		// wg.Add(1)
		// _ = ants.Submit(syncCalculateSum)
	}
}

func (s *Server) Run() {
	service.TestService("0.0.0.0:5000", "this is 5000")

	sigchnl := make(chan os.Signal, 1)
	signal.Notify(sigchnl)

	go func(s *Server) {
		for {
			sig_num := <-sigchnl
			s.sig_handler(sig_num)
		}
	}(s)

	output.PrintPortsListenerStart()
	listens := listener.Listen()

	for _, value := range listens {
		go s.serve_listener(value)
	}

	for !s.Shutdown {
		time.Sleep(time.Millisecond)
	}
}
