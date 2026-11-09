package cmd

import (
	"io"
	"time"

	"fast-https/cmd/internal/cli"
	"fast-https/management/state"

	"github.com/spf13/cobra"
)

// 输出、全局 flag、运行入口与状态目录的实现已迁到 cmd/internal/cli。
// 这里保留 cmd 包内的旧名字，让几十个调用点不必与迁移同步改动；
// 领域包全部迁完后整体删除。

func jsonMode(cmd *cobra.Command) bool { return cli.JSONMode(cmd) }

func ensureRequestID(cmd *cobra.Command) string { return cli.EnsureRequestID(cmd) }

func cmdTimeout(cmd *cobra.Command, def time.Duration) time.Duration {
	return cli.CmdTimeout(cmd, def)
}

func emitOK(cmd *cobra.Command, id string, data any, warnings []string) error {
	return cli.EmitOK(cmd, id, data, warnings)
}

func reportError(args []string, err error, out, errOut io.Writer) int {
	return cli.ReportError(args, err, out, errOut)
}

func registerGlobalFlags(root *cobra.Command) { cli.RegisterGlobalFlags(root) }

func wrapArgsErrors(root *cobra.Command) { cli.WrapArgsErrors(root) }

// Run 执行根命令并返回进程退出码，供 main 使用。
func Run() int { return cli.Run(RootCmd) }

func runWith(root *cobra.Command, args []string, out, errOut io.Writer) int {
	return cli.RunWith(root, args, out, errOut)
}

func resolveStateDir() string { return cli.ResolveStateDir() }

func stateDir() state.Dir { return cli.StateDir() }
