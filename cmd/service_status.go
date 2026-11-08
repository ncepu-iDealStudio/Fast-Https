package cmd

import (
	"fmt"
	"io"

	"fast-https/config"
	"fast-https/management/view"

	"github.com/spf13/cobra"
)

func newServiceStatusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: t("查询服务运行状态", "query the service status"),
		Long: t(
			"查询服务运行状态：读 pid 文件判断进程，再对配置里的端口做一次回环连接。\n"+
				"状态：running、degraded（进程在但端口都连不上）、stopped、unknown（证据互相矛盾）。\n"+
				"这是一次只读查询：服务没在跑也是成功的查询，退出码为 0；旧的顶层 status 命令仍以非 0 退出。\n\n"+
				"运行时长、启动时刻等要等服务提供只读快照后才有，目前不输出。",
			"query the service status: the pid file decides whether the process is alive, then each\n"+
				"configured port gets one loopback connection.\n"+
				"States: running, degraded (process alive but no port connects), stopped, unknown\n"+
				"(contradictory evidence). This is a read-only query: a stopped service is still a\n"+
				"successful query with exit code 0; the legacy top-level status command still exits non-zero.\n\n"+
				"Uptime and start time are not reported until the server exposes a read-only snapshot."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			ports, portsErr := config.ListenPortsFromFile(path)

			st, warnings := view.ServiceStatusOf(view.ServiceStatusInput{
				Inspect: func() (view.ProcState, int, error) {
					state, pid, err := InspectProcess(config.PID_FILE)
					return procStateOf(state), pid, err
				},
				Ports:    ports,
				PortsErr: portsErr,
			})

			if jsonMode(cmd) {
				return emitOK(cmd, "service.status", st, warnings)
			}
			printWarnings(cmd.ErrOrStderr(), warnings)
			writeServiceStatusText(cmd.OutOrStdout(), st)
			return nil
		},
	}
	addConfigFlag(c)
	return c
}

func procStateOf(s ProcessState) view.ProcState {
	switch s {
	case ProcessRunning:
		return view.ProcRunning
	case ProcessStale:
		return view.ProcStale
	default:
		return view.ProcAbsent
	}
}

func writeServiceStatusText(w io.Writer, st view.ServiceStatus) {
	if st.PID != nil {
		fmt.Fprintf(w, "fast-https: %s (pid %d)\n", st.Status, *st.PID)
	} else {
		fmt.Fprintf(w, "fast-https: %s\n", st.Status)
	}
	for _, p := range st.Ports {
		state := "unreachable"
		if p.Reachable {
			state = "reachable"
		}
		fmt.Fprintf(w, "port %d: %s\n", p.Port, state)
	}
}
