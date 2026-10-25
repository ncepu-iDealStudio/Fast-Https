// Package server 运行监听器、分发连接，并处理优雅停机与重载信号。
package server

import (
	"context"
	"errors"
	"fast-https/config"
	"fast-https/core"
	"fast-https/infra/logging"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/events"
	"fast-https/runtime/listener"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Server defines the core components of the server
// 服务器核心组件定义
// GracefulStopTimeout is how long SIGTERM and SIGQUIT wait for in-flight handlers.
const GracefulStopTimeout = 5 * time.Second

type Server struct {
	// Wg is used for server startup and shutdown synchronization
	// 用于同步服务器的启动和关闭
	Wg sync.WaitGroup
	// Listens holds all listener instances
	// 保存所有监听器实例
	Listens []*listener.Listener

	connWg   sync.WaitGroup
	mu       sync.Mutex
	stopping bool
	stopOnce sync.Once
}

// initModules initializes all modules that need to be initialized after the listener
// Including security module and dynamic log registration
//
// 初始化所有需要在监听器之后初始化的模块
// 包括安全模块和动态日志注册等
func initModules() {
	// Listeners already exist. Reload calls this again.
	core.RunAfterListen()
	dynlog.LogRegister()
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

	if err := core.MissingHandlers(config.Current().Servers); err != nil {
		logging.ConsoleLog.Fatal().Msgf("refuse to listen: %s", err.Error())
	}

	// 端口横幅由组合根打印，调度层不依赖展示层。
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
			listener.GLisinfos = []*listener.Listener{}
			return err
		}
		conn.Close()
	}
	listener.GLisinfos = []*listener.Listener{}
	return nil
}

// signalEffect is what the process should do with one OS signal.
// SIGINT reloads on every platform. Stopping uses SIGTERM, SIGQUIT, or the stop command.
// dev 前台模式例外：SIGINT 触发优雅停机（见 foregroundDev）。
type signalEffect int

const (
	signalIgnore signalEffect = iota
	signalStop
	signalReload
)

func signalEffectOf(sig os.Signal) signalEffect {
	switch sig {
	case syscall.SIGTERM, syscall.SIGQUIT:
		return signalStop
	case syscall.SIGINT:
		return signalReload
	default:
		return signalIgnore
	}
}

// foregroundDev 标记前台开发模式：SIGINT 触发优雅停机而非重载。
// 守护进程模式保持 SIGINT=reload（守护进程没有 Ctrl+C，reload 走命令）。
// 必须在 ServerInit 之前设置。
var foregroundDev atomic.Bool

// SetForegroundDev 标记前台开发模式（dev 命令调用）。
func SetForegroundDev(v bool) { foregroundDev.Store(v) }

// SigHandler handles system signals received by the server.
// SIGTERM and SIGQUIT stop the process. SIGINT reloads configuration,
// 除非处于 dev 前台模式——此时 SIGINT（Ctrl+C）优雅停机。
func (s *Server) SigHandler(sig os.Signal) {
	switch signalEffectOf(sig) {
	case signalStop:
		s.gracefulStop()
	case signalReload:
		if foregroundDev.Load() {
			logging.PrintInfo("dev mode: got SIGINT, shutting down gracefully")
			s.gracefulStop()
			return
		}
		logging.ConsoleLog.Info().Msg("========= server reload start ========")
		s.Reload()
	}
}

// gracefulStop stops accepting, waits for in-flight handlers, then unblocks Run.
// The wait ends at GracefulStopTimeout even if a handler is still running.
func (s *Server) gracefulStop() {
	s.stopOnce.Do(func() {
		logging.PrintInfo("The server got a stop signal")
		core.RunBeforeStop()
		s.drainConnections(GracefulStopTimeout)
		core.RunAfterStop()
		s.Wg.Done()
	})
}

// drainConnections cancels listeners and waits until tracked handlers return.
func (s *Server) drainConnections(timeout time.Duration) {
	s.mu.Lock()
	s.stopping = true
	s.mu.Unlock()
	for i := range s.Listens {
		if s.Listens[i].Cancel != nil {
			s.Listens[i].Cancel()
		}
	}
	done := make(chan struct{})
	go func() {
		s.connWg.Wait()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		logging.ConsoleLog.Info().Msg("graceful stop timed out")
	}
}

