package cmd

import (
	"errors"
	"fmt"
	"io"
	"time"

	"fast-https/management/rtclient"

	"github.com/spf13/cobra"
)

func newRuntimeCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "runtime",
		Short:        t("查询本机运行快照", "query the local runtime snapshot"),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.AddCommand(newRuntimeStatusCmd(), newRuntimeStatsCmd())
	return root
}

func newRuntimeStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: t("查询当前服务运行态", "query the current runtime state"),
		Long: t(
			"读取 127.0.0.1:9088/runtime，返回连接与请求等瞬时状态。\n"+
				"连不上快照时退出码 4，不要回退成读日志猜测。reading/writing/waiting 与 CPU/内存目前没有可靠数据源，会省略并给出告警。",
			"Read 127.0.0.1:9088/runtime and return instantaneous connection and request state.\n"+
				"An unreachable snapshot exits with code 4; the command never guesses from logs.\n"+
				"reading/writing/waiting and CPU/memory are omitted with warnings until a reliable source exists."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runRuntimeStatus,
	}
}

func newRuntimeStatsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "stats",
		Short: t("查询进程启动以来的累计统计", "query cumulative stats since process start"),
		Long: t(
			"读取 127.0.0.1:9088/runtime 的累计计数。窗口固定为 since_process_start。\n"+
				"--since 与 --domain 在没有窗口化数据前返回 NOT_SUPPORTED（退出码 11）。",
			"Read cumulative counters from 127.0.0.1:9088/runtime. The window is always since_process_start.\n"+
				"--since and --domain return NOT_SUPPORTED (exit code 11) until windowed data exists."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runRuntimeStats,
	}
	c.Flags().String("since", "", t("统计时间窗口（尚未支持）", "stats time window (not supported yet)"))
	c.Flags().String("domain", "", t("按域名过滤（尚未支持）", "filter by domain (not supported yet)"))
	return c
}

func runRuntimeStatus(cmd *cobra.Command, _ []string) error {
	snap, err := fetchRuntime(cmd.Context())
	if err != nil {
		return runtimeCLIError("runtime.status", err)
	}
	data, warnings := runtimeStatusData(snap)
	if jsonMode(cmd) {
		return emitOK(cmd, "runtime.status", data, warnings)
	}
	printWarnings(cmd.ErrOrStderr(), warnings)
	writeRuntimeStatusText(cmd.OutOrStdout(), data)
	return nil
}

func runRuntimeStats(cmd *cobra.Command, _ []string) error {
	if cmd.Flags().Changed("since") || cmd.Flags().Changed("domain") {
		return &CLIError{
			Code:    CodeNotSupported,
			Command: "runtime.stats",
			Message: "--since and --domain are not supported until windowed stats exist",
		}
	}
	snap, err := fetchRuntime(cmd.Context())
	if err != nil {
		return runtimeCLIError("runtime.stats", err)
	}
	data, warnings := runtimeStatsData(snap)
	if jsonMode(cmd) {
		return emitOK(cmd, "runtime.stats", data, warnings)
	}
	printWarnings(cmd.ErrOrStderr(), warnings)
	writeRuntimeStatsText(cmd.OutOrStdout(), data)
	return nil
}

func runtimeCLIError(id string, err error) *CLIError {
	ce := &CLIError{Command: id, Message: err.Error(), Cause: err}
	if errors.Is(err, rtclient.ErrSchema) {
		ce.Code = CodeUnmetDependency
		return ce
	}
	ce.Code = CodeServiceDown
	ce.Message = err.Error() + "; the runtime endpoint binds 127.0.0.1:9088 and is skipped when that address is occupied"
	return ce
}

func runtimeStatusData(snap rtclient.Snapshot) (map[string]any, []string) {
	data := map[string]any{
		"observed_at": snap.ObservedAt,
		"source":      "runtime",
		"state":       snap.Server.State,
		"connections": map[string]any{
			"active":         snap.Connections.Active,
			"accepted_total": snap.Connections.AcceptedTotal,
			"closed_total":   snap.Connections.ClosedTotal,
			"rejected_total": snap.Connections.RejectedTotal,
		},
		"requests": map[string]any{
			"active": snap.Requests.Active,
			"total":  snap.Requests.Total,
		},
	}
	if snap.Server.PID != 0 {
		data["pid"] = snap.Server.PID
	}
	if snap.Server.StartedAt != "" {
		data["started_at"] = snap.Server.StartedAt
	}
	if u := snap.Server.UptimeSeconds(time.Now()); u != nil {
		data["uptime_seconds"] = *u
	}
	warnings := []string{
		"connections.reading, writing and waiting are not collected",
		"resources.cpu_percent, memory_bytes and open_files are not collected",
	}
	return data, warnings
}

func runtimeStatsData(snap rtclient.Snapshot) (map[string]any, []string) {
	data := map[string]any{
		"observed_at": snap.ObservedAt,
		"source":      "runtime",
		"window":      "since_process_start",
		"requests": map[string]any{
			"active":             snap.Requests.Active,
			"total":              snap.Requests.Total,
			"parse_errors_total": snap.Requests.ParseErrorsTotal,
			"aborted_total":      snap.Requests.AbortedTotal,
		},
		"responses": map[string]any{
			"2xx_total":     snap.Responses2xx,
			"3xx_total":     snap.Responses3xx,
			"4xx_total":     snap.Responses4xx,
			"5xx_total":     snap.Responses5xx,
			"unknown_total": snap.ResponsesUnknown,
		},
	}
	if snap.RequestDuration != nil {
		data["request_duration"] = snap.RequestDuration
	}
	if snap.Upstream != nil {
		data["upstream"] = snap.Upstream
	}
	var warnings []string
	if snap.RequestDuration == nil {
		warnings = append(warnings, "request_duration is not present in the snapshot")
	}
	if snap.Upstream == nil {
		warnings = append(warnings, "upstream stats are omitted until the proxy section is registered")
	}
	return data, warnings
}

func writeRuntimeStatusText(w io.Writer, data map[string]any) {
	fmt.Fprintf(w, "state: %v\n", data["state"])
	if pid, ok := data["pid"]; ok {
		fmt.Fprintf(w, "pid: %v\n", pid)
	}
	if u, ok := data["uptime_seconds"]; ok {
		fmt.Fprintf(w, "uptime_seconds: %v\n", u)
	}
	if conn, ok := data["connections"].(map[string]any); ok {
		fmt.Fprintf(w, "connections.active: %v\n", conn["active"])
	}
	if req, ok := data["requests"].(map[string]any); ok {
		fmt.Fprintf(w, "requests.active: %v\n", req["active"])
		fmt.Fprintf(w, "requests.total: %v\n", req["total"])
	}
}

func writeRuntimeStatsText(w io.Writer, data map[string]any) {
	fmt.Fprintf(w, "window: %v\n", data["window"])
	if req, ok := data["requests"].(map[string]any); ok {
		fmt.Fprintf(w, "requests.total: %v\n", req["total"])
	}
	if resp, ok := data["responses"].(map[string]any); ok {
		fmt.Fprintf(w, "responses.4xx_total: %v\n", resp["4xx_total"])
		fmt.Fprintf(w, "responses.5xx_total: %v\n", resp["5xx_total"])
	}
}
