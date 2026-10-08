package server

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/dynlog"
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

// Server defines the core components of the server
// 服务器核心组件定义
type Server struct {
	// Wg is used for server startup and shutdown synchronization
	// 用于同步服务器的启动和关闭
	Wg sync.WaitGroup
	// Listens holds all listener instances
	// 保存所有监听器实例
	Listens []listener.Listener
}

// initModules initializes all modules that need to be initialized after the listener
// Including security module and dynamic log registration
//
// 初始化所有需要在监听器之后初始化的模块
// 包括安全模块和动态日志注册等
func initModules() {
	// TODO: improve this
	safe.Init() // need to be call after listener inited ...
	dynlog.LogRegister()
	// if config.GConfig.ServerEngine.Id != 0 {
	//engine.EngineInit()
	// }
}

// ServerInit initializes and returns a new server instance
// It sets up signal handling, initializes listeners and other modules
//
// Returns:
//   - *Server: The initialized server instance
//
// 初始化并返回一个新的服务器实例
// 设置信号处理，初始化监听器和其他模块
func ServerInit() *Server {
	s := Server{}
	sigchnl := make(chan os.Signal, 1)
	signal.Notify(sigchnl)
	go func(s *Server) {
		for {
			sig_num := <-sigchnl
			s.SigHandler(sig_num)
		}
	}(&s)
	//  to do : ScanPorts

	if err := core.MissingHandlers(config.GConfig.Servers); err != nil {
		logger.Fatal("refuse to listen: %s", err.Error())
	}

	output.PrintPortsListenerStart()
	s.Listens = listener.ListenWithCfg()

	initModules()

	return &s
}

// ScanPorts scans configured ports to check if they are already in use
//
// Returns:
//   - error: Returns error if ports are in use, nil otherwise
//
// 扫描配置的端口，检查它们是否已被占用
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

// sigHandler handles system signals received by the server
//
// Parameters:
//   - signal: The received system signal
//
// Signal handling:
//   - SIGTERM: Terminate signal, stops the service
//   - SIGINT: Interrupt signal (Ctrl+C)
//   - In foreground mode: stops the service
//   - In daemon mode: reloads the service
//   - SIGQUIT: Quit signal, stops the service
//
// 处理服务器接收到的系统信号
func (s *Server) SigHandler(signal os.Signal) {
	if signal == syscall.SIGTERM {
		message.PrintInfo("The server got a kill signal")
		s.Wg.Done()
	} else if signal == syscall.SIGINT {
		if config.GOs == "windows" {
			// windows 下，直接退出
			logger.Info("The server got an interrupt signal (Ctrl+C)")
			s.Wg.Done()
		} else {
			// linux 下，(Ctrl+C) 是重载信号
			logger.Info("========= server reload start ========")
			s.Reload()
		}
	} else if signal == syscall.SIGQUIT {
		message.PrintInfo("The server got a quit signal")
		s.Wg.Done()
	}
}

// setConnCfg configures connection parameters
//
// Parameters:
//   - conn: Pointer to the network connection to be configured
//
// Features:
//   - Sets connection timeout to 30 seconds
//   - Each connection activity requires setting a new deadline
//
// 设置连接的配置参数
func (s *Server) setConnCfg(conn *net.Conn) {
	now := time.Now()
	(*conn).SetDeadline(now.Add(time.Second * 30))
}

