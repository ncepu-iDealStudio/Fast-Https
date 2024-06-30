package server

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/events"
	"fast-https/modules/core/listener"
	"strconv"

	// routinepool "fast-https/modules/core/routine_pool"
	"fast-https/modules/safe"
	"fast-https/output"
	"fast-https/utils/logger"
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
)

type Server struct {
	Shutdown core.ServerControl
	Wg       sync.WaitGroup

	Listens []listener.Listener
}

// init modules thoses need to be inited after listener
func initModules() {
	// TODO: improve this
	safe.Init() // need to be call after listener inited ...
	core.LogRegister()
	// if config.GConfig.ServerEngine.Id != 0 {
	//engine.EngineInit()
	// }
}

// init server
func ServerInit() *Server {
	s := Server{Shutdown: core.ServerControl{}}
	sigchnl := make(chan os.Signal, 1)
	signal.Notify(sigchnl)
	go func(s *Server) {
		for {
			sig_num := <-sigchnl
			s.sigHandler(sig_num)
		}
	}(&s)
	//  to do : ScanPorts
	s.Shutdown = *core.NewServerContron()
	output.PrintPortsListenerStart()
	s.Listens = listener.ListenWithCfg()

	initModules()

	return &s
}

// ScanPorts scan ports to check whether they've been used
func ScanPorts() error {
	ports := listener.FindOldPorts()
	for _, port := range ports {
		conn, err := net.Listen("tcp", "0.0.0.0:"+port)
		if err != nil {
			listener.GLisinfos = []listener.Listener{}
			return err
		}
		conn.Close()
	}
	listener.GLisinfos = []listener.Listener{}
	return nil
}

// register some signal handlers
func (s *Server) sigHandler(signal os.Signal) {
	if signal == syscall.SIGTERM {
		message.PrintInfo("The server got a kill signal")
		// s.Shutdown.Shutdown = true
		s.Wg.Done()
	} else if signal == syscall.SIGINT {
		logger.Info("========= server reload start ========")
		// s.Shutdown.Shutdown = true
		s.Reload()
	} else if signal == syscall.SIGQUIT {
		message.PrintInfo("The server got a quit signal")
		// s.Shutdown.Shutdown = true
		s.Wg.Done()
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
func (s *Server) serveListener(offset int, port_index int) {

	// fmt.Printf("sizeof core.Event{}: %d\n", unsafe.Sizeof(core.Event{}))
	// fmt.Printf("sizeof []byte: %d\n", unsafe.Sizeof([]byte{100, 200}))
	// fmt.Printf("sizeof listener.ListenCfg{}: %d\n", unsafe.Sizeof(listener.ListenCfg{}))

	var listener1 *listener.Listener
	for !s.Shutdown.PortNeedShutdowm(port_index) {
		listener1 = &s.Listens[offset]
		conn, err := listener1.Lfd.Accept()
		logger.Debug("listener ptr %p, conn ptr %p", listener1, conn)
		if err != nil {
			logger.Debug("Error accepting connection: %v", err)
			continue
		}

		if listener1.LisType == 10 {
			// logger.Fatal("h2 not support in this branch")
			go events.H2HandleEvent(listener1, conn, &(s.Shutdown), port_index)
		} else {
			go events.HandleEvent(listener1, conn, &(s.Shutdown), port_index)
		}

	}

	logger.Debug("listening :%d shutdown ,it will not accept any connections", port_index)
	//s.Shutdown.PortShutdowmOk(port_index)
}

func (s *Server) Reload() {
	config.Reload()

	lisAll, lisAdded, removed := listener.ReloadListenCfg()

	// 设置需要移除的端口
	s.Shutdown.RemovedPortsToBitArray(removed)

	// 指向最新的ListenCfg数据
	s.Listens = lisAll

	// 开启新增端口的监听协程开始处理事件
	s.RunAdded(lisAdded, len(lisAll)-len(lisAdded))

	initModules()

	logger.Info("========= server reload  end  ========")
}

func (s *Server) RunAdded(lisAdded []listener.Listener, base int) {
	for offset, value := range lisAdded {
		n, err := strconv.Atoi(value.Port)
		if err != nil {
			logger.Fatal("cant convert listen port")
		}
		go s.serveListener(base+offset, n)
	}
}

func (s *Server) Run() {

	listens := s.Listens

	for offset, value := range listens {
		n, err := strconv.Atoi(value.Port)
		if err != nil {
			logger.Fatal("cant convert listen port")
		}
		go s.serveListener(offset, n)
	}

	// for !s.Shutdown.Shutdown {
	// 	// <-sigchnl
	// 	// fmt.Println("got sig")
	// 	// s.wg.Wait()
	// 	time.Sleep(time.Second * 1)
	// }
	s.Wg.Add(1)
	s.Wg.Wait()
}
