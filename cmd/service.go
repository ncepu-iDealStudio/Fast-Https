package cmd

import (
	"fast-https/config"
	"fmt"

	"github.com/spf13/cobra"
)

// service 命令组：服务本身的状态与版本。旧的顶层 start/stop/reload/status/version
// 保持原样，桌面端与 systemd 依赖它们的文本输出。
func newServiceCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "service",
		Short:        t("查询服务状态与版本", "query service status and version"),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.AddCommand(newServiceVersionCmd())
	return root
}

func newServiceVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "version",
		Short:        t("查询 CLI 与服务的版本", "query the CLI and server versions"),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !jsonMode(cmd) {
				fmt.Fprintf(cmd.OutOrStdout(), "fast-https %s\n", config.CURRENT_VERSION)
				return nil
			}
			// 运行中进程的版本要等 /runtime 才能取到，现在如实标为不可用。
			return emitOK(cmd, "service.version", map[string]any{
				"cli_version":    config.CURRENT_VERSION,
				"server_version": nil,
			}, []string{"server_version unavailable: the running server does not expose it yet"})
		},
	}
}
