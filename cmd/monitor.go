package cmd

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"fast-https/management/logquery"
	"fast-https/management/monview"

	"github.com/spf13/cobra"
)

func newMonitorCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "monitor",
		Short:        t("查询独立 Monitor 的状态与事件", "query the standalone monitor status and events"),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.AddCommand(newMonitorStatusCmd(), newMonitorEventsCmd())
	return root
}

func newMonitorStatusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: t("查询 Monitor 是否在采集", "query whether the monitor is collecting"),
		Long: t(
			"只读 Monitor 写下的状态文件，不连接 Monitor 进程。\n"+
				"文件不存在时仍是一次成功的查询，status 为 not_installed。\n"+
				"最近一次采集早于 30 秒则 status 为 stale。",
			"Read the status file the monitor writes. This command does not connect to the monitor process.\n"+
				"A missing file is still a successful query with status not_installed.\n"+
				"If the last collection is older than 30s, status is stale."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runMonitorStatus,
	}
	addConfigFlag(c)
	return c
}

func newMonitorEventsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "events",
		Short: t("查询 Monitor 事件文件", "query the monitor event file"),
		Long: t(
			"只读事件 JSONL。--since、--limit（默认 100，超过 1000 钳到 1000）、--severity（info、warning、error、critical）。\n"+
				"坏行跳过并计数。事件文本只作为数据返回。",
			"Read the event JSONL. --since, --limit (default 100, capped at 1000), and --severity\n"+
				"(info, warning, error, critical). Non-JSON lines are skipped and counted.\n"+
				"Event text is returned only as data."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runMonitorEvents,
	}
	addConfigFlag(c)
	c.Flags().String("since", "", t("只看这个时长之内的事件，例如 1h、30m", "only events newer than this duration, e.g. 1h or 30m"))
	c.Flags().Int("limit", 0, t("返回条数，默认 100，超过 1000 时钳到 1000", "events to return; default 100, values above 1000 are capped"))
	c.Flags().String("severity", "", t("按严重级别过滤：info、warning、error、critical", "filter by severity: info, warning, error, or critical"))
	return c
}

func runMonitorStatus(cmd *cobra.Command, _ []string) error {
	path, _ := cmd.Flags().GetString("config")
	logRoot, _ := logquery.ResolveLogDir(path)
	st, warnings, err := monview.ReadStatus(monview.StatusInput{LogRoot: logRoot})
	if err != nil {
		return fromMonitorError("monitor.status", err)
	}
	if jsonMode(cmd) {
		return emitOK(cmd, "monitor.status", st, warnings)
	}
	printWarnings(cmd.ErrOrStderr(), warnings)
	fmt.Fprintf(cmd.OutOrStdout(), "monitor: %s\n", st.Status)
	if st.LastCollectAt != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "last_collect_at: %s\n", st.LastCollectAt)
	}
	if st.LastError != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "last_error: %s\n", st.LastError)
	}
	return nil
}

func runMonitorEvents(cmd *cobra.Command, _ []string) error {
	path, _ := cmd.Flags().GetString("config")
	since, _ := cmd.Flags().GetString("since")
	limit, _ := cmd.Flags().GetInt("limit")
	severity, _ := cmd.Flags().GetString("severity")
	logRoot, warnings := logquery.ResolveLogDir(path)
	res, more, err := monview.ReadEvents(monview.EventsInput{
		LogRoot: logRoot, Since: since, Limit: limit, Severity: severity,
	})
	if err != nil {
		return fromMonitorError("monitor.events", err)
	}
	warnings = append(warnings, more...)
	if jsonMode(cmd) {
		return emitOK(cmd, "monitor.events", res, warnings)
	}
	printWarnings(cmd.ErrOrStderr(), warnings)
	writeMonitorEvents(cmd.OutOrStdout(), res)
	return nil
}

func writeMonitorEvents(w io.Writer, res monview.EventsResult) {
	if len(res.Events) == 0 {
		fmt.Fprintln(w, "(no events)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, ev := range res.Events {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", ev.Time, ev.Severity, ev.Type, ev.Message)
	}
	_ = tw.Flush()
}

func fromMonitorError(id string, err error) *CLIError {
	ce := &CLIError{Command: id, Message: err.Error(), Cause: err}
	var fe *monview.FilterError
	switch {
	case errors.As(err, &fe):
		ce.Code = CodeUsage
	case errors.Is(err, monview.ErrSchema):
		ce.Code = CodeUnmetDependency
	case errors.Is(err, monview.ErrSymlink):
		ce.Code = CodeLogUnavailable
	default:
		ce.Code = CodeInternal
	}
	return ce
}