// serveListener handles listening and serving on a single port
//
// Parameters:
//   - offset: The offset of the listener in the list
//   - port_index: The port number being listened on
//
// Features:
//   - Continuously accepts new connection requests
//   - Handles different types of connections (HTTP/1.1 or HTTP/2)
//   - Supports graceful shutdown
//
// 处理单个端口的监听和服务
func (s *Server) serveListener(offset int, port_index int) {

	// fmt.Printf("sizeof core.Event{}: %d\n", unsafe.Sizeof(core.Event{}))
	// fmt.Printf("sizeof []byte: %d\n", unsafe.Sizeof([]byte{100, 200}))
	// fmt.Printf("sizeof listener.ListenCfg{}: %d\n", unsafe.Sizeof(listener.ListenCfg{}))

	listener1 := &s.Listens[offset]
	ctx := listener1.Ctx

out:

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Server on port %d is shutting down...\n", offset)
			// connWG.Wait() // 等待所有连接关闭
			logger.Debug("All connections on port %d closed.\n", offset)
			return
		default:

			conn, err := listener1.Lfd.Accept()
			logger.Debug("listener ptr %p, conn ptr %p", listener1, conn)
			if err != nil {
				logger.Debug("Error accepting connection: %v", err)

				break out
			}

			if listener1.LisType == 10 {
				// logger.Fatal("h2 not support in this branch")
				go events.H2HandleEvent(listener1, conn, ctx)
			} else {
				go events.HandleEvent(listener1, conn, ctx)
			}

		}
	}

	logger.Debug("listening :%d shutdown ,it will not accept any connections", port_index)
}

// Reload reloads the server configuration
//
// Features:
//   - Reloads configuration file
//   - Updates listener configuration
//   - Starts newly added ports
//   - Reinitializes modules
//   - Hot-updates configuration for common ports (in-place)
//
// 重新加载服务器配置
func (s *Server) Reload() {
	if err := config.Reload(); err != nil {
		logger.Error("reload kept previous config: %s", err.Error())
		return
	}

	lisAll, lisAdded, removed := listener.ReloadListenCfg()

	removedSet := make(map[string]struct{}, len(removed))
	for _, port := range removed {
		removedSet[port] = struct{}{}
	}

	// 在替换 s.Listens 之前，更新旧 s.Listens 中 common 端口的 Cfg/HostMap
	// 这样正在运行的 serveListener（持有旧 Listens 元素指针）能读到新配置
	// 这对多次 reload 尤为重要：每次 reload 时旧 serveListener 持有的 Listener
	// 都会被更新为最新配置
	newCfgMap := make(map[string]*listener.Listener, len(lisAll))
	for i := range lisAll {
		newCfgMap[lisAll[i].Port] = &lisAll[i]
	}
	for i := range s.Listens {
		old := &s.Listens[i]
		if _, removed := removedSet[old.Port]; removed {
			continue
		}
		if newest, ok := newCfgMap[old.Port]; ok {
			if old.LisType == newest.LisType {
				old.Cfg = newest.Cfg
				old.HostMap = newest.HostMap
				logger.Debug("reload: hot-updated config for port %s", old.Port)
			}
		}
	}

	for i := range s.Listens {
		old := &s.Listens[i]
		if _, ok := removedSet[old.Port]; !ok {
			continue
		}

		if old.Cancel != nil {
			old.Cancel()
		}
		if old.Lfd != nil {
			_ = old.Lfd.Close()
		}
		logger.Info("reload removed listener on port %s", old.Port)
	}

	// 指向最新的ListenCfg数据
	s.Listens = lisAll

	// 开启新增端口的监听协程开始处理事件
	s.RunAdded(lisAdded, len(lisAll)-len(lisAdded))

	initModules()

	logger.Info("========= server reload  end  ========")
}

// RunAdded starts newly added listeners
//
// Parameters:
//   - lisAdded: List of newly added listeners
//   - base: Base offset for the new listeners
//
// Features:
//   - Starts a separate goroutine for each new listener
//
// 启动新增的监听器
func (s *Server) RunAdded(lisAdded []listener.Listener, base int) {
	for offset, value := range lisAdded {
		n, err := strconv.Atoi(value.Port)
		if err != nil {
			logger.Fatal("cant convert listen port")
		}
		go s.serveListener(base+offset, n)
	}
}

// Run starts the server
//
// Features:
//   - Starts all configured listeners
//   - Waits for server stop signal
//
// 启动服务器，开始监听所有配置的端口
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
