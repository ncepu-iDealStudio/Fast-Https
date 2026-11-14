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
	if ok, err := transitionTask(db, plan.taskID, "WAITING_APPROVAL", "EXECUTING"); err != nil || !ok {
		_, _ = db.Exec(`UPDATE agent_approval SET status = 'pending' WHERE plan_id = ? AND status = 'executing'`, planID)
		if err != nil {
			return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
		}
		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "task is not waiting for approval"}
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
		kind := interpretUpstreamHealth(row.Health, row.Healthy)
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
		// 名单还没写。审批和任务一起退回，才能再次批准。
		_ = restoreApprovalBeforeAction(db, planID, taskID)
		return fromStateError("agent.approve", err)
	}
	if rec.Status == operation.StatusSucceeded {
		return verifyAndFinish(cmd, db, planID, taskID, action, host, cfgPath, true)
	}
	if alreadyInState(action, host) {
		_, _ = store.Finish(rec, operation.StatusSucceeded, "already", map[string]any{"address": host, "plan_id": planID, "applied": false}, nil)
		return verifyAndFinish(cmd, db, planID, taskID, action, host, cfgPath, true)
	}
	if err := prepareApprovedDrain(); err != nil {
		_, _ = store.Finish(rec, operation.StatusFailed, "not-started", nil, &operation.ErrorBody{Message: "drain did not start"})
		_ = restoreApprovalBeforeAction(db, planID, taskID)
		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "drain did not start", Cause: err}
	}
	if err := applyApprovedDrain(action, host, cfgPath); err != nil {
		_, _ = store.Finish(rec, operation.StatusUnknown, "unconfirmed", nil, &operation.ErrorBody{Message: "drain result not confirmed"})
		_ = markApprovalUncertain(db, planID, taskID)
		return err
	}
	if _, err := store.Finish(rec, operation.StatusSucceeded, "committed", map[string]any{"address": host, "plan_id": planID, "applied": true}, nil); err != nil {
		_ = markApprovalUncertain(db, planID, taskID)
		return fromStateError("agent.approve", err)
	}
	return verifyAndFinish(cmd, db, planID, taskID, action, host, cfgPath, false)
}

func finishExecutingApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host string) error {
	if !alreadyInState(action, host) {
		if err := requireTransition(db, taskID, "EXECUTING", "WAITING_HUMAN"); err != nil {
			return err
		}
		store := operation.Store{Dir: stateDir()}
		if rec, found, err := store.Lookup("approve:" + planID); err == nil && found && rec.Status == operation.StatusRunning {
			_, _ = store.Finish(rec, operation.StatusUnknown, "unconfirmed", nil, &operation.ErrorBody{Message: "result not confirmed"})
		}
		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "approval is executing and the upstream state is not confirmed"}
	}
	return verifyAndFinish(cmd, db, planID, taskID, action, host, "", true)
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

// prepareApprovedDrain 在写名单之前运行。测试用它模拟「操作还没开始就失败」。
var prepareApprovedDrain = func() error { return nil }

