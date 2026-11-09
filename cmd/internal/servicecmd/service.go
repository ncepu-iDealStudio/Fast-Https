package servicecmd

import (
	"context"
	"errors"
	"fast-https/cmd/internal/cli"
	"fast-https/cmd/internal/procctl"
	"fmt"

	"fast-https/config"
	"fast-https/management/operation"
	"fast-https/management/rtclient"
	"fast-https/management/svcctl"

	"github.com/spf13/cobra"
)

// service 命令组：服务本身的状态、版本与受控 reload。旧的顶层 start/stop/reload/status/version
// 保持原样，桌面端与 systemd 依赖它们的文本输出。
func NewServiceCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "service",
		Short:        cli.T("查询服务状态、版本与受控 reload", "query service status and version, and reload"),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.AddCommand(NewServiceVersionCmd(), NewServiceStatusCmd(), NewServiceReloadCmd())
	return root
}

func NewServiceReloadCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "reload",
		Short: cli.T("受控重新加载已生效配置", "reload the running configuration under management control"),
		Long: cli.T(
			"校验当前配置、预检新增端口，再向运行中进程发 reload 信号。\n"+
				"读得到 /runtime 时做强验证（pid 与 started_at 不变，config_generation 加一）；否则退回弱验证（进程仍在、端口可连）。\n"+
				"不改配置文件。服务没在跑时退出码 4。--dry-run 只校验与预检，不发信号。",
			"Validate the current config, pre-check newly added ports, then signal the running process to reload.\n"+
				"When /runtime is reachable, verification requires an unchanged pid/started_at and a bumped config_generation;\n"+
				"otherwise it falls back to a basic check (process still alive, ports reachable).\n"+
				"It does not change config files. A stopped service exits with code 4. --dry-run only validates and pre-checks."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runServiceReload,
	}
	c.Flags().Bool("dry-run", false, cli.T("只校验与预检，不发信号", "validate and pre-check only, do not signal"))
	cli.AddConfigFlag(c)
	return c
}

func runServiceReload(cmd *cobra.Command, _ []string) error {
	path, _ := cmd.Flags().GetString("config")
	dry, _ := cmd.Flags().GetBool("dry-run")
	rid := cli.EnsureRequestID(cmd)
	timeout := cli.CmdTimeout(cmd, svcctl.DefaultTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	st, pid, err := procctl.InspectProcess(config.PID_FILE)
	if err != nil {
		return cli.FromTxnError("service.reload", err)
	}

	args := map[string]string{"config": path, "dry-run": fmt.Sprintf("%v", dry)}
	store := operation.Store{Dir: cli.StateDir()}
	if rec, ok, err := store.Lookup(rid); err != nil {
		return cli.FromStateError("service.reload", err)
	} else if ok {
		if rec.ArgsHash != operation.HashArgs("service.reload", args) {
			return cli.FromStateError("service.reload", operation.ErrRequestIDConflict)
		}
		return cli.EmitReloadResult(cmd, rec, svcctl.Result{}, true)
	}

	if st != procctl.ProcessRunning {
		return &cli.CLIError{Code: cli.CodeServiceDown, Command: "service.reload", Message: "service is not running"}
	}

	opts := svcctl.Options{
		ConfigPath: path,
		PidPath:    config.PID_FILE,
		Timeout:    timeout,
		DryRun:     dry,
		PID:        pid,
		Snapshot: func(ctx context.Context) (rtclient.Snapshot, error) {
			return rtclient.Default().Get(ctx)
		},
	}
	if dry {
		res, err := svcctl.Reload(ctx, opts)
		if err != nil {
			return cli.FromTxnError("service.reload", err)
		}
		return cli.EmitReloadResult(cmd, operation.Record{}, res, false)
	}

	rec, err := store.Begin(operation.BeginInput{RequestID: rid, Command: "service.reload", Args: args, Phase: "started"})
	if err != nil {
		return cli.FromStateError("service.reload", err)
	}
	if rec.Status != operation.StatusRunning {
		return cli.EmitReloadResult(cmd, rec, svcctl.Result{}, true)
	}

	res, err := svcctl.Reload(ctx, opts)
	if err != nil {
		code, msg := operation.StatusFailed, err.Error()
		if errors.Is(err, svcctl.ErrTimeout) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code, msg = operation.StatusUnknown, err.Error()
		}
		rec, _ = store.Finish(rec, code, "reloaded", map[string]any{"config_state": "uncertain"}, &operation.ErrorBody{Message: msg})
		ce := cli.FromTxnError("service.reload", err)
		ce.Details = append([]cli.ErrorDetail{{Path: "operation_id", Message: rec.ID}}, ce.Details...)
		return ce
	}
	rec, err = store.Finish(rec, operation.StatusSucceeded, "committed", map[string]any{
		"verification": res.Verification,
		"pid":          res.PID,
	}, nil)
	if err != nil {
		return cli.FromStateError("service.reload", err)
	}
	return cli.EmitReloadResult(cmd, rec, res, false)
}
func NewServiceVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "version",
		Short:        cli.T("查询 CLI 与服务的版本", "query the CLI and server versions"),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cli.JSONMode(cmd) {
				fmt.Fprintf(cmd.OutOrStdout(), "fast-https %s\n", config.CURRENT_VERSION)
				return nil
			}
			var server any
			var warnings []string
			if snap, err := cli.FetchRuntime(cmd.Context()); err == nil && snap.Server.Version != "" {
				server = snap.Server.Version
			} else {
				server = nil
				warnings = []string{"server_version unavailable: runtime snapshot unreachable"}
			}
			return cli.EmitOK(cmd, "service.version", map[string]any{
				"cli_version":    config.CURRENT_VERSION,
				"server_version": server,
			}, warnings)
		},
	}
}
