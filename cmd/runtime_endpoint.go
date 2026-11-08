package cmd

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"fast-https/infra/logging"
	"fast-https/runtime/metrics"
)

// runtimeListenAddr 是本机只读快照的固定地址（决策 MD2）。
const runtimeListenAddr = "127.0.0.1:9088"

var (
	runtimeMu  sync.Mutex
	runtimeSrv *http.Server
	runtimeLn  net.Listener
)

// StartRuntimeEndpoint 在 127.0.0.1:9088 提供 GET /runtime。
// 绑定失败只打日志并返回错误，调用方应继续启动业务端口。
func StartRuntimeEndpoint() error {
	return startRuntimeEndpoint(runtimeListenAddr)
}

func startRuntimeEndpoint(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logging.ConsoleLog.Error().Msgf("runtime endpoint listen %s failed: %s", addr, err.Error())
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/runtime", serveRuntime)
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: time.Second,
	}
	runtimeMu.Lock()
	if runtimeSrv != nil {
		runtimeMu.Unlock()
		_ = ln.Close()
		_ = srv.Close()
		return nil
	}
	runtimeSrv = srv
	runtimeLn = ln
	runtimeMu.Unlock()

	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.ConsoleLog.Error().Msgf("runtime endpoint serve: %s", err.Error())
		}
	}()
	return nil
}

func runtimeEndpointAddr() string {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if runtimeLn == nil {
		return ""
	}
	return runtimeLn.Addr().String()
}

// StopRuntimeEndpoint 关闭本机快照监听。AfterStop 调用它，不阻塞停机。
func StopRuntimeEndpoint() {
	runtimeMu.Lock()
	srv := runtimeSrv
	runtimeSrv = nil
	runtimeLn = nil
	runtimeMu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
}

func serveRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if err := enc.Encode(metrics.TakeSnapshot()); err != nil {
		logging.ConsoleLog.Debug().Msgf("runtime endpoint encode: %s", err.Error())
	}
}
