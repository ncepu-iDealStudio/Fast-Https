# Batch 1 执行结果：P0 可靠性与审批安全

日期：2026-11-14。基线是 Batch 0 提交 `ebc503f`。本批完成后停止，不进入 Batch 2。

## 1. 改了什么

1. 上游健康不再等于“累计错误是否为 0”。最近一分钟内，成功晚于失败是 `healthy`，失败晚于成功是 `unhealthy`，没有样本或样本已过窗口是 `unknown`。配置里的地址即使没有错误也会出现。`drained` 单独标记。累计计数 `dial_fail_total` 等仍然保留。
2. 事件和任务在同一个 SQLite 事务里写入。任务插入失败会回滚事件。已有事件但没有任务时补建任务。暂时性的锁错误会重试。
3. 没有可执行摘除步骤的 `REQUIRE_APPROVAL` 进入 `WAITING_HUMAN`，不写待审批行。有一条合法 `upstream drain/undrain` 才进入 `WAITING_APPROVAL`。历史空计划从 `WAITING_APPROVAL` 迁到 `WAITING_HUMAN`，审批行改为 `expired`，不删除。
4. 审批用 `UPDATE ... WHERE status = 'pending'` 占用为 `executing`。影响行数不是 1 就不再执行摘除。
5. 计划哈希覆盖事件 ID、步骤、风险、校验说明和失败策略。字段放在结构体里序列化，不用 map。
6. `agent approve` 在 `SaveDrained` 之前写 operation。已经处于目标摘除状态时不再写一次。执行中断且名单对不上时，任务改为 `WAITING_HUMAN`，operation 标为 `unknown`，不自动再摘一次。

## 2. 修改前后的关键代码

健康判断。修改前是累计错误为 0 则 `Healthy: true`，而且只有出过错的地址才出现。修改后：

```go
func healthState(c *addrCounters, now time.Time) string {
	if c == nil || c.samples.Load() == 0 {
		return "unknown"
	}
	// 窗口内成功不早于失败：healthy
	// 窗口内仍有更新的失败：unhealthy
	// 其余：unknown
}
```

事件与任务。修改前是两次独立 `Exec`，事件已存在就直接返回。修改后 `insertEventOnce` 在一个事务里插入；`RowsAffected == 0` 时如果没有任务就补建。

审批占用。修改前是先读到 `pending`，执行 `SaveDrained`，再 `UPDATE status = 'approved'`。修改后先：

```sql
UPDATE agent_approval
SET status = 'executing', actor = 'cli', decided_at = ?
WHERE plan_id = ? AND status = 'pending'
```

只有影响 1 行才继续。operation 的 `Begin` 发生在 `SaveDrained` 之前。

计划哈希。修改前是 `sha256(eventID + "\n" + json(steps))`。修改后是固定字段顺序的结构体：`event_id`、`on_failure`、`risk`、`steps`、`verify`。

## 3. 新增测试

- `modules/proxy/health_test.go`：`TestHealthUsesRecentWindow`、`TestRecentFailureIsUnhealthy`
- `agent/plan_health_test.go`：`TestDrainStepHealthCases`、`TestPlanHashChangesWithExecutionFields`
- `agent/store_event_test.go`：`TestInsertEventAndTaskTogether`、`TestTaskInsertFailureRollsBackEvent`、`TestOrphanEventIsRepaired`、`TestInsertEventRetriesTransientFailure`、`TestConcurrentInsertCreatesOneTask`、`TestWaitingApprovalWithoutPlanBecomesHuman`
- `cmd/cli_agent_test.go`：`TestApproveClaimIsSingleWinner`、`TestApproveRejectsStaleHealth`，并让原有 `TestAgentApproveDrain` 提供运行快照

## 4. 实际测试

```text
go test ./agent/ ./modules/proxy/ ./cmd/ -count=1
ok  fast-https/agent
ok  fast-https/modules/proxy
ok  fast-https/cmd

go test ./tests/arch/ -count=1 -run TestAgentDoesNotImportServer
ok  fast-https/tests/arch
```

并发插入在第一次运行时失败：8 个 goroutine 打到连接池里的不同 SQLite 连接，返回 `SQLITE_BUSY`，两次重试不够。之后把这个库设成 `SetMaxOpenConns(1)`，再跑通过。上面的通过结果是这次修复之后的。

## 5. 尚未解决

- 任务在摘除写入成功后仍直接变成 `COMPLETED`。执行后的 `VERIFYING` 和“配置已摘除不等于上游已恢复健康”留在 Batch 2。
- `closeRecovered` 仍按同一次 `snapshot_started_at` 关闭全部 `unreachable` 任务。
- 没有心跳、日志轮转身份、优雅退出，也没有 schema 版本表。新增列仍用 `PRAGMA table_info` 后 `ALTER`，Batch 3 再收成版本迁移。
- 审批状态沿用现有小写：`pending`、`executing`、`approved`、`rejected`、`expired`。没有另建一套大写状态。
- operation 沿用现有 `running`、`succeeded`、`failed`、`unknown`，没有新增 `PREPARED`。`Begin` 的 phase 写成 `prepared`，状态仍是 `running`。
- 计划哈希在 `agent/plan.go` 和 `cmd/agent_approve.go` 各有一份，因为分层测试不允许这两边互相引用。两边的 JSON 字段顺序必须一起改。
- `docs/feature/7-proxy.md` 仍写着 `healthy` 失败一次后保持到进程重启。这句已经过时，按批次留到 Batch 4。
- 健康窗口是固定的一分钟，没有主动探测。没有请求记录的地址是 `unknown`，不会被拿来生成摘除计划。

## 6. 完整 Git diff

下面是本批源码相对 `ebc503f` 的 diff，不含本报告。

```diff
diff --git a/agent/certpoll_test.go b/agent/certpoll_test.go
index a1045ed..a35085e 100644
--- a/agent/certpoll_test.go
+++ b/agent/certpoll_test.go
@@ -40,7 +40,7 @@ func TestCertPollThreshold(t *testing.T) {
 	if err != nil {
 		t.Fatal(err)
 	}
-	if status != "WAITING_APPROVAL" {
+	if status != "WAITING_HUMAN" {
 		t.Fatalf("status = %s", status)
 	}
 	var msg string
diff --git a/agent/decide.go b/agent/decide.go
index 485d6a7..316c99d 100644
--- a/agent/decide.go
+++ b/agent/decide.go
@@ -45,7 +45,7 @@ func DiagnosePending(stateDir string, now time.Time, narrator model.Provider) (i
 		}
 		p := makePlan(task.taskID, task.eventID, d.Classification, drainStep(d.Classification, task.context))
 		decision := policyOf(d.Classification, p.Steps)
-		status := statusFor(d.Classification, decision)
+		status := statusFor(d.Classification, decision, p.Steps)
 		if err := st.saveDiagnosis(task, d, p, decision, status, now); err != nil {
 			return n, err
 		}
@@ -120,16 +120,16 @@ VALUES (?, ?, ?, ?, ?, ?, ?)`,
 		return err
 	}
 	approval := 0
