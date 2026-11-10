package cmd

import (
	"fmt"

	"fast-https/config"
	"fast-https/management/operation"
	"fast-https/modules/proxy"

	"github.com/spf13/cobra"
)

func newUpstreamCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "upstream",
		Short:        t("临时摘除或恢复一个上游地址", "drain or restore one upstream address"),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.AddCommand(newUpstreamDrainCmd(true), newUpstreamDrainCmd(false))
	return root
}

func newUpstreamDrainCmd(drain bool) *cobra.Command {
	use := "undrain <address>"
	short := t("恢复一个被摘除的上游地址", "restore a drained upstream address")
	commandID := "upstream.undrain"
	if drain {
		use = "drain <address>"
		short = t("临时摘除一个上游地址", "temporarily drain one upstream address")
		commandID = "upstream.drain"
	}
	c := &cobra.Command{
		Use:          use,
		Short:        short,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpstreamDrain(cmd, commandID, drain, args[0])
		},
	}
	addConfigFlag(c)
	c.Flags().Bool("yes", false, t("确认修改摘除名单", "confirm the drain list change"))
	c.Flags().String("request-id", "", t("幂等请求标识", "idempotency request id"))
	return c
}

func runUpstreamDrain(cmd *cobra.Command, commandID string, drain bool, addr string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return usageError(fmt.Errorf("--yes is required"))
	}
	host := proxyHost(addr)
	if host == "" {
		return &CLIError{Command: commandID, Code: CodeNotFound, Message: "upstream address is not usable"}
	}
	path, _ := cmd.Flags().GetString("config")
	if path == "" {
		path = config.CONFIG_FILE_PATH
	}
	groups, err := config.ProxyPeerGroups(path)
	if err != nil {
		return &CLIError{Command: commandID, Code: CodeConfigInvalid, Message: err.Error(), Cause: err}
	}
	current := proxy.DrainedList()
	next := append([]string{}, current...)
	if drain {
		if err := proxy.EvaluateDrain(groups, current, host); err != nil {
			code := CodeNotFound
			if err == proxy.ErrLastUpstream {
				code = CodeOperationFailed
			}
			return &CLIError{Command: commandID, Code: code, Message: err.Error(), Cause: err}
		}
		next = append(next, host)
	} else {
		kept := next[:0]
		for _, item := range next {
			if item != host {
				kept = append(kept, item)
			}
		}
		next = kept
	}
	rid, _ := cmd.Flags().GetString("request-id")
	store := operation.Store{Dir: stateDir()}
	rec, err := store.Begin(operation.BeginInput{
		RequestID: rid,
		Command:   commandID,
		Args:      map[string]string{"address": host},
		Phase:     "started",
	})
	if err != nil {
		return fromStateError(commandID, err)
	}
	if rec.Status == operation.StatusSucceeded {
		return emitOK(cmd, commandID, map[string]any{"idempotent": true, "address": host, "operation_id": rec.ID}, nil)
	}
	if err := proxy.SaveDrained(next); err != nil {
		_, _ = store.Finish(rec, operation.StatusFailed, "failed", nil, &operation.ErrorBody{Message: err.Error()})
		return &CLIError{Command: commandID, Code: CodeOperationFailed, Message: err.Error(), Cause: err}
	}
	rec, err = store.Finish(rec, operation.StatusSucceeded, "committed", map[string]any{"address": host}, nil)
	if err != nil {
		return fromStateError(commandID, err)
	}
	if jsonMode(cmd) {
		return emitOK(cmd, commandID, map[string]any{"address": host, "operation_id": rec.ID, "drained": proxy.DrainedList()}, nil)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", commandID, host)
	return nil
}

func proxyHost(addr string) string {
	// 与快照使用同一套 host:port 规则。空协议时补上，便于 metric 函数识别。
	if addr == "" {
		return ""
	}
	return proxy.NormalizePeer(addr)
}
