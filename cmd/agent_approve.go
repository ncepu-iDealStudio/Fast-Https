package cmd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fast-https/config"
	"fast-https/management/agentview"
	"fast-https/management/operation"
	"fast-https/management/rtclient"
	"fast-https/modules/proxy"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

// 与 agent/plan.go 的常量保持一致。cmd 不能引用 Agent 进程。
const (
	approveVerify    = "重新读取 /runtime 与 upstream 名单，不自动执行"
	approveOnFailure = "停止自动尝试并保留待审批记录"
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

type approvePlan struct {
	eventID string
	hash    string
	steps   []string
	status  string
	taskID  string
	risk    string
	verify  string
	failure string
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
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
	}
	if err := ensureApprovalColumns(db); err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
	}
	plan, err := loadApprovePlan(db, planID)
	if err != nil {
		return err
	}
	if planHash(plan.eventID, plan.risk, plan.verify, plan.failure, plan.steps) != plan.hash {
		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan hash does not match the stored steps"}
	}
	action, host, ok := parseDrainStep(plan.steps)
	if !ok {
		return &CLIError{Command: "agent.approve", Code: CodeNotSupported, Message: "only one upstream drain or undrain step can be approved"}
	}
	switch plan.status {
	case "approved", "rejected", "expired":
		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is not pending"}
	case "executing":
		return finishExecutingApproval(cmd, db, planID, plan.taskID, action, host)
	case "pending":
	default:
		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is not pending"}
	}

	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath == "" {
		cfgPath = config.CONFIG_FILE_PATH
	}
	if err := recheckDrain(action, host, cfgPath); err != nil {
		return err
	}
	claimed, err := claimApproval(db, planID)
	if err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
	}
	if !claimed {
		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is already being approved"}
	}
	return executeClaimedApproval(cmd, db, planID, plan.taskID, action, host, cfgPath)
}

func loadApprovePlan(db *sql.DB, planID string) (approvePlan, error) {
	var plan approvePlan
	var stepsRaw string
	err := db.QueryRow(`
SELECT p.event_id, p.plan_hash, p.steps_json, a.status, p.task_id,
       IFNULL(p.risk, ''), IFNULL(p.verify_text, ''), IFNULL(p.on_failure, '')
FROM agent_plan p JOIN agent_approval a ON a.plan_id = p.plan_id
WHERE p.plan_id = ?`, planID).Scan(
		&plan.eventID, &plan.hash, &stepsRaw, &plan.status, &plan.taskID, &plan.risk, &plan.verify, &plan.failure)
	if err != nil {
		return approvePlan{}, &CLIError{Command: "agent.approve", Code: CodeNotFound, Message: "plan not found: " + err.Error(), Cause: err}
	}
	if json.Unmarshal([]byte(stepsRaw), &plan.steps) != nil {
		return approvePlan{}, &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "plan steps are not a string list"}
	}
	return plan, nil
}

func recheckDrain(action, host, cfgPath string) error {
	if action != "drain" {
		groups, err := config.ProxyPeerGroups(cfgPath)
		if err != nil {
			return &CLIError{Command: "agent.approve", Code: CodeConfigInvalid, Message: err.Error(), Cause: err}
		}
		if !peerKnown(groups, host) {
			return &CLIError{Command: "agent.approve", Code: CodeNotFound, Message: "upstream address is not in the configuration"}
		}
		return nil
	}
	groups, err := config.ProxyPeerGroups(cfgPath)
	if err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeConfigInvalid, Message: err.Error(), Cause: err}
	}
	if err := proxy.EvaluateDrain(groups, proxy.DrainedList(), host); err != nil {
		code := CodeNotFound
		if err == proxy.ErrLastUpstream {
			code = CodeOperationFailed
		}
		return &CLIError{Command: "agent.approve", Code: code, Message: err.Error(), Cause: err}
	}
	rows, err := currentUpstreams()
	if err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeServiceDown, Message: "runtime snapshot is required before drain", Cause: err}
	}
	if err := drainStillAllowed(rows, host); err != nil {
		code := CodeOperationFailed
		if errors.Is(err, proxy.ErrUpstreamNotFound) {
			code = CodeNotFound
		}
		return &CLIError{Command: "agent.approve", Code: code, Message: err.Error(), Cause: err}
	}
	return nil
}

func peerKnown(groups [][]string, host string) bool {
	for _, group := range groups {
		for _, peer := range group {
			if proxy.NormalizePeer(peer) == host {
				return true
			}
		}
	}
	return false
}

func currentUpstreams() ([]rtclient.UpstreamAddrInfo, error) {
	snap, err := rtclient.Default().Get(context.Background())
	if err != nil {
		return nil, err
	}
	if snap.Upstream == nil {
		return nil, errors.New("runtime snapshot has no upstream section")
	}
	return snap.Upstream.Upstreams, nil
}

func drainStillAllowed(rows []rtclient.UpstreamAddrInfo, host string) error {
	seen := false
	state := ""
	drained := false
	healthy := 0
	for _, row := range rows {
		kind := row.Health
		if kind == "" {
			if row.Healthy {
				kind = "healthy"
			} else {
				kind = "unhealthy"
			}
		}
		if row.Address == host {
			seen = true
			state = kind
			drained = row.Drained
		}
		if kind == "healthy" && row.Address != host && !row.Drained {
			healthy++
		}
	}
	if !seen {
		return proxy.ErrUpstreamNotFound
	}
	if drained {
		return nil
	}
	if state != "unhealthy" {
		return errors.New("upstream is not currently unhealthy")
	}
	if healthy < 1 {
		return errors.New("no other healthy upstream")
	}
	return nil
}

