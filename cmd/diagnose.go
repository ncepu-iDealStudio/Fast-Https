package cmd

import (
	"fmt"
	"io"
	"text/tabwriter"

	"fast-https/config"
	"fast-https/management/diagnose"
	"fast-https/management/view"

	"github.com/spf13/cobra"
)

func newDiagnoseCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "diagnose",
		Short: t("只读汇总健康检查", "run a read-only health summary"),
		Long: t(
			"汇总服务、配置、证书、端口和近期错误日志等只读检查。\n"+
				"检查失败与检查不可用分开：runtime 读 /runtime（连不上是 warning）；monitor 读状态文件，未安装为 ok，过期为 warning。\n"+
				"诊断本身是一次成功的查询，配置有问题也退出码 0；不自动修复，不写报告文件，不输出敏感值。",
			"Summarize read-only checks for the service, config, certificates, ports, and recent error logs.\n"+
				"Failed checks and unavailable checks are distinct: runtime reads /runtime (unreachable is a warning);\n"+
				"monitor reads its status file (not installed is ok, a stale file is a warning).\n"+
				"The command itself is a successful query and exits 0 even when the config is invalid;\n"+
				"it never repairs, never writes a report file, and never prints secrets."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			report := diagnose.Run(diagnose.Input{
				ConfigPath: path,
				PidFile:    config.PID_FILE,
				StateDir:   resolveStateDir(),
				Inspect: func() (view.ProcState, int, error) {
					state, pid, err := InspectProcess(config.PID_FILE)
					return procStateOf(state), pid, err
				},
			})
			if jsonMode(cmd) {
				return emitOK(cmd, "diagnose", report, nil)
			}
			writeDiagnoseText(cmd.OutOrStdout(), report)
			return nil
		},
	}
	addConfigFlag(c)
	return c
}

func writeDiagnoseText(w io.Writer, report diagnose.Report) {
	fmt.Fprintf(w, "overall: %s\nreport_id: %s\n", report.Overall, report.ReportID)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, c := range report.Checks {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name, c.Status, c.Message)
	}
	_ = tw.Flush()
}
