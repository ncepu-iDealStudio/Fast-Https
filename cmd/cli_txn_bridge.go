package cmd

import (
	"io"
	"time"

	"fast-https/cmd/internal/cli"
	"fast-https/management/operation"
	"fast-https/management/svcctl"
	"fast-https/management/txn"

	"github.com/spf13/cobra"
)

// 配置事务编排与结果输出的实现已迁到 cmd/internal/cli/txn.go。
// 这里保留 cmd 包内的旧名字；领域包迁完后整体删除。

func addConfigFlag(c *cobra.Command) { cli.AddConfigFlag(c) }

func confirmed(cmd *cobra.Command) bool { return cli.Confirmed(cmd) }

func bannedRoots(liveMain string) []string { return cli.BannedRoots(liveMain) }

func runSiteTxn(cmd *cobra.Command, id, summary string, args map[string]string, dry bool, mut func(string, []string) error) error {
	return cli.RunTxn(cmd, id, summary, args, dry, mut)
}

func fromSiteopsError(id string, err error) *CLIError { return cli.FromSiteopsError(id, err) }

func emitTxnResult(cmd *cobra.Command, id string, res txn.Result) error {
	return cli.EmitTxnResult(cmd, id, res)
}

func emitReloadResult(cmd *cobra.Command, rec operation.Record, res svcctl.Result, idem bool) error {
	return cli.EmitReloadResult(cmd, rec, res, idem)
}

func printWarnings(w io.Writer, warnings []string) { cli.PrintWarnings(w, warnings) }

func printIndentedJSON(w io.Writer, v any) error { return cli.PrintIndentedJSON(w, v) }

func newTxnEngine(liveMain string, timeout time.Duration) *txn.Engine {
	return cli.NewTxnEngine(liveMain, timeout)
}
