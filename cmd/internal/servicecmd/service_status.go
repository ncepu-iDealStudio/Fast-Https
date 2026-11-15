package servicecmd

import (
	"fast-https/cmd/internal/cli"
	"fast-https/cmd/internal/procctl"
	"fmt"
	"io"
	"time"

	"fast-https/config"
	"fast-https/management/cfgver"
	"fast-https/management/rtclient"
	"fast-https/management/view"

	"github.com/spf13/cobra"
)

func NewServiceStatusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: cli.T("查询服务运行状态", "query the service status"),
		Long: cli.T(
			"查询服务运行状态：读 pid 文件判断进程，再对配置里的端口做一次回环连接。\n"+
				"状态：running、degraded（进程在但端口都连不上）、stopped、unknown（证据互相矛盾）。\n"+
				"读得到 /runtime 时补 uptime_seconds、started_at、version；config_version 来自状态目录里当前 active 的快照。\n"+
				"这是一次只读查询：服务没在跑也是成功的查询，退出码为 0；旧的顶层 status 命令仍以非 0 退出。",
			"query the service status: the pid file decides whether the process is alive, then each\n"+
				"configured port gets one loopback connection.\n"+
				"States: running, degraded (process alive but no port connects), stopped, unknown\n"+
				"(contradictory evidence). When /runtime is reachable, uptime_seconds, started_at and version\n"+
				"are filled in; config_version comes from the active snapshot in the state directory.\n"+
				"This is a read-only query: a stopped service is still a successful query with exit code 0;\n"+
				"the legacy top-level status command still exits non-zero."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			ports, portsErr := config.ListenPortsFromFile(path)

			st, warnings := view.ServiceStatusOf(view.ServiceStatusInput{
				Inspect: func() (view.ProcState, int, error) {
					state, pid, err := procctl.InspectProcess(config.PID_FILE)
					return ProcStateOf(state), pid, err
				},
				Ports:    ports,
				PortsErr: portsErr,
			})
			snap, snapErr := cli.FetchRuntime(cmd.Context())
			st, warnings = EnrichServiceStatus(st, warnings, snap, snapErr, ActiveConfigVersion(path))

			if cli.JSONMode(cmd) {
				return cli.EmitOK(cmd, "service.status", st, warnings)
			}
			cli.PrintWarnings(cmd.ErrOrStderr(), warnings)
			WriteServiceStatusText(cmd.OutOrStdout(), st)
			return nil
		},
	}
	cli.AddConfigFlag(c)
	return c
}

func ProcStateOf(s procctl.ProcessState) view.ProcState {
	switch s {
	case procctl.ProcessRunning:
		return view.ProcRunning
	case procctl.ProcessStale:
		return view.ProcStale
	default:
		return view.ProcAbsent
	}
}

func WriteServiceStatusText(w io.Writer, st view.ServiceStatus) {
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
	if st.UptimeSeconds != nil {
		fmt.Fprintf(w, "uptime_seconds: %d\n", *st.UptimeSeconds)
	}
	if st.StartedAt != "" {
		fmt.Fprintf(w, "started_at: %s\n", st.StartedAt)
	}
	if st.ConfigVersion != "" {
		fmt.Fprintf(w, "config_version: %s\n", st.ConfigVersion)
	}
	if st.Version != "" {
		fmt.Fprintf(w, "version: %s\n", st.Version)
	}
}

func EnrichServiceStatus(st view.ServiceStatus, warnings []string, snap rtclient.Snapshot, snapErr error, cfgVer string) (view.ServiceStatus, []string) {
	if cfgVer != "" {
		st.ConfigVersion = cfgVer
	}
	processUp := st.Status == "running" || st.Status == "degraded"
	if !processUp {
		return st, warnings
	}
	if snapErr != nil {
		return st, append(warnings, "uptime, started_at and version unavailable: runtime snapshot unreachable")
	}
	st.StartedAt = snap.Server.StartedAt
	st.Version = snap.Server.Version
	st.UptimeSeconds = snap.Server.UptimeSeconds(time.Now())
	if st.Source == "pid_file+port_probe" {
		st.Source = "pid_file+port_probe+runtime"
	}
	return st, warnings
}

func ActiveConfigVersion(mainPath string) string {
	list, err := cfgver.List(cli.StateDir(), mainPath)
	if err != nil {
		return ""
	}
	for _, v := range list.Versions {
		if v.Active {
			return v.Version
		}
	}
	return ""
}
