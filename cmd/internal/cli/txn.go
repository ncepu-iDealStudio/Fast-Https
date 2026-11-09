package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"fast-https/cmd/internal/procctl"
	"fast-https/config"
	"fast-https/management/operation"
	"fast-https/management/pathguard"
	"fast-https/management/rtclient"
	"fast-https/management/siteops"
	"fast-https/management/svcctl"
	"fast-https/management/txn"
	"fast-https/management/view"

	"github.com/spf13/cobra"
)

// AddConfigFlag 给需要读配置树的命令加上 --config。
func AddConfigFlag(c *cobra.Command) {
	c.Flags().String("config", config.CONFIG_FILE_PATH, T("主配置文件", "main config file"))
}

// Confirmed 报告命令是否带了 --yes。
// 只有 add/update/delete 这类会改现有配置的命令需要它；disable/enable
// 只是改名，随时可逆，不再要求确认。
func Confirmed(cmd *cobra.Command) bool {
	yes, _ := cmd.Flags().GetBool("yes")
	return yes
}

// PrintWarnings 在文本模式下把告警写到 stderr，不污染 stdout。
func PrintWarnings(w io.Writer, warnings []string) {
	for _, msg := range warnings {
		fmt.Fprintf(w, "warning: %s\n", msg)
	}
}

// PrintIndentedJSON 以两空格缩进输出 JSON，用于 dry-run 的差异预览。
func PrintIndentedJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// BannedRoots 返回站点文档根目录不允许落进去的位置：
// 配置目录自身、证书目录、状态目录与日志根目录。
func BannedRoots(liveMain string) []string {
	return []string{
		filepath.Dir(liveMain),
		config.CERT_DIR,
		ResolveStateDir(),
		config.DEFAULT_LOG_ROOT,
	}
}

// Abs 把路径固定到当前工作目录，失败时原样返回。
func Abs(path string) string { return abs(path) }

// NewTxnEngine 构造配置事务引擎：状态目录来自 --state-dir，
// reload 与健康检查都按 pid 文件判断服务是否在跑。
func NewTxnEngine(liveMain string, timeout time.Duration) *txn.Engine {
	pidPath := config.PID_FILE
	var before *rtclient.Snapshot
	return &txn.Engine{
		State: StateDir(),
		Reloader: txn.FuncReloader(func(ctx context.Context) error {
			st, pid, err := procctl.InspectProcess(pidPath)
			if err != nil {
				return err
			}
			if st != procctl.ProcessRunning {
				// 服务没在跑也允许提交：配置已经写到候选并切换完成，
				// 下次启动会加载，没有 reload 可做。回滚反而是错的。
				fmt.Fprintln(os.Stderr, "提示：fast-https 服务未运行，配置已写入，启动后生效")
				return nil
			}
			before = nil
			if snap, err := rtclient.Default().Get(ctx); err == nil {
				cp := snap
				before = &cp
			}
			// Windows 上服务与 CLI 常常不在同一个控制台，Ctrl+Break /
			// SIGINT 投递不过去（syscall.EWINDOWS = "not supported by
			// windows"）。此时配置已经写好，不该因为发不出信号而回滚，
			// 提示用户手动重载即可。
			if err := svcctl.SignalReload(pid); err != nil {
				if runtime.GOOS == "windows" {
					fmt.Fprintln(os.Stderr,
						"提示：Windows 上无法向其他控制台的进程发送 reload 信号，配置已写入；"+
							"请在服务所在终端重启服务，或执行 fast-https reload")
					return nil
				}
				return err
			}
			return nil
		}),
		Health: txn.FuncHealth(func(ctx context.Context) error {
			st, pid, err := procctl.InspectProcess(pidPath)
			if err != nil {
				return err
			}
			if st != procctl.ProcessRunning {
				// 服务没在跑就没有健康检查可做，不构成失败。
				return nil
			}
			opts := svcctl.Options{
				ConfigPath: liveMain,
				Timeout:    timeout,
				PID:        pid,
				Snapshot: func(ctx context.Context) (rtclient.Snapshot, error) {
					return rtclient.Default().Get(ctx)
				},
			}
			if before != nil {
				_, err = svcctl.WaitRuntime(ctx, opts, *before)
				return err
			}
			_, err = svcctl.WaitBasic(ctx, opts, pid)
			return err
		}),
	}
}

