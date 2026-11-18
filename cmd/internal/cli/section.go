package cli

import (
	"context"
	"path/filepath"
	"time"

	"fast-https/management/siteops"
	"fast-https/management/svcctl"
	"fast-https/management/txn"

	"github.com/spf13/cobra"
)

// SectionTxnInput 是「只改主配置某个段」的写操作。
type SectionTxnInput struct {
	LiveMain  string
	RequestID string
	ID        string
	Summary   string
	Timeout   time.Duration
	Args      map[string]string
	// Mutate 在候选主配置上做局部改写。
	Mutate func(candMain string) error
}

// RunSectionTxn 把「只改主配置某个段」的写命令送进配置事务。
//
// 候选树是线上树的副本，Mutate 在候选主配置上做局部改写；校验、恢复点、
// 原子切换与 operation 记录全部由事务引擎负责，与 site.* 走同一条链路。
// 这些命令的语义是「下次 reload 或启动时生效」，所以 reload 与健康检查
// 在这里是空操作；站点命令仍然自动 reload。
func RunSectionTxn(in SectionTxnInput) (txn.Result, error) {
	timeout := in.Timeout
	if timeout <= 0 {
		timeout = svcctl.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	eng := NewTxnEngine(in.LiveMain, timeout)
	eng.Reloader = txn.FuncReloader(func(context.Context) error { return nil })
	eng.Health = txn.FuncHealth(func(context.Context) error { return nil })

	res, err := siteops.Run(ctx, eng, siteops.RunInput{
		LiveMain:  in.LiveMain,
		RequestID: in.RequestID,
		Command:   in.ID,
		Summary:   in.Summary,
		Args:      in.Args,
		Mutate: func(candRoot string, _ []string) error {
			return in.Mutate(filepath.Join(candRoot, "fast-https.json"))
		},
	})
	if err != nil {
		return txn.Result{}, FromSiteopsError(in.ID, err)
	}
	return res, nil
}

// RunSectionTxnCmd 从命令的 --config / --timeout / --request-id 读取事务参数。
func RunSectionTxnCmd(cmd *cobra.Command, id, summary string, args map[string]string, mutate func(candMain string) error) (txn.Result, error) {
	live, _ := cmd.Flags().GetString("config")
	return RunSectionTxn(SectionTxnInput{
		LiveMain:  live,
		RequestID: EnsureRequestID(cmd),
		ID:        id,
		Summary:   summary,
		Timeout:   CmdTimeout(cmd, svcctl.DefaultTimeout),
		Args:      args,
		Mutate:    mutate,
	})
}
