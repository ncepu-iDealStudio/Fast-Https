package server

import (
	"fast-https/modules/core"
	"fast-https/modules/core/events"
	"fast-https/modules/core/listener"
	"fast-https/modules/safe"
	"fast-https/output"
	"fast-https/utils/message"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "fast-https/modules/proxy"
	_ "fast-https/modules/rewrite"
	_ "fast-https/modules/static"

	"github.com/panjf2000/ants"
)

type Server struct {
	Shutdown bool
}

// init server
func ServerInit() *Server {
	//  to do : ScanPorts
	return &Server{Shutdown: false}
}

// ScanPorts scan ports to check whether they've been used
func ScanPorts() error {
	ports := listener.ProcessPorts()
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
func (s *Server) sigHandler(signal os.Signal) {
	if signal == syscall.SIGTERM {
		fmt.Println("Got kill signal. ")
		s.Shutdown = true

	} else if signal == syscall.SIGINT {
		fmt.Println("Got CTRL+C signal")
		s.Shutdown = true

	}
}

// set connection confgure
/*
   The Conn interface also has deadline settings; either for the connection as
   a whole (SetDeadLine()) or specific to read or write calls (SetReadDeadLine()
   and SetWriteDeadLine()). Note that the deadlines are fixed points in (wallclock)
   time. Unlike timeouts, they don’t reset after a new activity. Each activity on
   the connection must therefore set a new deadline.
*/
func (s *Server) setConnCfg(conn *net.Conn) {
	now := time.Now()
	(*conn).SetDeadline(now.Add(time.Second * 30))
}

// listen and serve one port
func (s *Server) serveListener(listener listener.Listener) {
	var wg sync.WaitGroup

	for !s.Shutdown {

		conn, err := listener.Lfd.Accept()
		if err != nil {
			message.PrintErr("Error accepting connection:", err)
			continue
		}
		s.setConnCfg(&conn)

		each_event := core.NewEvent(listener, conn)
		each_event.Conn = conn
		each_event.Lis_info = listener
		each_event.Timer = nil
		each_event.Reuse = false
		each_event.IsClose = false
		each_event.RR.Ev = each_event // include each other
		each_event.RR.IsCircle = true
		each_event.RR.CircleInit = false
		each_event.RR.ProxyConnInit = false

		if !safe.Bucket(each_event) {
			continue
		}

		if safe.IsInBlacklist(each_event) {
			continue
		}
		// go events.HandleEvent(each_event)

		syncCalculateSum := func() {
			events.HandleEvent(each_event)
			wg.Done()
		}
		wg.Add(1)
		_ = ants.Submit(syncCalculateSum)
	}
}

func (s *Server) Run() {
	// service.TestService("0.0.0.0:5000", "this is 5000")

	sigchnl := make(chan os.Signal, 1)
	signal.Notify(sigchnl)

	go func(s *Server) {
		for {
			sig_num := <-sigchnl
			s.sigHandler(sig_num)
		}
	}(s)

	output.PrintPortsListenerStart()
	listens := listener.Listen()

	// TODO: improve this
	safe.Init() // need to be call after listener inited ...

	for _, value := range listens {
		go s.serveListener(value)
	}

	for !s.Shutdown {
		time.Sleep(time.Millisecond * 10)
	}
}
