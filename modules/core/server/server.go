package server

import (
	"fast-https/modules/core"
	"fast-https/modules/core/engine"
	"fast-https/modules/core/events"
	"fast-https/modules/core/listener"

	// routinepool "fast-https/modules/core/routine_pool"
	"fast-https/modules/safe"
	"fast-https/output"
	"fast-https/utils/message"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "fast-https/modules/dev_mod"
	_ "fast-https/modules/proxy"
	_ "fast-https/modules/rewrite"
	_ "fast-https/modules/static"
	_ "net/http/pprof"
)

type Server struct {
	Shutdown core.ServerControl
	Wg       sync.WaitGroup
}

// init server
func ServerInit() *Server {
	s := Server{Shutdown: core.ServerControl{Shutdown: false}}
	sigchnl := make(chan os.Signal, 1)
	signal.Notify(sigchnl)
	go func(s *Server) {
		for {
			sig_num := <-sigchnl
			s.sigHandler(sig_num)
		}
	}(&s)
	//  to do : ScanPorts
	return &s
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
		message.PrintInfo("The server got a kill signal")
		s.Shutdown.Shutdown = true

	} else if signal == syscall.SIGINT {
		message.PrintInfo("The server got a CTRL+C signal")
		s.Shutdown.Shutdown = true

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
func (s *Server) serveListener(listener1 listener.Listener) {

	l := &listener1
	// fmt.Printf("sizeof core.Event{}: %d\n", unsafe.Sizeof(core.Event{}))
	// fmt.Printf("sizeof []byte: %d\n", unsafe.Sizeof([]byte{100, 200}))
	// fmt.Printf("sizeof listener.ListenCfg{}: %d\n", unsafe.Sizeof(listener.ListenCfg{}))

	for !s.Shutdown.Shutdown {

		conn, err := listener1.Lfd.Accept()
		if err != nil {
			message.PrintErr("Error accepting connection:", err)
			continue
		}

		if l.LisType == 10 {
			go events.H2HandleEvent(l, conn, &(s.Shutdown))
		} else {
			go events.HandleEvent(l, conn, &(s.Shutdown))
		}
	}
}

func (s *Server) Run() {

	output.PrintPortsListenerStart()
	listens := listener.Listen()

	// TODO: improve this
	safe.Init() // need to be call after listener inited ...
	core.LogRegister()
	// if config.GConfig.ServerEngine.Id != 0 {
	engine.EngineInit()
	// }

	for _, value := range listens {
		go s.serveListener(value)
	}

	for !s.Shutdown.Shutdown {
		// <-sigchnl
		// fmt.Println("got sig")
		// s.wg.Wait()
		time.Sleep(time.Second * 1)
	}
}
