package cmd

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fast-https/config"
	"fast-https/management/agentview"
	"fast-https/management/operation"
	"fast-https/modules/proxy"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

func newAgentApproveCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "approve <plan-id>",
		Short: t("批准并执行一条摘除或恢复计划", "approve and run one drain or undrain plan"),
		Long: t(
			"只执行计划里已经写好的一条 upstream drain 或 upstream undrain。\n参数哈希不一致、计划不是待审批，或会摘掉最后一个地址时，不改名单。",
			"Runs only the single upstream drain or undrain step already stored in the plan.\nA hash mismatch, a plan that is not pending, or draining the last address leaves the list unchanged."),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentApprove(cmd, args[0])
		},
	}
	addConfigFlag(c)
	c.Flags().Bool("yes", false, t("确认执行该计划", "confirm execution of this plan"))
	return c
}

func runAgentApprove(cmd *cobra.Command, planID string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return usageError(fmt.Errorf("--yes is required"))
	}
	dir, _ := cmd.Flags().GetString("state-dir")
	dir = agentview.ResolveDir(dir)
	dbPath := filepath.Join(dir, "agent.db")
	if _, err := os.Stat(dbPath); err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeNotFound, Message: "agent state not found", Cause: err}
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
	}
	defer db.Close()

	var eventID, storedHash, stepsRaw, approvalStatus, taskID string
	err = db.QueryRow(`
SELECT p.event_id, p.plan_hash, p.steps_json, a.status, p.task_id
FROM agent_plan p JOIN agent_approval a ON a.plan_id = p.plan_id
WHERE p.plan_id = ?`, planID).Scan(&eventID, &storedHash, &stepsRaw, &approvalStatus, &taskID)
	if err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeNotFound, Message: "plan not found: " + err.Error(), Cause: err}
	}
	if approvalStatus != "pending" {
		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is not pending"}
	}
	var steps []string
	if json.Unmarshal([]byte(stepsRaw), &steps) != nil {
		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "plan steps are not a string list"}
	}
	if planHash(eventID, steps) != storedHash {
		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan hash does not match the stored steps"}
	}
	action, host, ok := parseDrainStep(steps)
	if !ok {
		return &CLIError{Command: "agent.approve", Code: CodeNotSupported, Message: "only one upstream drain or undrain step can be approved"}
	}

	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath == "" {
		cfgPath = config.CONFIG_FILE_PATH
	}
	groups, err := config.ProxyPeerGroups(cfgPath)
	if err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeConfigInvalid, Message: err.Error(), Cause: err}
	}
	current := proxy.DrainedList()
	next := append([]string{}, current...)
	if action == "drain" {
		if err := proxy.EvaluateDrain(groups, current, host); err != nil {
			code := CodeNotFound
			if err == proxy.ErrLastUpstream {
				code = CodeOperationFailed
			}
			return &CLIError{Command: "agent.approve", Code: code, Message: err.Error(), Cause: err}
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
	if err := proxy.SaveDrained(next); err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: err.Error(), Cause: err}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE agent_approval SET status = 'approved' WHERE plan_id = ?`, planID); err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
	}
	_, _ = db.Exec(`UPDATE agent_task SET status = 'COMPLETED', updated_at = ? WHERE task_id = ?`, now, taskID)
	store := operation.Store{Dir: stateDir()}
	rec, err := store.Begin(operation.BeginInput{
		Command: "agent.approve",
		Args:    map[string]string{"plan_id": planID, "step": action + " " + host},
		Phase:   "started",
	})
	if err == nil && rec.Status == operation.StatusRunning {
		_, _ = store.Finish(rec, operation.StatusSucceeded, "committed", map[string]any{"address": host, "plan_id": planID}, nil)
	}
	if jsonMode(cmd) {
		return emitOK(cmd, "agent.approve", map[string]any{"plan_id": planID, "address": host, "action": action}, nil)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "approved %s %s\n", action, host)
	return nil
}

func planHash(eventID string, steps []string) string {
	if steps == nil {
		steps = []string{}
	}
	raw, _ := json.Marshal(steps)
	sum := sha256.Sum256(append([]byte(eventID+"\n"), raw...))
	return hex.EncodeToString(sum[:])
}

func parseDrainStep(steps []string) (action, host string, ok bool) {
	if len(steps) != 1 {
		return "", "", false
	}
	fields := strings.Fields(steps[0])
	if len(fields) != 3 || fields[0] != "upstream" {
		return "", "", false
	}
	if fields[1] != "drain" && fields[1] != "undrain" {
		return "", "", false
	}
	host = proxy.NormalizePeer(fields[2])
	if host == "" || strings.ContainsAny(fields[2], ";&|$`'\"\\") {
		return "", "", false
	}
	return fields[1], host, true
}
