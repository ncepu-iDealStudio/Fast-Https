package cmd

import (
	"io"
	"os"
	"time"

	"fast-https/cmd/internal/procctl"
	"fast-https/cmd/internal/servicecmd"
	"fast-https/management/rtclient"
	"fast-https/management/view"

	"github.com/spf13/cobra"
)

// 服务命令组与生命周期实现已迁到 cmd/internal/servicecmd。这里保留 cmd
// 包内的旧名字，供 commands.go 里的顶层 handler 与测试使用；收尾时删除。

func newServiceCmd() *cobra.Command { return servicecmd.NewServiceCmd() }

func newServiceStatusCmd() *cobra.Command { return servicecmd.NewServiceStatusCmd() }

func procStateOf(s procctl.ProcessState) view.ProcState { return servicecmd.ProcStateOf(s) }

func writeServiceStatusText(w io.Writer, st view.ServiceStatus) {
	servicecmd.WriteServiceStatusText(w, st)
}

func activeConfigVersion(mainPath string) string { return servicecmd.ActiveConfigVersion(mainPath) }

func daemonize() (int, bool, error) { return servicecmd.Daemonize() }

func notifyDaemonReady() { servicecmd.NotifyDaemonReady() }

func waitDaemonStarted(pid int, timeout time.Duration) error {
	return servicecmd.WaitDaemonStarted(pid, timeout)
}

func signalStopProcess(process *os.Process) error { return servicecmd.SignalStopProcess(process) }

func signalReloadProcess(process *os.Process, pid int) error {
	return servicecmd.SignalReloadProcess(process, pid)
}

func sendCtrlC(pid int) error { return servicecmd.SendCtrlC(pid) }

// StartRuntimeEndpoint / StopRuntimeEndpoint 是服务端的运行时查询端点。
func StartRuntimeEndpoint() error { return servicecmd.StartRuntimeEndpoint() }

// StopRuntimeEndpoint 关闭运行时查询端点。
func StopRuntimeEndpoint() { servicecmd.StopRuntimeEndpoint() }

// RegisterHandlers 注册服务器事件处理器。
func RegisterHandlers() { servicecmd.RegisterHandlers() }

// RegisterServices 注册所有自定义服务。
func RegisterServices() { servicecmd.RegisterServices() }

// RuntimeListenAddr 是运行时查询端点的固定监听地址。
const RuntimeListenAddr = servicecmd.RuntimeListenAddr

func startRuntimeEndpoint(addr string) error { return servicecmd.StartRuntimeEndpointWith(addr) }

func runtimeEndpointAddr() string { return servicecmd.RuntimeEndpointAddr() }

func enrichServiceStatus(st view.ServiceStatus, warnings []string, snap rtclient.Snapshot, snapErr error, version string) (view.ServiceStatus, []string) {
	return servicecmd.EnrichServiceStatus(st, warnings, snap, snapErr, version)
}

// runtimeListenAddr 与 servicecmd 里的固定监听地址保持一致（测试用）。
const runtimeListenAddr = servicecmd.RuntimeListenAddr