func (s *Server) serveConn(lis *listener.Listener, conn net.Conn, ctx context.Context) {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		_ = conn.Close()
		return
	}
	s.connWg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.connWg.Done()
		// 单个连接内模块代码的 panic 只能带走这条连接，不能带走进程。
		defer func() {
			if r := recover(); r != nil {
				logging.ConsoleLog.Error().Msgf("connection handler panic: %v\n%s", r, debug.Stack())
			}
		}()
		if lis.LisType == 10 {
			events.H2HandleEvent(lis, conn, ctx)
		} else {
			events.HandleEvent(lis, conn, ctx)
		}
	}()
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
// Features:
//   - Continuously accepts new connection requests
//   - Handles different types of connections (HTTP/1.1 or HTTP/2)
//   - Supports graceful shutdown
//
// 处理单个端口的监听和服务。
// reload 热更新通过 *Listener 上的配置快照到达这里，本函数不需要重启。
func (s *Server) serveListener(lis *listener.Listener) {
	ctx := lis.Ctx
	go func() {
		<-ctx.Done()
		if lis.Lfd != nil {
			_ = lis.Lfd.Close()
		}
	}()

out:
	for {
		select {
		case <-ctx.Done():
			logging.ConsoleLog.Debug().Msgf("Server on port %s is shutting down...\n", lis.Port)
			return
		default:

			conn, err := lis.Lfd.Accept()
			logging.ConsoleLog.Debug().Msgf("listener ptr %p, conn ptr %p", lis, conn)
			if err != nil {
				if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
					break out
				}
				// 瞬时错误（如 fd 用尽）退避后继续 accept，端口不能因一次错误停摆。
				logging.ConsoleLog.Debug().Msgf("Error accepting connection: %v", err)
				time.Sleep(10 * time.Millisecond)
				continue
			}

			s.serveConn(lis, conn, ctx)

		}
	}

	logging.ConsoleLog.Debug().Msgf("listening :%s shutdown ,it will not accept any connections", lis.Port)
}

var reloadConfig = config.Reload

// Reload reloads the server configuration.
// An invalid file, including a missing certificate, leaves the current listeners open.
func (s *Server) Reload() {
	if err := reloadConfig(); err != nil {
		logging.ConsoleLog.Error().Msgf("reload kept previous config: %s", err.Error())
		return
	}
	if err := logging.Open(config.Current().LogRoot); err != nil {
		logging.ConsoleLog.Error().Msgf("reload kept previous log files: %s", err.Error())
	}

	lisAll, lisAdded, removed := listener.ReloadListenCfg()

	removedSet := make(map[string]struct{}, len(removed))
	for _, port := range removed {
		removedSet[port] = struct{}{}
	}

	// 关闭被移除/类型变化的旧监听器。
	// common 端口的配置热更新已由 ReloadListenCfg 通过同一 *Listener
	// 指针完成，在途连接经配置快照读到新配置，这里无需再做。
	for _, old := range s.Listens {
		if _, ok := removedSet[old.Port]; !ok {
			continue
		}

		if old.Cancel != nil {
			old.Cancel()
		}
		if old.Lfd != nil {
			_ = old.Lfd.Close()
		}
		logging.ConsoleLog.Info().Msgf("reload removed listener on port %s", old.Port)
	}

	// 指向最新的ListenCfg数据
	s.Listens = lisAll

	// 开启新增端口的监听协程开始处理事件
	s.RunAdded(lisAdded)

	initModules()

	logging.ConsoleLog.Info().Msg("========= server reload  end  ========")
}

// RunAdded starts newly added listeners
//
// Parameters:
//   - lisAdded: List of newly added listeners
//
// Features:
//   - Starts a separate goroutine for each new listener
//
// 启动新增的监听器
func (s *Server) RunAdded(lisAdded []*listener.Listener) {
	for _, lis := range lisAdded {
		go s.serveListener(lis)
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

	for _, lis := range listens {
		go s.serveListener(lis)
	}

	s.Wg.Add(1)
	s.Wg.Wait()
}
