package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"fast-https/management/logquery"

	"github.com/spf13/cobra"
)

// logs 只读四路固定日志。没有 --file：文件名由子命令或 --kind 决定，
// 过滤条件是白名单字段，日志内容只作为数据返回。
func newLogsCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "logs",
		Short: t("按白名单条件查询日志", "query logs with an allow-listed filter"),
		Long: t(
			"只读 access.log、error.log、system.log、safe.log。\n"+
				"不提供 --file，不读轮转后的旧文件，也不执行日志里的任何内容。\n"+
				"结果只含固定字段，查询串、Bearer 和 Basic 凭据会脱敏。",
			"Read access.log, error.log, system.log, and safe.log.\n"+
				"There is no --file flag, rotated files are not read, and nothing in a log line is executed.\n"+
				"Only fixed fields are returned. Query secrets, Bearer tokens, and Basic credentials are redacted."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.AddCommand(newLogsErrorCmd(), newLogsAccessCmd(), newLogsQueryCmd())
	return root
}

func newLogsErrorCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "error",
		Short: t("查询 error.log", "query error.log"),
		Long: t(
			"从 error.log 尾部读取。目前这个文件只含 PrintErr 写进去的行，\n"+
				"控制台里的警告和错误要等后续步骤才会写入。",
			"Read the tail of error.log. Today that file only contains lines written by PrintErr;\n"+
				"console warnings and errors are not persisted yet."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd, "logs.error", "error")
		},
	}
	addLogReadFlags(c, false)
	return c
}

func newLogsAccessCmd() *cobra.Command {
	c := &cobra.Command{
		Use:          "access",
		Short:        t("查询 access.log", "query access.log"),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd, "logs.access", "access")
		},
	}
	addLogReadFlags(c, true)
	return c
}

func newLogsQueryCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "query",
		Short: t("按白名单字段查询一路日志", "query one log stream with allow-listed fields"),
		Long: t(
			"--kind 必填，取值 access、error、system、safe，一次只查一个文件。\n"+
				"不支持表达式、SQL 或脚本。",
			"--kind is required: access, error, system, or safe. One file per invocation.\n"+
				"Expressions, SQL, and scripts are not accepted."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, _ := cmd.Flags().GetString("kind")
			if strings.TrimSpace(kind) == "" {
				return &CLIError{Code: CodeUsage, Command: "logs.query", Message: "--kind is required"}
			}
			return runLogs(cmd, "logs.query", kind)
		},
	}
	addLogReadFlags(c, true)
	c.Flags().String("kind", "", t("日志种类：access、error、system、safe（必填）", "log stream: access, error, system, or safe (required)"))
	c.Flags().String("until", "", t("只看这个时长之前的记录，例如 10m", "only records older than this duration, e.g. 10m"))
	c.Flags().String("level", "", t("按 level 字段过滤", "filter on the level field"))
	c.Flags().String("method", "", t("按 method 字段过滤", "filter on the method field"))
	c.Flags().String("path-prefix", "", t("按 path 前缀过滤", "filter on a path prefix"))
	c.Flags().String("reason", "", t("按 reason 字段过滤", "filter on the reason field"))
	return c
}

func addLogReadFlags(c *cobra.Command, withStatus bool) {
	addConfigFlag(c)
	c.Flags().String("since", "", t("只看这个时长之内的记录，例如 1h、30m", "only records newer than this duration, e.g. 1h or 30m"))
	c.Flags().Int("limit", 0, t("返回条数，默认 100，超过 1000 时钳到 1000", "records to return; default 100, values above 1000 are capped"))
	c.Flags().String("domain", "", t("按 host 字段过滤", "filter on the host field"))
	if withStatus {
		c.Flags().String("status", "", t("状态码：500、4xx 或 400-499", "status code: 500, 4xx, or 400-499"))
	}
}

func runLogs(cmd *cobra.Command, id, kind string) error {
	q := logquery.Query{Kind: kind}
	q.Since, _ = cmd.Flags().GetString("since")
	q.Limit, _ = cmd.Flags().GetInt("limit")
	q.Domain, _ = cmd.Flags().GetString("domain")
	if cmd.Flags().Lookup("status") != nil {
		q.Status, _ = cmd.Flags().GetString("status")
	}
	if cmd.Flags().Lookup("until") != nil {
		q.Until, _ = cmd.Flags().GetString("until")
	}
	if cmd.Flags().Lookup("level") != nil {
		q.Level, _ = cmd.Flags().GetString("level")
	}
	if cmd.Flags().Lookup("method") != nil {
		q.Method, _ = cmd.Flags().GetString("method")
	}
	if cmd.Flags().Lookup("path-prefix") != nil {
		q.PathPrefix, _ = cmd.Flags().GetString("path-prefix")
	}
	if cmd.Flags().Lookup("reason") != nil {
		q.Reason, _ = cmd.Flags().GetString("reason")
	}

	configPath, _ := cmd.Flags().GetString("config")
	dir, warnings := logquery.ResolveLogDir(configPath)
	q.Dir = dir

	res, more, err := logquery.Run(q)
	if err != nil {
		return fromLogsError(id, err)
	}
	warnings = append(warnings, more...)
	if jsonMode(cmd) {
		return emitOK(cmd, id, res, warnings)
	}
	printWarnings(cmd.ErrOrStderr(), warnings)
	writeLogsText(cmd.OutOrStdout(), res)
	return nil
}

func fromLogsError(id string, err error) *CLIError {
	var src *logquery.SourceError
	if errors.As(err, &src) {
		return &CLIError{Code: CodeLogUnavailable, Command: id, Message: err.Error(), Cause: err}
	}
	var fe *logquery.FilterError
	if errors.As(err, &fe) {
		return &CLIError{Code: CodeUsage, Command: id, Message: err.Error(), Cause: err}
	}
	if os.IsPermission(err) {
		return &CLIError{Code: CodePermissionDenied, Command: id, Message: err.Error(), Cause: err}
	}
	return &CLIError{Code: CodeInternal, Command: id, Message: err.Error(), Cause: err}
}

func writeLogsText(w io.Writer, res logquery.Result) {
	fmt.Fprintf(w, "file: %s\nscanned_bytes: %d\nrecords: %d\n", res.Source.File, res.Source.ScannedBytes, len(res.Records))
	if res.Truncated {
		fmt.Fprintf(w, "truncated: true\n")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, rec := range res.Records {
		status := ""
		if rec.Status != nil {
			status = fmt.Sprintf("%d", *rec.Status)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", rec.Time, rec.Level, status, rec.Host, rec.Message)
	}
	_ = tw.Flush()
}
