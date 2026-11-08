package cmd

import (
	"fmt"
	"io"
	"os"

	"fast-https/infra/logging"

	"github.com/spf13/cobra"
)

// registerGlobalFlags 给根命令加上规范要求的全局选项。
// --timeout 与 --request-id 只是被解析，具体含义由各命令自己使用。
func registerGlobalFlags(root *cobra.Command) {
	f := root.PersistentFlags()
	f.Bool("json", false, t("以 JSON 输出（stdout 只有一个 JSON 文档，日志走 stderr）",
		"print JSON (stdout carries exactly one JSON document, logs go to stderr)"))
	f.Duration("timeout", 0, t("命令超时时间，例如 30s（0 表示用命令自己的默认值）",
		"command timeout, e.g. 30s (0 means the command default)"))
	f.String("request-id", "", t("调用方提供的幂等请求标识（修改类命令使用）",
		"caller-supplied idempotency key (used by commands that change state)"))

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if jsonMode(cmd) {
			// 配置加载等路径会写控制台日志；不挪走的话 stdout 里就混进了日志行。
			logging.SetConsoleOutput(cmd.ErrOrStderr())
		}
		if d, _ := cmd.Flags().GetDuration("timeout"); d < 0 {
			return usageError(fmt.Errorf("--timeout must not be negative"))
		}
		return nil
	}

	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError(err)
	})
}

// wrapArgsErrors 让位置参数校验失败也成为 USAGE_ERROR（退出码 2）。
// 根命令带位置参数时 cobra 自己报 unknown command，同样归入用法错误。
func wrapArgsErrors(root *cobra.Command) {
	walkCommands(root, func(c *cobra.Command) {
		switch {
		case c.Args != nil:
			orig := c.Args
			c.Args = func(cmd *cobra.Command, args []string) error {
				if err := orig(cmd, args); err != nil {
					return usageError(err)
				}
				return nil
			}
		case !c.HasParent() && c.HasSubCommands():
			c.Args = func(cmd *cobra.Command, args []string) error {
				if len(args) > 0 {
					return usageError(fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath()))
				}
				return nil
			}
		}
	})
}

// Run 执行根命令并返回进程退出码，供 main 使用。
func Run() int {
	return runWith(RootCmd(), os.Args[1:], os.Stdout, os.Stderr)
}

func runWith(root *cobra.Command, args []string, out, errOut io.Writer) int {
	// 错误统一由 reportError 输出：旧命令仍是 "Error: ..." 到 stderr，
	// --json 时是 stdout 上的错误封套。
	root.SilenceErrors = true
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		return reportError(args, err, out, errOut)
	}
	return 0
}