-	if decision == policyApproval {
+	if decision == policyApproval && executableDrain(p.Steps) {
 		approval = 1
 	}
 	if _, err := tx.Exec(`
-INSERT INTO agent_plan (plan_id, task_id, event_id, risk, plan_hash, steps_json, needs_approval)
-VALUES (?, ?, ?, ?, ?, ?, ?)`,
-		p.ID, task.taskID, task.eventID, p.Risk, p.Hash, string(steps), approval); err != nil {
+INSERT INTO agent_plan (plan_id, task_id, event_id, risk, plan_hash, steps_json, needs_approval, verify_text, on_failure)
+VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
+		p.ID, task.taskID, task.eventID, p.Risk, p.Hash, string(steps), approval, p.Verify, p.OnFailure); err != nil {
 		return err
 	}
-	if decision == policyApproval {
+	if decision == policyApproval && executableDrain(p.Steps) {
 		if _, err := tx.Exec(`
 INSERT INTO agent_approval (plan_id, status, reason, created_at) VALUES (?, 'pending', ?, ?)`,
 			p.ID, d.Cause, at); err != nil {
diff --git a/agent/decide_test.go b/agent/decide_test.go
index a583fea..1e78aeb 100644
--- a/agent/decide_test.go
+++ b/agent/decide_test.go
@@ -26,21 +26,28 @@ func TestDiagnoseMapping(t *testing.T) {
 		t.Fatalf("class = %q err=%v", class, err)
 	}
 	status, err := st.taskStatus("t_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
-	if err != nil || status != "WAITING_APPROVAL" {
+	if err != nil || status != "WAITING_HUMAN" {
 		t.Fatalf("status = %q err=%v", status, err)
 	}
-	var steps, decisionCause string
+	var steps, cause string
 	if err := st.db.QueryRow(`SELECT steps_json FROM agent_plan WHERE event_id = ?`, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee").Scan(&steps); err != nil {
 		t.Fatal(err)
 	}
 	if steps != "[]" {
 		t.Fatalf("steps = %s", steps)
 	}
-	if err := st.db.QueryRow(`SELECT reason FROM agent_approval WHERE plan_id = ?`, "p_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee").Scan(&decisionCause); err != nil {
+	if err := st.db.QueryRow(`SELECT cause FROM agent_diagnosis WHERE task_id = ?`, "t_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee").Scan(&cause); err != nil {
 		t.Fatal(err)
 	}
-	if !strings.Contains(decisionCause, "responses_502_total=4") {
-		t.Fatalf("cause = %s", decisionCause)
+	if !strings.Contains(cause, "responses_502_total=4") {
+		t.Fatalf("cause = %s", cause)
+	}
+	var approvals int
+	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_approval`).Scan(&approvals); err != nil {
+		t.Fatal(err)
+	}
+	if approvals != 0 {
+		t.Fatalf("empty plan entered the approval queue: %d", approvals)
 	}
 }
 
@@ -139,7 +146,7 @@ func TestNarrationKeepsPolicy(t *testing.T) {
 		t.Fatal(err)
 	}
 	status, _ = st.taskStatus("t_abababababababababababababababab")
-	if steps != "[]" || status != "WAITING_APPROVAL" {
+	if steps != "[]" || status != "WAITING_HUMAN" {
 		t.Fatalf("steps=%s status=%s", steps, status)
 	}
 }
diff --git a/agent/plan.go b/agent/plan.go
index 549771e..013bfdd 100644
--- a/agent/plan.go
+++ b/agent/plan.go
@@ -18,12 +18,39 @@ type Plan struct {
 	Hash      string
 }
 
+const (
+	planVerifyText    = "重新读取 /runtime 与 upstream 名单，不自动执行"
+	planOnFailureText = "停止自动尝试并保留待审批记录"
+)
+
+// planCanon 的字段顺序就是哈希输入。不要改成 map。
+type planCanon struct {
+	EventID   string   `json:"event_id"`
+	OnFailure string   `json:"on_failure"`
+	Risk      string   `json:"risk"`
+	Steps     []string `json:"steps"`
+	Verify    string   `json:"verify"`
+}
+
+func hashPlan(eventID, risk, verify, onFailure string, steps []string) string {
+	if steps == nil {
+		steps = []string{}
+	}
+	raw, _ := json.Marshal(planCanon{
+		EventID:   eventID,
+		OnFailure: onFailure,
+		Risk:      risk,
+		Steps:     steps,
+		Verify:    verify,
+	})
+	sum := sha256.Sum256(raw)
+	return hex.EncodeToString(sum[:])
+}
+
 func makePlan(taskID, eventID, classification string, steps []string) Plan {
 	if steps == nil {
 		steps = []string{}
 	}
-	raw, _ := json.Marshal(steps)
-	sum := sha256.Sum256(append([]byte(eventID+"\n"), raw...))
 	risk := "low"
 	if classification == "unknown" || classification == "collect_error" || classification == "certificate_read_error" {
 		risk = "unknown"
@@ -34,41 +61,38 @@ func makePlan(taskID, eventID, classification string, steps []string) Plan {
 		TaskID:    taskID,
 		Risk:      risk,
 		Steps:     steps,
-		Verify:    "重新读取 /runtime 与 upstream 名单，不自动执行",
-		OnFailure: "停止自动尝试并保留待审批记录",
-		Hash:      hex.EncodeToString(sum[:]),
+		Verify:    planVerifyText,
+		OnFailure: planOnFailureText,
+		Hash:      hashPlan(eventID, risk, planVerifyText, planOnFailureText, steps),
 	}
 }
 
-// drainStep 只有在恰好一个地址不健康、并且还有另一个地址时，才提出摘除。
-// 事件说明里的命令文本不会出现在步骤里。
+type upstreamFact struct {
+	Address string `json:"address"`
+	Healthy bool   `json:"healthy"`
+	Health  string `json:"health"`
+	Drained bool   `json:"drained"`
+}
+
+// drainStep 只在一个地址当前不健康、且另有健康地址时提出摘除。
+// unknown 既不算可摘除，也不算备用。已经 drained 的地址不再生成计划。
 func drainStep(classification, contextJSON string) []string {
 	if classification != "http_5xx_spike" && classification != "port_unreachable" {
 		return nil
 	}
-	var doc struct {
-		Runtime json.RawMessage `json:"runtime"`
-	}
-	if json.Unmarshal([]byte(contextJSON), &doc) != nil || len(doc.Runtime) == 0 || doc.Runtime[0] != '{' {
-		return nil
-	}
-	var snap struct {
-		Upstreams []struct {
-			Address string `json:"address"`
-			Healthy bool   `json:"healthy"`
-		} `json:"upstreams"`
-	}
-	if json.Unmarshal(doc.Runtime, &snap) != nil {
+	rows := upstreamFacts(contextJSON)
+	if len(rows) < 2 {
 		return nil
 	}
 	var bad, good []string
-	for _, item := range snap.Upstreams {
-		if item.Address == "" {
+	for _, item := range rows {
+		if item.Address == "" || item.Drained {
 			continue
 		}
-		if item.Healthy {
+		switch upstreamHealth(item) {
+		case "healthy":
 			good = append(good, item.Address)
-		} else {
+		case "unhealthy":
 			bad = append(bad, item.Address)
 		}
 	}
@@ -77,3 +101,36 @@ func drainStep(classification, contextJSON string) []string {
 	}
 	return []string{"upstream drain " + bad[0]}
 }
+
+func upstreamFacts(contextJSON string) []upstreamFact {
+	var doc struct {
+		Runtime json.RawMessage `json:"runtime"`
+	}
+	if json.Unmarshal([]byte(contextJSON), &doc) != nil || len(doc.Runtime) == 0 || doc.Runtime[0] != '{' {
+		return nil
+	}
+	var snap struct {
+		Upstreams []upstreamFact `json:"upstreams"`
+		Upstream  struct {
+			Upstreams []upstreamFact `json:"upstreams"`
+		} `json:"upstream"`
+	}
+	if json.Unmarshal(doc.Runtime, &snap) != nil {
+		return nil
+	}
+	if len(snap.Upstream.Upstreams) > 0 {
+		return snap.Upstream.Upstreams
+	}
+	return snap.Upstreams
+}
+
+func upstreamHealth(item upstreamFact) string {
+	switch item.Health {
+	case "healthy", "unhealthy", "unknown":
+		return item.Health
+	}
+	if item.Healthy {
+		return "healthy"
+	}
+	return "unhealthy"
+}
diff --git a/agent/plan_health_test.go b/agent/plan_health_test.go
new file mode 100644
index 0000000..29472d5
--- /dev/null
+++ b/agent/plan_health_test.go
@@ -0,0 +1,52 @@
+package main
+
+import "testing"
+
+func TestDrainStepHealthCases(t *testing.T) {
+	cases := []struct {
+		name string
+		ctx  string
+		want string
+	}{
+		{"all healthy", `{"runtime":{"upstream":{"upstreams":[{"address":"10.0.0.1:8080","health":"healthy"},{"address":"10.0.0.2:8080","health":"healthy"}]}}}`, ""},
+		{"one unhealthy", `{"runtime":{"upstream":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy"},{"address":"10.0.0.2:8080","health":"healthy"}]}}}`, "upstream drain 10.0.0.1:8080"},
+		{"all unhealthy", `{"runtime":{"upstream":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy"},{"address":"10.0.0.2:8080","health":"unhealthy"}]}}}`, ""},
+		{"recovered", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"healthy","healthy":false},{"address":"10.0.0.2:8080","health":"healthy"}]}}`, ""},
+		{"no samples", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"unknown"},{"address":"10.0.0.2:8080","health":"healthy"}]}}`, ""},
+		{"already drained", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy","drained":true},{"address":"10.0.0.2:8080","health":"healthy"}]}}`, ""},
+		{"only one", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy"}]}}`, ""},
+		{"one healthy among several", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy"},{"address":"10.0.0.2:8080","health":"unknown"},{"address":"10.0.0.3:8080","health":"healthy"}]}}`, "upstream drain 10.0.0.1:8080"},
+		{"legacy bool", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","healthy":false},{"address":"10.0.0.2:8080","healthy":true}]}}`, "upstream drain 10.0.0.1:8080"},
+	}
+	for _, tc := range cases {
+		t.Run(tc.name, func(t *testing.T) {
+			got := drainStep("http_5xx_spike", tc.ctx)
+			text := ""
+			if len(got) == 1 {
+				text = got[0]
+			} else if len(got) > 1 {
+				t.Fatalf("steps = %v", got)
+			}
+			if text != tc.want {
+				t.Fatalf("step = %q, want %q", text, tc.want)
+			}
+		})
+	}
+}
+
+func TestPlanHashChangesWithExecutionFields(t *testing.T) {
+	steps := []string{"upstream drain 10.0.0.1:8080"}
+	base := hashPlan("e1", "low", planVerifyText, planOnFailureText, steps)
+	if hashPlan("e1", "low", planVerifyText, planOnFailureText, steps) != base {
+		t.Fatal("same plan produced two hashes")
+	}
+	if hashPlan("e1", "high", planVerifyText, planOnFailureText, steps) == base {
+		t.Fatal("risk change did not change the hash")
+	}
+	if hashPlan("e1", "low", "other", planOnFailureText, steps) == base {
+		t.Fatal("verify change did not change the hash")
+	}
+	if hashPlan("e1", "low", planVerifyText, "stop", steps) == base {
+		t.Fatal("failure strategy change did not change the hash")
+	}
+}
diff --git a/agent/policy.go b/agent/policy.go
index a4270b3..a4f3c5b 100644
--- a/agent/policy.go
+++ b/agent/policy.go
@@ -55,16 +55,23 @@ func isDrainStep(step string) bool {
 	return fields[2] != "" && !strings.ContainsAny(fields[2], ";&|$`'\"\\")
 }
 
-func statusFor(classification, decision string) string {
+func statusFor(classification, decision string, steps []string) string {
 	switch classification {
 	case "collect_error", "certificate_read_error", "unknown", "service_unavailable":
 		return "ESCALATED"
 	}
-	if decision == policyApproval {
-		return "WAITING_APPROVAL"
-	}
 	if decision == policyAllow {
 		return "COMPLETED"
 	}
+	if decision == policyApproval && executableDrain(steps) {
+		return "WAITING_APPROVAL"
+	}
+	if decision == policyApproval {
+		return "WAITING_HUMAN"
+	}
 	return "ESCALATED"
 }
+
+func executableDrain(steps []string) bool {
+	return len(steps) == 1 && isDrainStep(steps[0])
+}
diff --git a/agent/store.go b/agent/store.go
index fb9b946..840baed 100644
--- a/agent/store.go
+++ b/agent/store.go
@@ -2,14 +2,26 @@ package main
 
 import (
 	"database/sql"
+	"errors"
 	"fmt"
 	"os"
 	"path/filepath"
+	"strings"
 	"time"
 
 	_ "modernc.org/sqlite"
 )
 
+// errRetryTask 让 InsertEvent 再试一次。测试用它模拟 SQLite 暂时失败。
+var errRetryTask = errors.New("retry task insert")
+
+var insertTaskRow = func(tx *sql.Tx, taskID, eventID, at string) error {
+	_, err := tx.Exec(`
+INSERT INTO agent_task (task_id, event_id, status, attempts, created_at, updated_at)
+VALUES (?, ?, 'PENDING', 0, ?, ?)`, taskID, eventID, at, at)
+	return err
+}
+
 // Store 是 Agent 自己的 SQLite 文件，不使用 Server 的状态目录。
 type Store struct {
 	db *sql.DB
@@ -23,12 +35,16 @@ func OpenStore(dir string) (*Store, error) {
 	if err != nil {
 		return nil, err
 	}
+	db.SetMaxOpenConns(1)
 	s := &Store{db: db}
+	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
+		_ = db.Close()
+		return nil, err
+	}
 	if err := s.migrate(); err != nil {
 		_ = db.Close()
 		return nil, err
 	}
-	_, _ = db.Exec(`ALTER TABLE agent_event ADD COLUMN snapshot_started_at TEXT`)
 	return s, nil
 }
 
@@ -91,6 +107,66 @@ CREATE TABLE IF NOT EXISTS agent_offset (
 );
 INSERT OR IGNORE INTO agent_offset (id, monitor_offset) VALUES (1, 0);
 `)
+	if err != nil {
+		return err
+	}
+	for _, col := range []struct{ table, name, decl string }{
+		{"agent_event", "snapshot_started_at", "TEXT"},
+		{"agent_plan", "verify_text", "TEXT"},
+		{"agent_plan", "on_failure", "TEXT"},
+		{"agent_approval", "decided_at", "TEXT"},
+		{"agent_approval", "actor", "TEXT"},
+	} {
+		if err := ensureColumn(s.db, col.table, col.name, col.decl); err != nil {
+			return err
+		}
+	}
+	return s.migrateWaitingHuman()
+}
+
+func ensureColumn(db *sql.DB, table, name, decl string) error {
+	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
+	if err != nil {
+		return err
+	}
+	defer rows.Close()
+	for rows.Next() {
+		var cid int
+		var col, typ string
+		var notNull, pk int
+		var dflt sql.NullString
+		if err := rows.Scan(&cid, &col, &typ, &notNull, &dflt, &pk); err != nil {
+			return err
+		}
+		if col == name {
+			return nil
+		}
+	}
+	if err := rows.Err(); err != nil {
+		return err
+	}
+	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + name + ` ` + decl)
+	return err
+}
+
+func (s *Store) migrateWaitingHuman() error {
+	_, err := s.db.Exec(`
+UPDATE agent_task SET status = 'WAITING_HUMAN'
+WHERE status = 'WAITING_APPROVAL'
+  AND task_id IN (
+    SELECT task_id FROM agent_plan
+    WHERE IFNULL(steps_json, '') IN ('', '[]', 'null')
+  )`)
+	if err != nil {
+		return err
+	}
+	_, err = s.db.Exec(`
+UPDATE agent_approval SET status = 'expired'
+WHERE status = 'pending'
+  AND plan_id IN (
+    SELECT plan_id FROM agent_plan
+    WHERE IFNULL(steps_json, '') IN ('', '[]', 'null')
+  )`)
 	return err
 }
 
@@ -117,14 +193,32 @@ func (s *Store) AuditCount(kind string) (int, error) {
 	return n, err
 }
 
-// InsertEvent 在事件首次出现时写入事件和一条 PENDING 任务。已存在则返回 false。
+// InsertEvent 在同一个事务里写入事件和任务。事件已在且任务也在时返回 false。
+// 只有事件、没有任务的历史行会补建任务。任务插入失败会回滚事件。
 func (s *Store) InsertEvent(ev lineEvent, id string, now time.Time) (bool, error) {
+	var added bool
+	var err error
+	for attempt := 0; attempt < 8; attempt++ {
+		added, err = s.insertEventOnce(ev, id, now)
+		if err == nil || (!errors.Is(err, errRetryTask) && !sqliteBusy(err)) {
+			return added, err
+		}
+	}
+	return added, err
+}
+
+func (s *Store) insertEventOnce(ev lineEvent, id string, now time.Time) (bool, error) {
 	at := now.UTC().Format(time.RFC3339)
 	started := ""
 	if ev.Snapshot != nil {
 		started = ev.Snapshot.StartedAt
 	}
-	res, err := s.db.Exec(`
+	tx, err := s.db.Begin()
+	if err != nil {
+		return false, err
+	}
+	defer func() { _ = tx.Rollback() }()
+	res, err := tx.Exec(`
 INSERT OR IGNORE INTO agent_event (event_id, monitor_type, severity, summary, received_at, source, snapshot_started_at)
 VALUES (?, ?, ?, ?, ?, 'fast_https_monitor', ?)`,
 		id, ev.Type, ev.Severity, ev.Message, at, started)
@@ -136,13 +230,40 @@ VALUES (?, ?, ?, ?, ?, 'fast_https_monitor', ?)`,
 		return false, err
 	}
 	if n == 0 {
-		return false, nil
+		var count int
+		if err := tx.QueryRow(`SELECT COUNT(*) FROM agent_task WHERE event_id = ?`, id).Scan(&count); err != nil {
+			return false, err
+		}
+		if count > 0 {
+			return false, tx.Commit()
+		}
 	}
-	_, err = s.db.Exec(`
-INSERT INTO agent_task (task_id, event_id, status, attempts, created_at, updated_at)
-VALUES (?, ?, 'PENDING', 0, ?, ?)`,
-		"t_"+id, id, at, at)
-	return err == nil, err
+	if err := insertTaskRow(tx, "t_"+id, id, at); err != nil {
+		if sqliteConstraint(err) {
+			return false, nil
+		}
+		return false, err
+	}
+	if err := tx.Commit(); err != nil {
+		return false, err
+	}
+	return true, nil
+}
+
+func sqliteBusy(err error) bool {
+	if err == nil {
+		return false
+	}
+	msg := strings.ToLower(err.Error())
+	return strings.Contains(msg, "locked") || strings.Contains(msg, "busy")
+}
+
+func sqliteConstraint(err error) bool {
+	if err == nil {
+		return false
+	}
+	msg := strings.ToLower(err.Error())
+	return strings.Contains(msg, "constraint") || strings.Contains(msg, "unique")
 }
 
 func (s *Store) Audit(kind, payload string, now time.Time) error {
diff --git a/agent/store_event_test.go b/agent/store_event_test.go
new file mode 100644
index 0000000..134cd9b
--- /dev/null
+++ b/agent/store_event_test.go
@@ -0,0 +1,162 @@
+package main
+
+import (
+	"database/sql"
+	"errors"
+	"sync"
+	"testing"
+	"time"
+)
+
+func TestInsertEventAndTaskTogether(t *testing.T) {
+	dir := t.TempDir()
+	st := mustStore(t, dir)
+	defer st.Close()
+	ev := lineEvent{Type: "5xx_ratio", Severity: "warning", Message: "up"}
+	added, err := st.InsertEvent(ev, "abc", time.Now())
+	if err != nil || !added {
+		t.Fatalf("added=%v err=%v", added, err)
+	}
+	added, err = st.InsertEvent(ev, "abc", time.Now())
+	if err != nil || added {
+		t.Fatalf("duplicate added=%v err=%v", added, err)
+	}
+	var tasks int
+	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_task WHERE event_id = 'abc'`).Scan(&tasks); err != nil {
+		t.Fatal(err)
+	}
+	if tasks != 1 {
+		t.Fatalf("tasks = %d", tasks)
+	}
+}
+
+func TestTaskInsertFailureRollsBackEvent(t *testing.T) {
+	prev := insertTaskRow
+	insertTaskRow = func(tx *sql.Tx, taskID, eventID, at string) error {
+		return errors.New("task insert failed")
+	}
+	t.Cleanup(func() { insertTaskRow = prev })
+	dir := t.TempDir()
+	st := mustStore(t, dir)
+	defer st.Close()
+	_, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "up"}, "def", time.Now())
+	if err == nil {
+		t.Fatal("expected task insert error")
+	}
+	var events int
+	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_event WHERE event_id = 'def'`).Scan(&events); err != nil {
+		t.Fatal(err)
+	}
+	if events != 0 {
+		t.Fatalf("event committed without a task: %d", events)
+	}
+}
+
+func TestOrphanEventIsRepaired(t *testing.T) {
+	dir := t.TempDir()
+	st := mustStore(t, dir)
+	defer st.Close()
+	if _, err := st.db.Exec(`INSERT INTO agent_event (event_id, monitor_type, summary, received_at, source) VALUES ('old', '5xx_ratio', 'up', '2026-11-14T00:00:00Z', 'test')`); err != nil {
+		t.Fatal(err)
+	}
+	added, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "up"}, "old", time.Now())
+	if err != nil || !added {
+		t.Fatalf("added=%v err=%v", added, err)
+	}
+	var tasks int
+	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_task WHERE event_id = 'old'`).Scan(&tasks); err != nil {
+		t.Fatal(err)
+	}
+	if tasks != 1 {
+		t.Fatalf("tasks = %d", tasks)
+	}
+}
+
+func TestInsertEventRetriesTransientFailure(t *testing.T) {
+	prev := insertTaskRow
+	var calls int
+	insertTaskRow = func(tx *sql.Tx, taskID, eventID, at string) error {
+		calls++
+		if calls == 1 {
+			return errRetryTask
+		}
+		return prev(tx, taskID, eventID, at)
+	}
+	t.Cleanup(func() { insertTaskRow = prev })
+	dir := t.TempDir()
+	st := mustStore(t, dir)
+	defer st.Close()
+	added, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "up"}, "retry", time.Now())
+	if err != nil || !added {
+		t.Fatalf("added=%v err=%v", added, err)
+	}
+	if calls != 2 {
+		t.Fatalf("attempts = %d", calls)
+	}
+}
+
+func TestConcurrentInsertCreatesOneTask(t *testing.T) {
+	dir := t.TempDir()
+	st := mustStore(t, dir)
+	defer st.Close()
+	var wg sync.WaitGroup
+	errs := make(chan error, 8)
+	for i := 0; i < 8; i++ {
+		wg.Add(1)
+		go func() {
+			defer wg.Done()
+			_, err := st.InsertEvent(lineEvent{Type: "5xx_ratio", Message: "up"}, "same", time.Now())
+			errs <- err
+		}()
+	}
+	wg.Wait()
+	close(errs)
+	for err := range errs {
+		if err != nil {
+			t.Fatal(err)
+		}
+	}
+	var tasks int
+	if err := st.db.QueryRow(`SELECT COUNT(*) FROM agent_task WHERE event_id = 'same'`).Scan(&tasks); err != nil {
+		t.Fatal(err)
+	}
+	if tasks != 1 {
+		t.Fatalf("tasks = %d", tasks)
+	}
+}
+
+func TestWaitingApprovalWithoutPlanBecomesHuman(t *testing.T) {
+	dir := t.TempDir()
+	db, err := sql.Open("sqlite", dir+"/agent.db")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, err := db.Exec(`
+CREATE TABLE agent_task (task_id TEXT PRIMARY KEY, event_id TEXT, status TEXT, attempts INTEGER, context_json TEXT, created_at TEXT, updated_at TEXT);
+CREATE TABLE agent_plan (plan_id TEXT PRIMARY KEY, task_id TEXT, event_id TEXT, risk TEXT, plan_hash TEXT, steps_json TEXT, needs_approval INTEGER);
+CREATE TABLE agent_approval (plan_id TEXT PRIMARY KEY, status TEXT, reason TEXT, created_at TEXT);
+INSERT INTO agent_task VALUES ('t1', 'e1', 'WAITING_APPROVAL', 0, '{}', 't', 't');
+INSERT INTO agent_plan VALUES ('p1', 't1', 'e1', 'low', 'h', '[]', 1);
+INSERT INTO agent_approval VALUES ('p1', 'pending', 'look at the certificate', 't');
+`); err != nil {
+		t.Fatal(err)
+	}
+	_ = db.Close()
+	st := mustStore(t, dir)
+	defer st.Close()
+	status, err := st.taskStatus("t1")
+	if err != nil || status != "WAITING_HUMAN" {
+		t.Fatalf("status = %s err=%v", status, err)
+	}
+	var approval string
+	if err := st.db.QueryRow(`SELECT status FROM agent_approval WHERE plan_id = 'p1'`).Scan(&approval); err != nil {
+		t.Fatal(err)
+	}
+	if approval != "expired" {
+		t.Fatalf("approval = %s", approval)
+	}
+	status, err = st.taskStatus("t1")
+	if err != nil || status != "WAITING_HUMAN" {
+		t.Fatalf("second open status = %s err=%v", status, err)
+	}
+}
diff --git a/cmd/agent_approve.go b/cmd/agent_approve.go
index 0d2b005..cf90f23 100644
--- a/cmd/agent_approve.go
+++ b/cmd/agent_approve.go
@@ -1,10 +1,12 @@
 package cmd
 
 import (
+	"context"
 	"crypto/sha256"
 	"database/sql"
 	"encoding/hex"
 	"encoding/json"
+	"errors"
 	"fmt"
 	"os"
 	"path/filepath"
@@ -14,12 +16,19 @@ import (
 	"fast-https/config"
 	"fast-https/management/agentview"
 	"fast-https/management/operation"
+	"fast-https/management/rtclient"
 	"fast-https/modules/proxy"
 
 	"github.com/spf13/cobra"
 	_ "modernc.org/sqlite"
 )
 
+// 与 agent/plan.go 的常量保持一致。cmd 不能引用 Agent 进程。
+const (
+	approveVerify    = "重新读取 /runtime 与 upstream 名单，不自动执行"
+	approveOnFailure = "停止自动尝试并保留待审批记录"
+)
+
 func newAgentApproveCmd() *cobra.Command {
 	c := &cobra.Command{
 		Use:   "approve <plan-id>",
@@ -38,6 +47,17 @@ func newAgentApproveCmd() *cobra.Command {
 	return c
 }
 
+type approvePlan struct {
+	eventID string
+	hash    string
+	steps   []string
+	status  string
+	taskID  string
+	risk    string
+	verify  string
+	failure string
+}
+
 func runAgentApprove(cmd *cobra.Command, planID string) error {
 	yes, _ := cmd.Flags().GetBool("yes")
 	if !yes {
@@ -54,34 +74,234 @@ func runAgentApprove(cmd *cobra.Command, planID string) error {
 		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
 	}
 	defer db.Close()
-
-	var eventID, storedHash, stepsRaw, approvalStatus, taskID string
-	err = db.QueryRow(`
-SELECT p.event_id, p.plan_hash, p.steps_json, a.status, p.task_id
-FROM agent_plan p JOIN agent_approval a ON a.plan_id = p.plan_id
-WHERE p.plan_id = ?`, planID).Scan(&eventID, &storedHash, &stepsRaw, &approvalStatus, &taskID)
-	if err != nil {
-		return &CLIError{Command: "agent.approve", Code: CodeNotFound, Message: "plan not found: " + err.Error(), Cause: err}
+	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
+		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
 	}
-	if approvalStatus != "pending" {
-		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is not pending"}
+	if err := ensureApprovalColumns(db); err != nil {
+		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
 	}
-	var steps []string
-	if json.Unmarshal([]byte(stepsRaw), &steps) != nil {
-		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "plan steps are not a string list"}
+	plan, err := loadApprovePlan(db, planID)
+	if err != nil {
+		return err
 	}
-	if planHash(eventID, steps) != storedHash {
+	if planHash(plan.eventID, plan.risk, plan.verify, plan.failure, plan.steps) != plan.hash {
 		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan hash does not match the stored steps"}
 	}
-	action, host, ok := parseDrainStep(steps)
+	action, host, ok := parseDrainStep(plan.steps)
 	if !ok {
 		return &CLIError{Command: "agent.approve", Code: CodeNotSupported, Message: "only one upstream drain or undrain step can be approved"}
 	}
+	switch plan.status {
+	case "approved", "rejected", "expired":
+		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is not pending"}
+	case "executing":
+		return finishExecutingApproval(cmd, db, planID, plan.taskID, action, host)
+	case "pending":
+	default:
+		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is not pending"}
+	}
 
 	cfgPath, _ := cmd.Flags().GetString("config")
 	if cfgPath == "" {
 		cfgPath = config.CONFIG_FILE_PATH
 	}
+	if err := recheckDrain(action, host, cfgPath); err != nil {
+		return err
+	}
+	claimed, err := claimApproval(db, planID)
+	if err != nil {
+		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
+	}
+	if !claimed {
+		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is already being approved"}
+	}
+	return executeClaimedApproval(cmd, db, planID, plan.taskID, action, host, cfgPath)
+}
+
+func loadApprovePlan(db *sql.DB, planID string) (approvePlan, error) {
+	var plan approvePlan
+	var stepsRaw string
+	err := db.QueryRow(`
+SELECT p.event_id, p.plan_hash, p.steps_json, a.status, p.task_id,
+       IFNULL(p.risk, ''), IFNULL(p.verify_text, ''), IFNULL(p.on_failure, '')
+FROM agent_plan p JOIN agent_approval a ON a.plan_id = p.plan_id
+WHERE p.plan_id = ?`, planID).Scan(
+		&plan.eventID, &plan.hash, &stepsRaw, &plan.status, &plan.taskID, &plan.risk, &plan.verify, &plan.failure)
+	if err != nil {
+		return approvePlan{}, &CLIError{Command: "agent.approve", Code: CodeNotFound, Message: "plan not found: " + err.Error(), Cause: err}
+	}
+	if json.Unmarshal([]byte(stepsRaw), &plan.steps) != nil {
+		return approvePlan{}, &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: "plan steps are not a string list"}
+	}
+	return plan, nil
+}
+
+func recheckDrain(action, host, cfgPath string) error {
+	if action != "drain" {
+		groups, err := config.ProxyPeerGroups(cfgPath)
+		if err != nil {
+			return &CLIError{Command: "agent.approve", Code: CodeConfigInvalid, Message: err.Error(), Cause: err}
+		}
+		if !peerKnown(groups, host) {
+			return &CLIError{Command: "agent.approve", Code: CodeNotFound, Message: "upstream address is not in the configuration"}
+		}
+		return nil
+	}
+	groups, err := config.ProxyPeerGroups(cfgPath)
+	if err != nil {
+		return &CLIError{Command: "agent.approve", Code: CodeConfigInvalid, Message: err.Error(), Cause: err}
+	}
+	if err := proxy.EvaluateDrain(groups, proxy.DrainedList(), host); err != nil {
+		code := CodeNotFound
+		if err == proxy.ErrLastUpstream {
+			code = CodeOperationFailed
+		}
+		return &CLIError{Command: "agent.approve", Code: code, Message: err.Error(), Cause: err}
+	}
+	rows, err := currentUpstreams()
+	if err != nil {
+		return &CLIError{Command: "agent.approve", Code: CodeServiceDown, Message: "runtime snapshot is required before drain", Cause: err}
+	}
+	if err := drainStillAllowed(rows, host); err != nil {
+		code := CodeOperationFailed
+		if errors.Is(err, proxy.ErrUpstreamNotFound) {
+			code = CodeNotFound
+		}
+		return &CLIError{Command: "agent.approve", Code: code, Message: err.Error(), Cause: err}
+	}
+	return nil
+}
+
+func peerKnown(groups [][]string, host string) bool {
+	for _, group := range groups {
+		for _, peer := range group {
+			if proxy.NormalizePeer(peer) == host {
+				return true
+			}
+		}
+	}
+	return false
+}
+
+func currentUpstreams() ([]rtclient.UpstreamAddrInfo, error) {
+	snap, err := rtclient.Default().Get(context.Background())
+	if err != nil {
+		return nil, err
+	}
+	if snap.Upstream == nil {
+		return nil, errors.New("runtime snapshot has no upstream section")
+	}
+	return snap.Upstream.Upstreams, nil
+}
+
+func drainStillAllowed(rows []rtclient.UpstreamAddrInfo, host string) error {
+	seen := false
+	state := ""
+	drained := false
+	healthy := 0
+	for _, row := range rows {
+		kind := row.Health
+		if kind == "" {
+			if row.Healthy {
+				kind = "healthy"
+			} else {
+				kind = "unhealthy"
+			}
+		}
+		if row.Address == host {
+			seen = true
+			state = kind
+			drained = row.Drained
+		}
+		if kind == "healthy" && row.Address != host && !row.Drained {
+			healthy++
+		}
+	}
+	if !seen {
+		return proxy.ErrUpstreamNotFound
+	}
+	if drained {
+		return nil
+	}
+	if state != "unhealthy" {
+		return errors.New("upstream is not currently unhealthy")
+	}
+	if healthy < 1 {
+		return errors.New("no other healthy upstream")
+	}
+	return nil
+}
+
+func claimApproval(db *sql.DB, planID string) (bool, error) {
+	now := time.Now().UTC().Format(time.RFC3339)
+	res, err := db.Exec(`
+UPDATE agent_approval SET status = 'executing', actor = 'cli', decided_at = ?
+WHERE plan_id = ? AND status = 'pending'`, now, planID)
+	if err != nil {
+		return false, err
+	}
+	n, err := res.RowsAffected()
+	return n == 1, err
+}
+
+func executeClaimedApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host, cfgPath string) error {
+	store := operation.Store{Dir: stateDir()}
+	rec, err := store.Begin(operation.BeginInput{
+		RequestID: "approve:" + planID,
+		Command:   "agent.approve",
+		Args:      map[string]string{"plan_id": planID, "action": action, "address": host},
+		Phase:     "prepared",
+	})
+	if err != nil {
+		_, _ = db.Exec(`UPDATE agent_approval SET status = 'pending' WHERE plan_id = ? AND status = 'executing'`, planID)
+		return fromStateError("agent.approve", err)
+	}
+	if rec.Status == operation.StatusSucceeded {
+		return completeApproval(cmd, db, planID, taskID, action, host, true)
+	}
+	if alreadyInState(action, host) {
+		_, _ = store.Finish(rec, operation.StatusSucceeded, "already", map[string]any{"address": host, "plan_id": planID, "applied": false}, nil)
+		return completeApproval(cmd, db, planID, taskID, action, host, true)
+	}
+	if err := applyApprovedDrain(action, host, cfgPath); err != nil {
+		_, _ = store.Finish(rec, operation.StatusFailed, "failed", nil, &operation.ErrorBody{Message: "drain change failed"})
+		_, _ = db.Exec(`UPDATE agent_approval SET status = 'pending' WHERE plan_id = ? AND status = 'executing'`, planID)
+		return err
+	}
+	if _, err := store.Finish(rec, operation.StatusSucceeded, "committed", map[string]any{"address": host, "plan_id": planID, "applied": true}, nil); err != nil {
+		return fromStateError("agent.approve", err)
+	}
+	return completeApproval(cmd, db, planID, taskID, action, host, false)
+}
+
+func finishExecutingApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host string) error {
+	if !alreadyInState(action, host) {
+		now := time.Now().UTC().Format(time.RFC3339)
+		_, _ = db.Exec(`UPDATE agent_task SET status = 'WAITING_HUMAN', updated_at = ? WHERE task_id = ?`, now, taskID)
+		store := operation.Store{Dir: stateDir()}
+		if rec, found, err := store.Lookup("approve:" + planID); err == nil && found && rec.Status == operation.StatusRunning {
+			_, _ = store.Finish(rec, operation.StatusUnknown, "unconfirmed", nil, &operation.ErrorBody{Message: "result not confirmed"})
+		}
+		return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "approval is executing and the upstream state is not confirmed"}
+	}
+	return completeApproval(cmd, db, planID, taskID, action, host, true)
+}
+
+func alreadyInState(action, host string) bool {
+	found := false
+	for _, item := range proxy.DrainedList() {
+		if item == host {
+			found = true
+			break
+		}
+	}
+	if action == "drain" {
+		return found
+	}
+	return !found
+}
+
+func applyApprovedDrain(action, host, cfgPath string) error {
 	groups, err := config.ProxyPeerGroups(cfgPath)
 	if err != nil {
 		return &CLIError{Command: "agent.approve", Code: CodeConfigInvalid, Message: err.Error(), Cause: err}
@@ -109,33 +329,85 @@ WHERE p.plan_id = ?`, planID).Scan(&eventID, &storedHash, &stepsRaw, &approvalSt
 	if err := proxy.SaveDrained(next); err != nil {
 		return &CLIError{Command: "agent.approve", Code: CodeOperationFailed, Message: err.Error(), Cause: err}
 	}
+	return nil
+}
+
+func completeApproval(cmd *cobra.Command, db *sql.DB, planID, taskID, action, host string, idempotent bool) error {
 	now := time.Now().UTC().Format(time.RFC3339)
-	if _, err := db.Exec(`UPDATE agent_approval SET status = 'approved' WHERE plan_id = ?`, planID); err != nil {
+	res, err := db.Exec(`UPDATE agent_approval SET status = 'approved' WHERE plan_id = ? AND status = 'executing'`, planID)
+	if err != nil {
 		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
 	}
-	_, _ = db.Exec(`UPDATE agent_task SET status = 'COMPLETED', updated_at = ? WHERE task_id = ?`, now, taskID)
-	store := operation.Store{Dir: stateDir()}
-	rec, err := store.Begin(operation.BeginInput{
-		Command: "agent.approve",
-		Args:    map[string]string{"plan_id": planID, "step": action + " " + host},
-		Phase:   "started",
-	})
-	if err == nil && rec.Status == operation.StatusRunning {
-		_, _ = store.Finish(rec, operation.StatusSucceeded, "committed", map[string]any{"address": host, "plan_id": planID}, nil)
+	if n, _ := res.RowsAffected(); n != 1 {
+		var status string
+		_ = db.QueryRow(`SELECT status FROM agent_approval WHERE plan_id = ?`, planID).Scan(&status)
+		if status != "approved" {
+			return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is not pending"}
+		}
+	}
+	if _, err := db.Exec(`UPDATE agent_task SET status = 'COMPLETED', updated_at = ? WHERE task_id = ?`, now, taskID); err != nil {
+		return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
 	}
+	data := map[string]any{"plan_id": planID, "address": host, "action": action, "idempotent": idempotent}
 	if jsonMode(cmd) {
-		return emitOK(cmd, "agent.approve", map[string]any{"plan_id": planID, "address": host, "action": action}, nil)
+		return emitOK(cmd, "agent.approve", data, nil)
 	}
 	fmt.Fprintf(cmd.OutOrStdout(), "approved %s %s\n", action, host)
 	return nil
 }
 
-func planHash(eventID string, steps []string) string {
+func ensureApprovalColumns(db *sql.DB) error {
+	for _, col := range []struct{ table, name, decl string }{
+		{"agent_plan", "verify_text", "TEXT"},
+		{"agent_plan", "on_failure", "TEXT"},
+		{"agent_approval", "decided_at", "TEXT"},
+		{"agent_approval", "actor", "TEXT"},
+	} {
+		rows, err := db.Query(`PRAGMA table_info(` + col.table + `)`)
+		if err != nil {
+			return err
+		}
+		found := false
+		for rows.Next() {
+			var cid, notNull, pk int
+			var name, typ string
+			var dflt sql.NullString
+			if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
+				rows.Close()
+				return err
+			}
+			if name == col.name {
+				found = true
+			}
+		}
+		if err := rows.Err(); err != nil {
+			rows.Close()
+			return err
+		}
+		rows.Close()
+		if found {
+			continue
+		}
+		if _, err := db.Exec(`ALTER TABLE ` + col.table + ` ADD COLUMN ` + col.name + ` ` + col.decl); err != nil {
+			return err
+		}
+	}
+	return nil
+}
+
+// planHash 覆盖事件、步骤、风险、校验方式和失败策略。结构体字段顺序固定。
+func planHash(eventID, risk, verify, onFailure string, steps []string) string {
 	if steps == nil {
 		steps = []string{}
 	}
-	raw, _ := json.Marshal(steps)
-	sum := sha256.Sum256(append([]byte(eventID+"\n"), raw...))
+	raw, _ := json.Marshal(struct {
+		EventID   string   `json:"event_id"`
+		OnFailure string   `json:"on_failure"`
+		Risk      string   `json:"risk"`
+		Steps     []string `json:"steps"`
+		Verify    string   `json:"verify"`
+	}{eventID, onFailure, risk, steps, verify})
+	sum := sha256.Sum256(raw)
 	return hex.EncodeToString(sum[:])
 }
 
diff --git a/cmd/cli_agent_test.go b/cmd/cli_agent_test.go
index 51d58e5..00e6094 100644
--- a/cmd/cli_agent_test.go
+++ b/cmd/cli_agent_test.go
@@ -3,9 +3,12 @@ package cmd
 import (
 	"database/sql"
 	"encoding/json"
+	"net/http"
+	"net/http/httptest"
 	"os"
 	"path/filepath"
 	"strings"
+	"sync"
 	"testing"
 
 	"fast-https/management/agentview"
@@ -100,6 +103,13 @@ func TestAgentApproveDrain(t *testing.T) {
 	t.Setenv("FASTHTTPS_UPSTREAM_DRAIN_FILE", drainFile)
 	t.Setenv("FASTHTTPS_STATE_DIR", dir)
 	t.Setenv(agentview.EnvDir, dir)
+	serveApproveRuntime(t, `{
+	  "schema_version":"1",
+	  "upstream":{"upstreams":[
+	    {"address":"10.0.0.1:8080","health":"unhealthy","healthy":false},
+	    {"address":"10.0.0.2:8080","health":"healthy","healthy":true}
+	  ]}
+	}`)
 	steps := []string{"upstream drain 10.0.0.1:8080"}
 	writeApprovablePlan(t, filepath.Join(dir, "agent.db"), "e-approve", steps)
 
@@ -156,24 +166,111 @@ func writeApprovablePlan(t *testing.T, path, eventID string, steps []string) {
 		t.Fatal(err)
 	}
 	defer db.Close()
-	hash := planHash(eventID, steps)
+	hash := planHash(eventID, "low", approveVerify, approveOnFailure, steps)
 	if _, err := db.Exec(`
 CREATE TABLE agent_event (event_id TEXT PRIMARY KEY, monitor_type TEXT, severity TEXT, summary TEXT, received_at TEXT, source TEXT, snapshot_started_at TEXT);
 CREATE TABLE agent_task (task_id TEXT PRIMARY KEY, event_id TEXT, status TEXT, attempts INTEGER, context_json TEXT, created_at TEXT, updated_at TEXT);
-CREATE TABLE agent_plan (plan_id TEXT PRIMARY KEY, task_id TEXT, event_id TEXT, risk TEXT, plan_hash TEXT, steps_json TEXT, needs_approval INTEGER);
-CREATE TABLE agent_approval (plan_id TEXT PRIMARY KEY, status TEXT, reason TEXT, created_at TEXT);
+CREATE TABLE agent_plan (plan_id TEXT PRIMARY KEY, task_id TEXT, event_id TEXT, risk TEXT, plan_hash TEXT, steps_json TEXT, needs_approval INTEGER, verify_text TEXT, on_failure TEXT);
+CREATE TABLE agent_approval (plan_id TEXT PRIMARY KEY, status TEXT, reason TEXT, created_at TEXT, decided_at TEXT, actor TEXT);
 `); err != nil {
 		t.Fatal(err)
 	}
 	if _, err := db.Exec(`INSERT INTO agent_task VALUES ('t1', ?, 'WAITING_APPROVAL', 0, '{}', '2026-11-10T00:00:00Z', '2026-11-10T00:00:01Z')`, eventID); err != nil {
 		t.Fatal(err)
 	}
-	if _, err := db.Exec(`INSERT INTO agent_plan VALUES (?, 't1', ?, 'low', ?, ?, 1)`, "p_"+eventID, eventID, hash, string(rawSteps)); err != nil {
+	if _, err := db.Exec(`INSERT INTO agent_plan VALUES (?, 't1', ?, 'low', ?, ?, 1, ?, ?)`, "p_"+eventID, eventID, hash, string(rawSteps), approveVerify, approveOnFailure); err != nil {
+		t.Fatal(err)
+	}
+	if _, err := db.Exec(`INSERT INTO agent_approval (plan_id, status, reason, created_at) VALUES (?, 'pending', 'drain one peer', '2026-11-10T00:00:02Z')`, "p_"+eventID); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func serveApproveRuntime(t *testing.T, body string) {
+	t.Helper()
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		_, _ = w.Write([]byte(body))
+	}))
+	t.Cleanup(srv.Close)
+	t.Setenv("FASTHTTPS_RUNTIME_URL", srv.URL)
+}
+
+func TestApproveClaimIsSingleWinner(t *testing.T) {
+	dir := t.TempDir()
+	dbPath := filepath.Join(dir, "agent.db")
+	writeApprovablePlan(t, dbPath, "e-race", []string{"upstream drain 10.0.0.1:8080"})
+	db, err := sql.Open("sqlite", dbPath)
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer db.Close()
+	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
 		t.Fatal(err)
 	}
-	if _, err := db.Exec(`INSERT INTO agent_approval VALUES (?, 'pending', 'drain one peer', '2026-11-10T00:00:02Z')`, "p_"+eventID); err != nil {
+	var wg sync.WaitGroup
+	wins := 0
+	var mu sync.Mutex
+	for i := 0; i < 2; i++ {
+		wg.Add(1)
+		go func() {
+			defer wg.Done()
+			conn, err := sql.Open("sqlite", dbPath)
+			if err != nil {
+				t.Error(err)
+				return
+			}
+			defer conn.Close()
+			_, _ = conn.Exec(`PRAGMA busy_timeout = 5000`)
+			ok, err := claimApproval(conn, "p_e-race")
+			if err != nil {
+				t.Error(err)
+				return
+			}
+			if ok {
+				mu.Lock()
+				wins++
+				mu.Unlock()
+			}
+		}()
+	}
+	wg.Wait()
+	if wins != 1 {
+		t.Fatalf("claims = %d, want 1", wins)
+	}
+}
+
+func TestApproveRejectsStaleHealth(t *testing.T) {
+	dir := t.TempDir()
+	cfgPath := filepath.Join(dir, "fast-https.json")
+	body := `{
+	  "http": {"server": [{
+	    "listen": "80",
+	    "server_name": "a.example",
+	    "location": [{"url": "/", "type": "proxy", "proxy_pass": ["http://10.0.0.1:8080", "http://10.0.0.2:8080"]}]
+	  }]}
+	}`
+	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
 		t.Fatal(err)
 	}
+	drainFile := filepath.Join(dir, "drained.json")
+	t.Setenv("FASTHTTPS_UPSTREAM_DRAIN_FILE", drainFile)
+	t.Setenv("FASTHTTPS_STATE_DIR", dir)
+	t.Setenv(agentview.EnvDir, dir)
+	serveApproveRuntime(t, `{
+	  "schema_version":"1",
+	  "upstream":{"upstreams":[
+	    {"address":"10.0.0.1:8080","health":"unknown"},
+	    {"address":"10.0.0.2:8080","health":"healthy","healthy":true}
+	  ]}
+	}`)
+	writeApprovablePlan(t, filepath.Join(dir, "agent.db"), "e-stale", []string{"upstream drain 10.0.0.1:8080"})
+	_, _, code := runCLI(t, "agent", "approve", "p_e-stale", "--config", cfgPath, "--yes", "--json")
+	if code != 6 {
+		t.Fatalf("exit = %d, want 6", code)
+	}
+	if _, err := os.Stat(drainFile); !os.IsNotExist(err) {
+		t.Fatal("stale health wrote the drain list")
+	}
 }
 
 func writeAgentFixture(t *testing.T, path string) {
diff --git a/management/rtclient/client.go b/management/rtclient/client.go
index b1bf67b..c1f60d9 100644
--- a/management/rtclient/client.go
+++ b/management/rtclient/client.go
@@ -164,6 +164,8 @@ type UpstreamAddrInfo struct {
 	SendTimeoutTotal uint64 `json:"send_timeout_total"`
 	ReadTimeoutTotal uint64 `json:"read_timeout_total"`
 	Healthy          bool   `json:"healthy"`
+	Health           string `json:"health,omitempty"`
+	Drained          bool   `json:"drained,omitempty"`
 }
 
 // UptimeSeconds 由 started_at 推算。解析失败则省略。
diff --git a/modules/proxy/event_proxy.go b/modules/proxy/event_proxy.go
index 4b75309..d56435e 100644
--- a/modules/proxy/event_proxy.go
+++ b/modules/proxy/event_proxy.go
@@ -344,6 +344,7 @@ func (p *Proxy) getDataFromServer(ev *core.Event, req_data []byte) (int, error)
 	headCode := p.ChangeHeader(ev.RR.Req.IsKeepalive(), resData, &ev.RR)
 	status, _ := strconv.Atoi(headCode)
 	logProxyAccess(ev, status, len(ev.RR.Res.Body))
+	noteUpstreamOK(p.ProxyAddr)
 
 	return status, nil
 }
diff --git a/modules/proxy/health_test.go b/modules/proxy/health_test.go
new file mode 100644
index 0000000..54226b8
--- /dev/null
+++ b/modules/proxy/health_test.go
@@ -0,0 +1,81 @@
+package proxy
+
+import (
+	"path/filepath"
+	"testing"
+	"time"
+
+	"fast-https/config"
+)
+
+func TestHealthUsesRecentWindow(t *testing.T) {
+	ResetMetricsForTest()
+	t.Cleanup(ResetMetricsForTest)
+	t.Setenv("FASTHTTPS_UPSTREAM_DRAIN_FILE", filepath.Join(t.TempDir(), "drained.json"))
+	now := time.Date(2026, 11, 14, 1, 0, 0, 0, time.UTC)
+	healthNow = func() time.Time { return now }
+	t.Cleanup(func() { healthNow = time.Now })
+
+	cfg := config.Current()
+	t.Cleanup(func() { config.Store(cfg, nil) })
+	config.Store(&config.Fast_Https{Servers: []config.Server{{
+		Path: []config.Path{{
+			ProxyPeers: []string{"10.0.0.1:8080", "10.0.0.2:8080", "10.0.0.3:8080", "10.0.0.4:8080"},
+		}},
+	}}}, nil)
+
+	noteUpstreamOK("10.0.0.1:8080")
+	noteDialFail("10.0.0.2:8080")
+	noteDialFail("10.0.0.3:8080")
+	if err := SaveDrained([]string{"10.0.0.2:8080"}); err != nil {
+		t.Fatal(err)
+	}
+
+	got := healthByAddr(t)
+	if got["10.0.0.1:8080"].Health != "healthy" || !got["10.0.0.1:8080"].Healthy {
+		t.Fatalf("recent success = %+v", got["10.0.0.1:8080"])
+	}
+	if got["10.0.0.2:8080"].Health != "unhealthy" || !got["10.0.0.2:8080"].Drained {
+		t.Fatalf("drained failure = %+v", got["10.0.0.2:8080"])
+	}
+	if got["10.0.0.4:8080"].Health != "unknown" || got["10.0.0.4:8080"].Healthy {
+		t.Fatalf("no samples = %+v", got["10.0.0.4:8080"])
+	}
+
+	healthNow = func() time.Time { return now.Add(2 * time.Minute) }
+	noteUpstreamOK("10.0.0.3:8080")
+	got = healthByAddr(t)
+	if got["10.0.0.1:8080"].Health != "unknown" {
+		t.Fatalf("success outside the window = %+v", got["10.0.0.1:8080"])
+	}
+	if got["10.0.0.2:8080"].Health != "unknown" || got["10.0.0.2:8080"].DialFailTotal != 1 || !got["10.0.0.2:8080"].Drained {
+		t.Fatalf("old failure = %+v", got["10.0.0.2:8080"])
+	}
+	if got["10.0.0.3:8080"].Health != "healthy" {
+		t.Fatalf("recovered = %+v", got["10.0.0.3:8080"])
+	}
+}
+
+func TestRecentFailureIsUnhealthy(t *testing.T) {
+	ResetMetricsForTest()
+	t.Cleanup(ResetMetricsForTest)
+	now := time.Date(2026, 11, 14, 2, 0, 0, 0, time.UTC)
+	healthNow = func() time.Time { return now }
+	t.Cleanup(func() { healthNow = time.Now })
+	noteDialFail("10.1.0.1:8080")
+	noteUpstreamOK("10.1.0.2:8080")
+	got := healthByAddr(t)
+	if got["10.1.0.1:8080"].Health != "unhealthy" || got["10.1.0.2:8080"].Health != "healthy" {
+		t.Fatalf("health = %+v", got)
+	}
+}
+
+func healthByAddr(t *testing.T) map[string]AddrStats {
+	t.Helper()
+	st, _ := Snapshot()
+	got := map[string]AddrStats{}
+	for _, row := range st.Upstreams {
+		got[row.Address] = row
+	}
+	return got
+}
diff --git a/modules/proxy/metrics.go b/modules/proxy/metrics.go
index 204a601..7e716b9 100644
--- a/modules/proxy/metrics.go
+++ b/modules/proxy/metrics.go
@@ -6,10 +6,19 @@ import (
 	"strings"
 	"sync"
 	"sync/atomic"
+	"time"
+
+	"fast-https/config"
 )
 
 const maxUpstreamAddrs = 64
 
+// healthWindow 是判断“当前”健康所用的最近一段时间。窗口外的失败只留在累计计数里。
+const healthWindow = time.Minute
+
+// healthNow 让测试把失败和恢复放进或移出窗口。
+var healthNow = time.Now
+
 var (
 	dialFailTotal    atomic.Uint64
 	sendTimeoutTotal atomic.Uint64
@@ -20,24 +29,52 @@ var (
 )
 
 type addrCounters struct {
-	dial atomic.Uint64
-	send atomic.Uint64
-	read atomic.Uint64
+	dial     atomic.Uint64
+	send     atomic.Uint64
+	read     atomic.Uint64
+	samples  atomic.Uint64
+	consec   atomic.Uint64
+	lastOK   atomic.Int64
+	lastFail atomic.Int64
 }
 
 func noteDialFail(addr string) {
 	dialFailTotal.Add(1)
-	noteAddr(metricAddr(addr), func(c *addrCounters) { c.dial.Add(1) })
+	noteAddr(metricAddr(addr), func(c *addrCounters) {
+		c.dial.Add(1)
+		noteFailure(c)
+	})
 }
 
 func noteSendTimeout(addr string) {
 	sendTimeoutTotal.Add(1)
-	noteAddr(metricAddr(addr), func(c *addrCounters) { c.send.Add(1) })
+	noteAddr(metricAddr(addr), func(c *addrCounters) {
+		c.send.Add(1)
+		noteFailure(c)
+	})
 }
 
 func noteReadTimeout(addr string) {
 	readTimeoutTotal.Add(1)
-	noteAddr(metricAddr(addr), func(c *addrCounters) { c.read.Add(1) })
+	noteAddr(metricAddr(addr), func(c *addrCounters) {
+		c.read.Add(1)
+		noteFailure(c)
+	})
+}
+
+// noteUpstreamOK 记录一次已经拿到上游响应的请求。拨号或超时失败不走这里。
+func noteUpstreamOK(addr string) {
+	noteAddr(metricAddr(addr), func(c *addrCounters) {
+		c.samples.Add(1)
+		c.consec.Store(0)
+		c.lastOK.Store(healthNow().UnixNano())
+	})
+}
+
+func noteFailure(c *addrCounters) {
+	c.samples.Add(1)
+	c.consec.Add(1)
+	c.lastFail.Store(healthNow().UnixNano())
 }
 
 func noteAddr(host string, add func(*addrCounters)) {
@@ -82,6 +119,10 @@ type AddrStats struct {
 	SendTimeoutTotal uint64 `json:"send_timeout_total"`
 	ReadTimeoutTotal uint64 `json:"read_timeout_total"`
 	Healthy          bool   `json:"healthy"`
+	// Health 是当前窗口内的判断：healthy、unhealthy、unknown。
+	// Healthy 只在 Health 为 healthy 时为 true。累计失败次数不单独决定它。
+	Health  string `json:"health"`
+	Drained bool   `json:"drained"`
 }
 
 // Stats 是 /runtime 的 upstream 分段。没有代理流量时各字段为 0，那是真实的零。
@@ -97,30 +138,51 @@ type Stats struct {
 // Snapshot 供组合根挂进 /runtime。始终 ok=true。
 func Snapshot() (Stats, bool) {
 	addrMu.Lock()
-	hosts := make([]string, 0, len(byAddr))
-	counters := make([]*addrCounters, 0, len(byAddr))
+	counters := make(map[string]*addrCounters, len(byAddr))
 	for host, c := range byAddr {
-		hosts = append(hosts, host)
-		counters = append(counters, c)
+		counters[host] = c
 	}
 	addrMu.Unlock()
 
+	drained := loadDrained()
+	seen := map[string]struct{}{}
+	var hosts []string
+	addHost := func(host string) {
+		if host == "" {
+			return
+		}
+		if _, ok := seen[host]; ok {
+			return
+		}
+		if len(seen) >= maxUpstreamAddrs {
+			return
+		}
+		seen[host] = struct{}{}
+		hosts = append(hosts, host)
+	}
+	for _, host := range configuredMetricAddrs() {
+		addHost(host)
+	}
+	for host := range counters {
+		addHost(host)
+	}
+	sort.Strings(hosts)
+
+	now := healthNow()
 	var rows []AddrStats
 	if len(hosts) > 0 {
-		order := make([]int, len(hosts))
-		for i := range order {
-			order[i] = i
-		}
-		sort.Slice(order, func(i, j int) bool { return hosts[order[i]] < hosts[order[j]] })
-		rows = make([]AddrStats, 0, len(order))
-		for _, i := range order {
-			rows = append(rows, AddrStats{
-				Address:          hosts[i],
-				DialFailTotal:    counters[i].dial.Load(),
-				SendTimeoutTotal: counters[i].send.Load(),
-				ReadTimeoutTotal: counters[i].read.Load(),
-				Healthy:          counters[i].dial.Load()+counters[i].send.Load()+counters[i].read.Load() == 0,
-			})
+		rows = make([]AddrStats, 0, len(hosts))
+		for _, host := range hosts {
+			c := counters[host]
+			state := healthState(c, now)
+			_, isDrained := drained[host]
+			row := AddrStats{Address: host, Health: state, Healthy: state == "healthy", Drained: isDrained}
+			if c != nil {
+				row.DialFailTotal = c.dial.Load()
+				row.SendTimeoutTotal = c.send.Load()
+				row.ReadTimeoutTotal = c.read.Load()
+			}
+			rows = append(rows, row)
 		}
 	}
 	return Stats{
@@ -132,6 +194,55 @@ func Snapshot() (Stats, bool) {
 	}, true
 }
 
+func healthState(c *addrCounters, now time.Time) string {
+	if c == nil || c.samples.Load() == 0 {
+		return "unknown"
+	}
+	okAt := c.lastOK.Load()
+	failAt := c.lastFail.Load()
+	okRecent := okAt > 0 && now.Sub(time.Unix(0, okAt)) <= healthWindow
+	failRecent := failAt > 0 && now.Sub(time.Unix(0, failAt)) <= healthWindow
+	if okRecent && (!failRecent || okAt >= failAt) {
+		return "healthy"
+	}
+	if failRecent && c.consec.Load() > 0 && (!okRecent || failAt > okAt) {
+		return "unhealthy"
+	}
+	return "unknown"
+}
+
+func configuredMetricAddrs() []string {
+	cfg := config.Current()
+	if cfg == nil {
+		return nil
+	}
+	var out []string
+	seen := map[string]struct{}{}
+	add := func(addr string) {
+		host := metricAddr(addr)
+		if host == "" {
+			return
+		}
+		if _, ok := seen[host]; ok {
+			return
+		}
+		seen[host] = struct{}{}
+		out = append(out, host)
+	}
+	for _, server := range cfg.Servers {
+		for _, path := range server.Path {
+			if len(path.ProxyPeers) > 0 {
+				for _, peer := range path.ProxyPeers {
+					add(peer)
+				}
+				continue
+			}
+			add(path.ProxyData)
+		}
+	}
+	return out
+}
+
 // ResetMetricsForTest 清空上游失败计数。空闲连接数是池的当前值，不清。
 func ResetMetricsForTest() {
 	dialFailTotal.Store(0)