// RunTxn 是站点写命令的公共入口：读 --config、按 --timeout 建上下文、
// 跑事务并输出结果。
func RunTxn(cmd *cobra.Command, id, summary string, args map[string]string, dry bool, mut func(string, []string) error) error {
	live, _ := cmd.Flags().GetString("config")
	timeout := CmdTimeout(cmd, svcctl.DefaultTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	eng := NewTxnEngine(live, timeout)
	res, err := siteops.Run(ctx, eng, siteops.RunInput{
		LiveMain:  live,
		RequestID: EnsureRequestID(cmd),
		Command:   id,
		Summary:   summary,
		DryRun:    dry,
		Args:      args,
		Mutate:    mut,
	})
	if err != nil {
		return FromSiteopsError(id, err)
	}
	return EmitTxnResult(cmd, id, res)
}

// FromSiteopsError 把 siteops 的错误转成带错误码的 CLIError。
func FromSiteopsError(id string, err error) *CLIError {
	var inv *view.InvalidError
	switch {
	case errors.Is(err, siteops.ErrUsage), errors.Is(err, pathguard.ErrPath):
		return &CLIError{Code: CodeUsage, Command: id, Message: err.Error(), Cause: err}
	case errors.Is(err, siteops.ErrConflict):
		return &CLIError{Code: CodeConflict, Command: id, Message: err.Error(), Cause: err}
	case errors.Is(err, siteops.ErrNotFound):
		return &CLIError{Code: CodeNotFound, Command: id, Message: err.Error(), Cause: err}
	case errors.Is(err, siteops.ErrNotSupported):
		return &CLIError{Code: CodeNotSupported, Command: id, Message: err.Error(), Cause: err}
	case errors.As(err, &inv):
		return &CLIError{Code: CodeConfigInvalid, Command: id, Message: err.Error(), Cause: err}
	default:
		return FromTxnError(id, err)
	}
}

// EmitTxnResult 按 --json 与是否 dry-run 输出事务结果。
func EmitTxnResult(cmd *cobra.Command, id string, res txn.Result) error {
	data := map[string]any{
		"dry_run":       res.DryRun,
		"idempotent":    res.Idempotent,
		"identical":     res.Diff.Identical,
		"changes":       res.Diff.Changes,
		"restore_point": res.RestorePoint,
		"version":       res.Version,
		"rolled_back":   res.RolledBack,
		"config_state":  res.ConfigState,
	}
	if res.Operation.ID != "" {
		data["operation_id"] = res.Operation.ID
		data["status"] = res.Operation.Status
		data["result"] = res.Operation.Result
	}
	if JSONMode(cmd) {
		return EmitOK(cmd, id, data, res.Warnings)
	}
	PrintWarnings(cmd.ErrOrStderr(), res.Warnings)
	if res.DryRun {
		if res.Diff.Identical {
			fmt.Fprintln(cmd.OutOrStdout(), "dry-run: identical")
			return nil
		}
		return PrintIndentedJSON(cmd.OutOrStdout(), res.Diff)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s restore=%s version=%s\n",
		res.Operation.ID, res.Operation.Status, res.RestorePoint, res.Version)
	return nil
}

// EmitReloadResult 输出 reload 的结果。
func EmitReloadResult(cmd *cobra.Command, rec operation.Record, res svcctl.Result, idem bool) error {
	data := map[string]any{
		"dry_run":      res.DryRun,
		"idempotent":   idem,
		"verification": res.Verification,
	}
	if rec.ID != "" {
		data["operation_id"] = rec.ID
		data["status"] = rec.Status
		data["result"] = rec.Result
	}
	if res.PID != 0 {
		data["pid"] = res.PID
	}
	if JSONMode(cmd) {
		return EmitOK(cmd, "service.reload", data, nil)
	}
	if rec.ID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "operation %s %s\n", rec.ID, rec.Status)
	} else if res.DryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "reload dry-run: ok")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "reload: ok")
	}
	return nil
}
