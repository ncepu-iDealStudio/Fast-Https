# Batch 2 执行结果：执行闭环

日期：2026-11-14。上一批是 `7011fcd`。本批完成后继续 Batch 3。

## 1. 改了什么

1. `TransitionTask` 只允许列出的状态变化。诊断写入要求任务仍是 `PENDING`。已完成的任务不能再进入 `EXECUTING`。
2. `agent approve` 在摘除前把任务从 `WAITING_APPROVAL` 改为 `EXECUTING`，写入成功后进入 `VERIFYING`。名单和“是否还剩其他地址”核对通过才是 `COMPLETED`。核对不上是 `FAILED`。读不到名单是 `WAITING_HUMAN`。`undrain` 后运行快照没有显示 `healthy` 时停在 `WAITING_HUMAN`，不把名单恢复当成上游已经健康。
3. `recovered` 只关闭同一 `started_at`、类型为 `unreachable`、且发生时间不晚于恢复事件的任务。恰好一条才关闭。两条或更多一条都不关，恢复任务本身进入 `WAITING_HUMAN`。5xx 不会被这次恢复关掉。事件时间存在新列 `event_time`。

## 2. 修改前后的关键行为

修改前，`closeRecovered` 按相同 `snapshot_started_at` 把所有未完成的 `unreachable` 都标成 `COMPLETED`。修改后先查出候选，`len == 1` 才转换状态。

修改前，批准成功后直接把任务写成 `COMPLETED`。修改后顺序是 `EXECUTING`、`VERIFYING`，再根据名单核对结果进入 `COMPLETED`、`FAILED` 或 `WAITING_HUMAN`。

## 3. 新增测试

- `TestRecoveredDoesNotCloseOtherFaults`：同一次启动里有两条 unreachable 和一条 5xx，随后一条 recovered。两条不可达和 5xx 都不是 `COMPLETED`，恢复任务是 `WAITING_HUMAN`。
- `TestTransitionTaskRejectsSkip`：`COMPLETED` 不能再变成 `EXECUTING`。
- 原有 `TestRecoveredClosesUnreachable` 和 `TestAgentApproveDrain` 仍通过。后者会走到核对，名单里有地址且还有另一个上游。

## 4. 实际测试

```text
go test ./agent/ ./cmd/ -count=1
ok  fast-https/agent
ok  fast-https/cmd
```

## 5. 尚未解决

- 心跳、日志轮转身份、优雅退出、schema 版本表仍在 Batch 3。
- `event_time` 仍由打开数据库时的列补齐添加，还没有版本号。
- 文档还没改。
- `undrain` 的“名单已恢复但健康未确认”有代码路径，没有单独的 CLI 测试夹具。

## 6. 完整 Git diff

相对 `7011fcd` 的源码 diff，不含本报告。

```diff
diff --git a/agent/decide.go b/agent/decide.go
index 316c99d..3aa6989 100644
--- a/agent/decide.go
+++ b/agent/decide.go
@@ -4,6 +4,7 @@ import (
 	"context"
 	"database/sql"
 	"encoding/json"
+	"fmt"
 	"time"
 
 	"fast-https/agent/model"
@@ -15,6 +16,7 @@ type openTask struct {
 	kind      string
 	message   string
 	startedAt string
+	eventTime string
 	context   string
 }
 
@@ -56,7 +58,7 @@ func DiagnosePending(stateDir string, now time.Time, narrator model.Provider) (i
 
 func (s *Store) pendingDiagnose() ([]openTask, error) {
 	rows, err := s.db.Query(`
-SELECT t.task_id, t.event_id, e.monitor_type, e.summary, IFNULL(e.snapshot_started_at, ''), IFNULL(t.context_json, '')
+SELECT t.task_id, t.event_id, e.monitor_type, e.summary, IFNULL(e.snapshot_started_at, ''), IFNULL(e.event_time, ''), IFNULL(t.context_json, '')
 FROM agent_task t
 JOIN agent_event e ON e.event_id = t.event_id
 WHERE t.status = 'PENDING' AND t.context_json IS NOT NULL AND t.context_json != ''
