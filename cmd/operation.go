package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"fast-https/management/operation"
	"fast-https/management/state"

	"github.com/spf13/cobra"
)

func newOperationCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "operation",
		Short: t("查询管理操作记录", "query management operations"),
		Long: t(
			"查询落在状态目录里的管理操作。状态由服务端持久化，SSH 断线后仍可按 operation_id 查询。\n"+
				"running 且持有者进程已不在时，会把记录对账为 unknown，并保留最后完成的 phase。",
			"Query management operations stored in the state directory. Status is persisted on disk,\n"+
				"so a caller can look up an operation_id after an SSH disconnect.\n"+
				"A running record whose owner process is gone is reconciled to unknown, keeping the last phase."),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.AddCommand(newOperationListCmd(), newOperationGetCmd())
	return root
}

func newOperationListCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "list",
		Short: t("列出近期管理操作", "list recent management operations"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, _ := cmd.Flags().GetInt("limit")
			status, _ := cmd.Flags().GetString("status")
			since, _ := cmd.Flags().GetString("since")
			var sinceAt time.Time
			if strings.TrimSpace(since) != "" {
				d, err := time.ParseDuration(since)
				if err != nil {
					return usageError(fmt.Errorf("invalid --since %q (want 1h, 30m)", since))
				}
				sinceAt = time.Now().Add(-d)
			}
			if status != "" && !validOpStatus(status) {
				return usageError(fmt.Errorf("invalid --status %q", status))
			}
			store := operation.Store{Dir: stateDir()}
			list, err := store.List(operation.ListFilter{Status: status, Since: sinceAt, Limit: limit})
			if err != nil {
				return fromStateError("operation.list", err)
			}
			if jsonMode(cmd) {
				return emitOK(cmd, "operation.list", map[string]any{"operations": list}, nil)
			}
			writeOperationListText(cmd.OutOrStdout(), list)
			return nil
		},
	}
	c.Flags().Int("limit", 0, t("返回条数，默认 50，上限 1000", "records to return; default 50, cap 1000"))
	c.Flags().String("status", "", t("按状态过滤", "filter by status"))
	c.Flags().String("since", "", t("只看这个时长之内的记录，例如 1h", "only records newer than this duration, e.g. 1h"))
	return c
}

func newOperationGetCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "get <operation-id>",
		Short: t("查询一条管理操作", "get one management operation"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := operation.Store{Dir: stateDir()}
			rec, err := store.Get(args[0])
			if err != nil {
				return fromStateError("operation.get", err)
			}
			if jsonMode(cmd) {
				return emitOK(cmd, "operation.get", rec, nil)
			}
			writeOperationListText(cmd.OutOrStdout(), []operation.Record{rec})
			return nil
		},
	}
	return c
}

func validOpStatus(s string) bool {
	switch s {
	case operation.StatusQueued, operation.StatusRunning, operation.StatusSucceeded,
		operation.StatusFailed, operation.StatusCancelled, operation.StatusUnknown,
		operation.StatusRecoveryRequired:
		return true
	}
	return false
}

func fromStateError(id string, err error) *CLIError {
	switch {
	case errors.Is(err, operation.ErrNotFound):
		return &CLIError{Code: CodeNotFound, Command: id, Message: err.Error(), Cause: err}
	case errors.Is(err, operation.ErrRequestIDConflict):
		return &CLIError{Code: CodeRequestIDClash, Command: id, Message: err.Error(), Cause: err}
	case errors.Is(err, state.ErrLocked):
		return &CLIError{Code: CodeConflict, Command: id, Message: err.Error(), Cause: err}
	default:
		return &CLIError{Code: CodeInternal, Command: id, Message: err.Error(), Cause: err}
	}
}

func writeOperationListText(w io.Writer, list []operation.Record) {
	if len(list) == 0 {
		fmt.Fprintln(w, "(no operations)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tCOMMAND\tCREATED")
	for _, rec := range list {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", rec.ID, rec.Status, rec.Command, rec.CreatedAt)
	}
	_ = tw.Flush()
}