func claimApproval(db *sql.DB, planID string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`
UPDATE agent_approval SET status = 'executing', actor = 'cli', decided_at = ?
WHERE plan_id = ? AND status = 'pending'`, now, planID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func executeClaimedApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host, cfgPath string) error {
	store := operation.Store{Dir: stateDir()}
	rec, err := store.Begin(operation.BeginInput{
		RequestID: "approve:" + planID,
		Command:   "agent.approve",
		Args:      map[string]string{"plan_id": planID, "action": action, "address": host},
		Phase:     "prepared",
	})
	if err != nil {
		_, _ = db.Exec(`UPDATE agent_approval SET status = 'pending' WHERE plan_id = ? AND status = 'executing'`, planID)
		return fromStateError("agent.approve", err)
	}
	if rec.Status == operation.StatusSucceeded {
		return completeApproval(cmd, db, planID, taskID, action, host, true)
	}
	if alreadyInState(action, host) {
		_, _ = store.Finish(rec, operation.StatusSucceeded, "already", map[string]any{"address": host, "plan_id": planID, "applied": false}, nil)
		return completeApproval(cmd, db, planID, taskID, action, host, true)
	}
	if err := applyApprovedDrain(action, host, cfgPath); err != nil {
		_, _ = store.Finish(rec, operation.StatusFailed, "failed", nil, &operation.ErrorBody{Message: "drain change failed"})
		_, _ = db.Exec(`UPDATE agent_approval SET status = 'pending' WHERE plan_id = ? AND status = 'executing'`, planID)
		return err
	}
	if _, err := store.Finish(rec, operation.StatusSucceeded, "committed", map[string]any{"address": host, "plan_id": planID, "applied": true}, nil); err != nil {
		return fromStateError("agent.approve", err)
	}
	return completeApproval(cmd, db, planID, taskID, action, host, false)
}

func finishExecutingApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host string) error {
	if !alreadyInState(action, host) {
		now := time.Now().UTC().Format(time.RFC3339)
		_, _ = db.Exec(`UPDATE agent_task SET status = 'WAITING_HUMAN', updated_at = ? WHERE task_id = ?`, now, taskID)
		store := operation.Store{Dir: stateDir()}
		if rec, found, err := store.Lookup("approve:" + planID); err == nil && found && rec.Status == operation.StatusRunning {
			_, _ = store.Finish(rec, operation.StatusUnknown, "unconfirmed", nil, &operation.ErrorBody{Message: "result not confirmed"})
		}
		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "approval is executing and the upstream state is not confirmed"}
	}
	return completeApproval(cmd, db, planID, taskID, action, host, true)
}

func alreadyInState(action, host string) bool {
	found := false
	for _, item := range proxy.DrainedList() {
		if item == host {
			found = true
			break
		}
	}
	if action == "drain" {
		return found
	}
	return !found
}

func applyApprovedDrain(action, host, cfgPath string) error {
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
	return nil
}

func completeApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host string, idempotent bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`UPDATE agent_approval SET status = 'approved' WHERE plan_id = ? AND status = 'executing'`, planID)
	if err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
	}
	if n, _ := res.RowsAffected(); n != 1 {
		var status string
		_ = db.QueryRow(`SELECT status FROM agent_approval WHERE plan_id = ?`, planID).Scan(&status)
		if status != "approved" {
			return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is not pending"}
		}
	}
	if _, err := db.Exec(`UPDATE agent_task SET status = 'COMPLETED', updated_at = ? WHERE task_id = ?`, now, taskID); err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
	}
	data := map[string]any{"plan_id": planID, "address": host, "action": action, "idempotent": idempotent}
	if jsonMode(cmd) {
		return emitOK(cmd, "agent.approve", data, nil)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "approved %s %s\n", action, host)
	return nil
}

func ensureApprovalColumns(db *sql.DB) error {
	for _, col := range []struct{ table, name, decl string }{
		{"agent_plan", "verify_text", "TEXT"},
		{"agent_plan", "on_failure", "TEXT"},
		{"agent_approval", "decided_at", "TEXT"},
		{"agent_approval", "actor", "TEXT"},
	} {
		rows, err := db.Query(`PRAGMA table_info(` + col.table + `)`)
		if err != nil {
			return err
		}
		found := false
		for rows.Next() {
			var cid, notNull, pk int
			var name, typ string
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
				rows.Close()
				return err
			}
			if name == col.name {
				found = true
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if found {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE ` + col.table + ` ADD COLUMN ` + col.name + ` ` + col.decl); err != nil {
			return err
		}
	}
	return nil
}

// planHash 覆盖事件、步骤、风险、校验方式和失败策略。结构体字段顺序固定。
func planHash(eventID, risk, verify, onFailure string, steps []string) string {
	if steps == nil {
		steps = []string{}
	}
	raw, _ := json.Marshal(struct {
		EventID   string   `json:"event_id"`
		OnFailure string   `json:"on_failure"`
		Risk      string   `json:"risk"`
		Steps     []string `json:"steps"`
		Verify    string   `json:"verify"`
	}{eventID, onFailure, risk, steps, verify})
	sum := sha256.Sum256(raw)
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