func restoreApprovalBeforeAction(db *sql.DB, planID, taskID string) error {
	if _, err := db.Exec(`UPDATE agent_approval SET status = 'pending' WHERE plan_id = ? AND status = 'executing'`, planID); err != nil {
		return err
	}
	ok, err := transitionTask(db, taskID, "EXECUTING", "WAITING_APPROVAL")
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM agent_task WHERE task_id = ?`, taskID).Scan(&status); err != nil {
		return err
	}
	if status == "WAITING_APPROVAL" {
		return nil
	}
	return fmt.Errorf("task %s is %s after approval was returned to pending", taskID, status)
}

func markApprovalUncertain(db *sql.DB, planID, taskID string) error {
	ok, err := transitionTask(db, taskID, "EXECUTING", "WAITING_HUMAN")
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	var status string
	if scanErr := db.QueryRow(`SELECT status FROM agent_task WHERE task_id = ?`, taskID).Scan(&status); scanErr != nil {
		return scanErr
	}
	if status == "WAITING_HUMAN" {
		return nil
	}
	return fmt.Errorf("task %s stayed %s while approval %s is still executing", taskID, status, planID)
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

func verifyAndFinish(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host, cfgPath string, idempotent bool) error {
	if err := requireTransition(db, taskID, "EXECUTING", "VERIFYING"); err != nil {
		return err
	}
	verified, health, err := verifyDrainEffect(action, host, cfgPath)
	if err != nil {
		if moveErr := requireTransition(db, taskID, "VERIFYING", "WAITING_HUMAN"); moveErr != nil {
			return moveErr
		}
		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "drain result could not be confirmed", Cause: err}
	}
	if !verified.ConfigOK {
		if moveErr := requireTransition(db, taskID, "VERIFYING", "WAITING_HUMAN"); moveErr != nil {
			return moveErr
		}
		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "drain list does not match the approved action"}
	}
	if action == "undrain" && health != "healthy" {
		if moveErr := requireTransition(db, taskID, "VERIFYING", "WAITING_HUMAN"); moveErr != nil {
			return moveErr
		}
		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "upstream was restored in the drain list but is not confirmed healthy"}
	}
	if err := requireTransition(db, taskID, "VERIFYING", "COMPLETED"); err != nil {
		return err
	}
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
	data := map[string]any{
		"plan_id": planID, "address": host, "action": action, "idempotent": idempotent,
		"config_verified": true, "runtime_health": health,
	}
	if jsonMode(cmd) {
		return emitOK(cmd, "agent.approve", data, nil)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "approved %s %s\n", action, host)
	return nil
}

type drainVerification struct {
	ConfigOK bool
}

func verifyDrainEffect(action, host, cfgPath string) (drainVerification, string, error) {
	inList := false
	for _, item := range proxy.DrainedList() {
		if item == host {
			inList = true
			break
		}
	}
	configOK := (action == "drain" && inList) || (action == "undrain" && !inList)
	if action == "drain" && configOK && cfgPath != "" {
		groups, err := config.ProxyPeerGroups(cfgPath)
		if err != nil {
			return drainVerification{}, "", err
		}
		if !otherPeerRemains(groups, host) {
			configOK = false
		}
	}
	health := "unavailable"
	if rows, err := currentUpstreams(); err == nil {
		health = "unknown"
		for _, row := range rows {
			if row.Address != host {
				continue
			}
			health = interpretUpstreamHealth(row.Health, row.Healthy)
		}
	}
	return drainVerification{ConfigOK: configOK}, health, nil
}

func otherPeerRemains(groups [][]string, host string) bool {
	drained := map[string]struct{}{}
	for _, item := range proxy.DrainedList() {
		drained[item] = struct{}{}
	}
	for _, group := range groups {
		contains := false
		left := 0
		for _, peer := range group {
			h := proxy.NormalizePeer(peer)
			if h == host {
				contains = true
			}
			if _, skip := drained[h]; !skip {
				left++
			}
		}
		if contains && left == 0 {
			return false
		}
	}
	return true
}

func interpretUpstreamHealth(health string, legacy *bool) string {
	switch health {
	case "healthy", "unhealthy", "unknown":
		return health
	case "":
		if legacy != nil && *legacy {
			return "healthy"
		}
		return "unknown"
	default:
		return "unknown"
	}
}

func requireTransition(db *sql.DB, taskID, from, to string) error {
	ok, err := transitionTask(db, taskID, from, to)
	if err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
	}
	if ok {
		return nil
	}
	var status string
	err = db.QueryRow(`SELECT status FROM agent_task WHERE task_id = ?`, taskID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return &CLIError{Command: "agent.approve", Code: CodeNotFound, Message: "task not found"}
	}
	if err != nil {
		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
	}
	return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: fmt.Sprintf("task status is %s, cannot move from %s to %s", status, from, to)}
}

func transitionTask(db *sql.DB, taskID, from, to string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`UPDATE agent_task SET status = ?, updated_at = ? WHERE task_id = ? AND status = ?`, to, now, taskID, from)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
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