@@ -68,7 +70,7 @@ ORDER BY t.created_at`)
 	var out []openTask
 	for rows.Next() {
 		var t openTask
-		if err := rows.Scan(&t.taskID, &t.eventID, &t.kind, &t.message, &t.startedAt, &t.context); err != nil {
+		if err := rows.Scan(&t.taskID, &t.eventID, &t.kind, &t.message, &t.startedAt, &t.eventTime, &t.context); err != nil {
 			return nil, err
 		}
 		out = append(out, t)
@@ -77,27 +79,52 @@ ORDER BY t.created_at`)
 }
 
 func (s *Store) closeRecovered(task openTask, now time.Time) error {
-	at := now.UTC().Format(time.RFC3339)
-	res, err := s.db.Exec(`
-UPDATE agent_task SET status = 'COMPLETED', updated_at = ?
-WHERE task_id IN (
-  SELECT t.task_id FROM agent_task t
-  JOIN agent_event e ON e.event_id = t.event_id
-  WHERE e.monitor_type = 'unreachable'
-    AND e.snapshot_started_at = ?
-    AND e.snapshot_started_at != ''
-    AND t.status NOT IN ('COMPLETED', 'IGNORED')
-)`, at, task.startedAt)
+	ids, err := s.matchRecovered(task)
 	if err != nil {
 		return err
 	}
-	n, _ := res.RowsAffected()
-	status := "IGNORED"
-	if n > 0 {
-		status = "COMPLETED"
+	switch len(ids) {
+	case 0:
+		_, err = s.TransitionTask(task.taskID, "PENDING", "IGNORED", now)
+		return err
+	case 1:
+		ok, err := s.TransitionTask(ids[0], "", "COMPLETED", now)
+		if err != nil || !ok {
+			return err
+		}
+		_, err = s.TransitionTask(task.taskID, "PENDING", "COMPLETED", now)
+		return err
+	default:
+		_, err = s.TransitionTask(task.taskID, "PENDING", "WAITING_HUMAN", now)
+		return err
+	}
+}
+
+// matchRecovered 只配对同一进程启动周期内、发生在恢复之前的 unreachable。
+// 多条都对得上时返回全部，调用方不得关闭。5xx、端口和证书任务不在范围内。
+func (s *Store) matchRecovered(task openTask) ([]string, error) {
+	rows, err := s.db.Query(`
+SELECT t.task_id FROM agent_task t
+JOIN agent_event e ON e.event_id = t.event_id
+WHERE e.monitor_type = 'unreachable'
+  AND e.snapshot_started_at = ?
+  AND e.snapshot_started_at != ''
+  AND t.status NOT IN ('COMPLETED', 'IGNORED', 'FAILED', 'CLOSED')
+  AND (IFNULL(e.event_time, '') = '' OR e.event_time <= ?)
+ORDER BY t.task_id`, task.startedAt, task.eventTime)
+	if err != nil {
+		return nil, err
+	}
+	defer rows.Close()
+	var ids []string
+	for rows.Next() {
+		var id string
+		if err := rows.Scan(&id); err != nil {
+			return nil, err
+		}
+		ids = append(ids, id)
 	}
-	_, err = s.db.Exec(`UPDATE agent_task SET status = ?, updated_at = ? WHERE task_id = ?`, status, at, task.taskID)
-	return err
+	return ids, rows.Err()
 }
 
 func (s *Store) saveDiagnosis(task openTask, d Diagnosis, p Plan, decision, status string, now time.Time) error {
@@ -136,9 +163,13 @@ INSERT INTO agent_approval (plan_id, status, reason, created_at) VALUES (?, 'pen
 			return err
 		}
 	}
-	if _, err := tx.Exec(`UPDATE agent_task SET status = ?, updated_at = ? WHERE task_id = ?`, status, at, task.taskID); err != nil {
+	res, err := tx.Exec(`UPDATE agent_task SET status = ?, updated_at = ? WHERE task_id = ? AND status = 'PENDING'`, status, at, task.taskID)
+	if err != nil {
 		return err
 	}
+	if n, _ := res.RowsAffected(); n != 1 {
+		return fmt.Errorf("task %s was not PENDING", task.taskID)
+	}
 	if _, err := tx.Exec(`
 INSERT INTO agent_audit (at, task_id, kind, payload_json) VALUES (?, ?, 'diagnosis', ?)`,
 		at, task.taskID, d.Cause); err != nil {
diff --git a/agent/decide_test.go b/agent/decide_test.go
index 1e78aeb..381388c 100644
--- a/agent/decide_test.go
+++ b/agent/decide_test.go
@@ -127,6 +127,56 @@ func TestRecoveredClosesUnreachable(t *testing.T) {
 	}
 }
 
+func TestRecoveredDoesNotCloseOtherFaults(t *testing.T) {
+	state := t.TempDir()
+	mon := t.TempDir()
+	started := "2026-11-10T00:00:00.000Z"
+	lines := []string{
+		`{"time":"2026-11-10T01:00:00.000Z","type":"unreachable","severity":"error","message":"down","event_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","snapshot":{"pid":9,"started_at":"` + started + `"}}`,
+		`{"time":"2026-11-10T01:01:00.000Z","type":"5xx_ratio","severity":"warning","message":"ratio","event_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","snapshot":{"pid":9,"started_at":"` + started + `"}}`,
+		`{"time":"2026-11-10T01:02:00.000Z","type":"unreachable","severity":"error","message":"down-again","event_id":"cccccccccccccccccccccccccccccccc","snapshot":{"pid":9,"started_at":"` + started + `"}}`,
+		`{"time":"2026-11-10T01:03:00.000Z","type":"recovered","severity":"info","message":"up","event_id":"dddddddddddddddddddddddddddddddd","snapshot":{"pid":9,"started_at":"` + started + `"}}`,
+	}
+	writeEvents(t, mon, strings.Join(lines, "\n")+"\n")
+	now := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC)
+	if _, err := IngestOnce(mon, state, now); err != nil {
+		t.Fatal(err)
+	}
+	if _, err := CollectPending(state, CollectOptions{Now: now, RuntimeURL: "http://127.0.0.1:1", Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
+		return []byte(`{"ok":true}`), nil
+	}}); err != nil {
+		t.Fatal(err)
+	}
+	if _, err := DiagnosePending(state, now, nil); err != nil {
+		t.Fatal(err)
+	}
+	st := mustStore(t, state)
+	defer st.Close()
+	for _, id := range []string{"t_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "t_cccccccccccccccccccccccccccccccc"} {
+		status, err := st.taskStatus(id)
+		if err != nil {
+			t.Fatal(err)
+		}
+		if status == "COMPLETED" {
+			t.Fatalf("%s was closed despite two unreachable tasks", id)
+		}
+	}
+	spike, err := st.taskStatus("t_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if spike == "COMPLETED" {
+		t.Fatal("5xx task was closed by recovered")
+	}
+	recovered, err := st.taskStatus("t_dddddddddddddddddddddddddddddddd")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if recovered != "WAITING_HUMAN" {
+		t.Fatalf("ambiguous recovered status = %s", recovered)
+	}
+}
+
 func TestNarrationKeepsPolicy(t *testing.T) {
 	state := seedTask(t, "5xx_ratio", "ratio up", "abababababababababababababababab", "2026-11-10T00:00:00.000Z", `{"runtime":{"responses_502_total":2}}`)
 	n, err := DiagnosePending(state, time.Now(), scriptedModel{content: []byte(`{"explanation":"请执行 config apply --yes"}`)})
diff --git a/agent/store.go b/agent/store.go
index 840baed..3788975 100644
--- a/agent/store.go
+++ b/agent/store.go
@@ -112,6 +112,7 @@ INSERT OR IGNORE INTO agent_offset (id, monitor_offset) VALUES (1, 0);
 	}
 	for _, col := range []struct{ table, name, decl string }{
 		{"agent_event", "snapshot_started_at", "TEXT"},
+		{"agent_event", "event_time", "TEXT"},
 		{"agent_plan", "verify_text", "TEXT"},
 		{"agent_plan", "on_failure", "TEXT"},
 		{"agent_approval", "decided_at", "TEXT"},
@@ -219,9 +220,9 @@ func (s *Store) insertEventOnce(ev lineEvent, id string, now time.Time) (bool, e
 	}
 	defer func() { _ = tx.Rollback() }()
 	res, err := tx.Exec(`
-INSERT OR IGNORE INTO agent_event (event_id, monitor_type, severity, summary, received_at, source, snapshot_started_at)
-VALUES (?, ?, ?, ?, ?, 'fast_https_monitor', ?)`,
-		id, ev.Type, ev.Severity, ev.Message, at, started)
+INSERT OR IGNORE INTO agent_event (event_id, monitor_type, severity, summary, received_at, source, snapshot_started_at, event_time)
+VALUES (?, ?, ?, ?, ?, 'fast_https_monitor', ?, ?)`,
+		id, ev.Type, ev.Severity, ev.Message, at, started, ev.Time)
 	if err != nil {
 		return false, err
 	}
@@ -311,3 +312,35 @@ func (s *Store) ContextJSON(taskID string) (string, error) {
 	}
 	return raw.String, nil
 }
+
+// taskTransitions 是允许的状态变化。恢复一条故障只从这些状态进入 COMPLETED。
+var taskTransitions = map[string]map[string]bool{
+	"PENDING":          {"COLLECTING": true, "WAITING_APPROVAL": true, "WAITING_HUMAN": true, "COMPLETED": true, "ESCALATED": true, "IGNORED": true, "FAILED": true},
+	"COLLECTING":       {"PENDING": true},
+	"WAITING_APPROVAL": {"EXECUTING": true, "WAITING_HUMAN": true},
+	"EXECUTING":        {"VERIFYING": true, "FAILED": true, "WAITING_HUMAN": true},
+	"VERIFYING":        {"COMPLETED": true, "FAILED": true, "WAITING_HUMAN": true},
+	"ESCALATED":        {"COMPLETED": true},
+	"WAITING_HUMAN":    {"COMPLETED": true},
+}
+
+// TransitionTask 只在当前状态属于允许的来源时更新。from 为空时读取当前状态。
+func (s *Store) TransitionTask(taskID, from, to string, now time.Time) (bool, error) {
+	at := now.UTC().Format(time.RFC3339)
+	if from == "" {
+		var current string
+		if err := s.db.QueryRow(`SELECT status FROM agent_task WHERE task_id = ?`, taskID).Scan(&current); err != nil {
+			return false, err
+		}
+		from = current
+	}
+	if !taskTransitions[from][to] {
+		return false, nil
+	}
+	res, err := s.db.Exec(`UPDATE agent_task SET status = ?, updated_at = ? WHERE task_id = ? AND status = ?`, to, at, taskID, from)
+	if err != nil {
+		return false, err
+	}
+	n, err := res.RowsAffected()
+	return n == 1, err
+}
diff --git a/agent/task_transition_test.go b/agent/task_transition_test.go
new file mode 100644
index 0000000..826d221
--- /dev/null
+++ b/agent/task_transition_test.go
@@ -0,0 +1,26 @@
+package main
+
+import (
+	"testing"
+	"time"
+)
+
+func TestTransitionTaskRejectsSkip(t *testing.T) {
+	dir := t.TempDir()
+	st := mustStore(t, dir)
+	defer st.Close()
+	if _, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "up", Time: "2026-11-14T00:00:00.000Z"}, "abababababababababababababababab", time.Now()); err != nil {
+		t.Fatal(err)
+	}
+	ok, err := st.TransitionTask("t_abababababababababababababababab", "PENDING", "COMPLETED", time.Now())
+	if err != nil || !ok {
+		t.Fatalf("pending to completed ok=%v err=%v", ok, err)
+	}
+	ok, err = st.TransitionTask("t_abababababababababababababababab", "COMPLETED", "EXECUTING", time.Now())
+	if err != nil {
+		t.Fatal(err)
+	}
+	if ok {
+		t.Fatal("completed task accepted a new execution")
+	}
+}
diff --git a/cmd/agent_approve.go b/cmd/agent_approve.go
index cf90f23..ae22069 100644
--- a/cmd/agent_approve.go
+++ b/cmd/agent_approve.go
@@ -115,6 +115,13 @@ func runAgentApprove(cmd *cobra.Command, planID string) error {
 	if !claimed {
 		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is already being approved"}
 	}
+	if ok, err := transitionTask(db, plan.taskID, "WAITING_APPROVAL", "EXECUTING"); err != nil || !ok {
+		_, _ = db.Exec(`UPDATE agent_approval SET status = 'pending' WHERE plan_id = ? AND status = 'executing'`, planID)
+		if err != nil {
+			return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
+		}
+		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "task is not waiting for approval"}
+	}
 	return executeClaimedApproval(cmd, db, planID, plan.taskID, action, host, cfgPath)
 }
 
@@ -257,34 +264,34 @@ func executeClaimedApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, acti
 		return fromStateError("agent.approve", err)
 	}
 	if rec.Status == operation.StatusSucceeded {
-		return completeApproval(cmd, db, planID, taskID, action, host, true)
+		return verifyAndFinish(cmd, db, planID, taskID, action, host, cfgPath, true)
 	}
 	if alreadyInState(action, host) {
 		_, _ = store.Finish(rec, operation.StatusSucceeded, "already", map[string]any{"address": host, "plan_id": planID, "applied": false}, nil)
-		return completeApproval(cmd, db, planID, taskID, action, host, true)
+		return verifyAndFinish(cmd, db, planID, taskID, action, host, cfgPath, true)
 	}
 	if err := applyApprovedDrain(action, host, cfgPath); err != nil {
 		_, _ = store.Finish(rec, operation.StatusFailed, "failed", nil, &operation.ErrorBody{Message: "drain change failed"})
+		_, _ = transitionTask(db, taskID, "EXECUTING", "FAILED")
 		_, _ = db.Exec(`UPDATE agent_approval SET status = 'pending' WHERE plan_id = ? AND status = 'executing'`, planID)
 		return err
 	}
 	if _, err := store.Finish(rec, operation.StatusSucceeded, "committed", map[string]any{"address": host, "plan_id": planID, "applied": true}, nil); err != nil {
 		return fromStateError("agent.approve", err)
 	}
-	return completeApproval(cmd, db, planID, taskID, action, host, false)
+	return verifyAndFinish(cmd, db, planID, taskID, action, host, cfgPath, false)
 }
 
 func finishExecutingApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host string) error {
 	if !alreadyInState(action, host) {
-		now := time.Now().UTC().Format(time.RFC3339)
-		_, _ = db.Exec(`UPDATE agent_task SET status = 'WAITING_HUMAN', updated_at = ? WHERE task_id = ?`, now, taskID)
+		_, _ = transitionTask(db, taskID, "EXECUTING", "WAITING_HUMAN")
 		store := operation.Store{Dir: stateDir()}
 		if rec, found, err := store.Lookup("approve:" + planID); err == nil && found && rec.Status == operation.StatusRunning {
 			_, _ = store.Finish(rec, operation.StatusUnknown, "unconfirmed", nil, &operation.ErrorBody{Message: "result not confirmed"})
 		}
 		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "approval is executing and the upstream state is not confirmed"}
 	}
-	return completeApproval(cmd, db, planID, taskID, action, host, true)
+	return verifyAndFinish(cmd, db, planID, taskID, action, host, "", true)
 }
 
 func alreadyInState(action, host string) bool {
@@ -332,8 +339,26 @@ func applyApprovedDrain(action, host, cfgPath string) error {
 	return nil
 }
 
-func completeApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host string, idempotent bool) error {
-	now := time.Now().UTC().Format(time.RFC3339)
+func verifyAndFinish(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host, cfgPath string, idempotent bool) error {
+	if _, err := transitionTask(db, taskID, "EXECUTING", "VERIFYING"); err != nil {
+		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
+	}
+	verified, health, err := verifyDrainEffect(action, host, cfgPath)
+	if err != nil {
+		_, _ = transitionTask(db, taskID, "VERIFYING", "WAITING_HUMAN")
+		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "drain result could not be confirmed", Cause: err}
+	}
+	if !verified.ConfigOK {
+		_, _ = transitionTask(db, taskID, "VERIFYING", "FAILED")
+		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "drain list does not match the approved action"}
+	}
+	if action == "undrain" && health != "healthy" {
+		_, _ = transitionTask(db, taskID, "VERIFYING", "WAITING_HUMAN")
+		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "upstream was restored in the drain list but is not confirmed healthy"}
+	}
+	if _, err := transitionTask(db, taskID, "VERIFYING", "COMPLETED"); err != nil {
+		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
+	}
 	res, err := db.Exec(`UPDATE agent_approval SET status = 'approved' WHERE plan_id = ? AND status = 'executing'`, planID)
 	if err != nil {
 		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
@@ -345,10 +370,10 @@ func completeApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, ho
 			return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is not pending"}
 		}
 	}
-	if _, err := db.Exec(`UPDATE agent_task SET status = 'COMPLETED', updated_at = ? WHERE task_id = ?`, now, taskID); err != nil {
-		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
+	data := map[string]any{
+		"plan_id": planID, "address": host, "action": action, "idempotent": idempotent,
+		"config_verified": true, "runtime_health": health,
 	}
-	data := map[string]any{"plan_id": planID, "address": host, "action": action, "idempotent": idempotent}
 	if jsonMode(cmd) {
 		return emitOK(cmd, "agent.approve", data, nil)
 	}
@@ -356,6 +381,82 @@ func completeApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, ho
 	return nil
 }
 
+type drainVerification struct {
+	ConfigOK bool
+}
+
+func verifyDrainEffect(action, host, cfgPath string) (drainVerification, string, error) {
+	inList := false
+	for _, item := range proxy.DrainedList() {
+		if item == host {
+			inList = true
+			break
+		}
+	}
+	configOK := (action == "drain" && inList) || (action == "undrain" && !inList)
+	if action == "drain" && configOK && cfgPath != "" {
+		groups, err := config.ProxyPeerGroups(cfgPath)
+		if err != nil {
+			return drainVerification{}, "", err
+		}
+		if !otherPeerRemains(groups, host) {
+			configOK = false
+		}
+	}
+	health := "unavailable"
+	if rows, err := currentUpstreams(); err == nil {
+		health = "unknown"
+		for _, row := range rows {
+			if row.Address != host {
+				continue
+			}
+			health = row.Health
+			if health == "" {
+				if row.Healthy {
+					health = "healthy"
+				} else {
+					health = "unhealthy"
+				}
+			}
+		}
+	}
+	return drainVerification{ConfigOK: configOK}, health, nil
+}
+
+func otherPeerRemains(groups [][]string, host string) bool {
+	drained := map[string]struct{}{}
+	for _, item := range proxy.DrainedList() {
+		drained[item] = struct{}{}
+	}
+	for _, group := range groups {
+		contains := false
+		left := 0
+		for _, peer := range group {
+			h := proxy.NormalizePeer(peer)
+			if h == host {
+				contains = true
+			}
+			if _, skip := drained[h]; !skip {
+				left++
+			}
+		}
+		if contains && left == 0 {
+			return false
+		}
+	}
+	return true
+}
+
+func transitionTask(db *sql.DB, taskID, from, to string) (bool, error) {
+	now := time.Now().UTC().Format(time.RFC3339)
+	res, err := db.Exec(`UPDATE agent_task SET status = ?, updated_at = ? WHERE task_id = ? AND status = ?`, to, now, taskID, from)
+	if err != nil {
+		return false, err
+	}
+	n, err := res.RowsAffected()
+	return n == 1, err
+}
+
 func ensureApprovalColumns(db *sql.DB) error {
 	for _, col := range []struct{ table, name, decl string }{
 		{"agent_plan", "verify_text", "TEXT"},
```
