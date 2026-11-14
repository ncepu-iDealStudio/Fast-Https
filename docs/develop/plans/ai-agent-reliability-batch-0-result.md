# Batch 0 执行结果：代码核查

日期：2026-11-14。本批不改业务代码。

核查开始时分支是 `fast-https-ai-agent`，HEAD 是 `017187e986087c40bf2f5711ebdb69e67a89860c`，与方案写明的基线一致。工作区当时只有方案文档处于暂存状态。核查期间该文档已由 `ce13313` 单独提交并推送，其中没有源码改动。源码结论仍以 `017187e` 为准。没有发现本方案里的改造已经落地。

## 1. 和方案的差异

方案对现状的判断成立。下面是对照源码后的具体位置，不是方案原文的复述。

### 已经有的能力

- Agent 是独立进程，`agent/main.go` 的循环只做摄入、证书巡检、采集、诊断。
- SQLite 在 `agent/store.go`，表为 `agent_event`、`agent_task`、`agent_diagnosis`、`agent_plan`、`agent_approval`、`agent_audit`、`agent_offset`。没有 `agent_operation`，也没有 schema 版本表。
- 诊断写入在 `saveDiagnosis` 里使用一个事务。
- `agent approve` 只接受一条 `upstream drain` 或 `upstream undrain`，会重算 `planHash`，最后一个地址时不写摘除名单。
- Server 侧 operation 包已有 `queued`、`running`、`succeeded`、`failed`、`unknown`。`agent approve` 没有使用这套状态来包住真正的摘除动作。

### 尚未实现，和方案列出的问题一致

1. `modules/proxy/metrics.go` 的 `Healthy` 等于该地址累计拨号失败、发送超时、读取超时之和是否为 0。地址只在出错时进入 `byAddr`。没有成功计数，没有时间窗口，没有 UNKNOWN。
2. `InsertEvent` 先 `INSERT OR IGNORE` 事件，再单独 `INSERT` 任务。事件已存在时直接返回，不检查任务是否缺失。两次写入不在同一个事务里。
3. `statusFor` 按故障类型把 `certificate_expiring` 等无步骤分类写成 `WAITING_APPROVAL`。没有 `WAITING_HUMAN`。
4. `runAgentApprove` 先读出 `pending`，执行 `SaveDrained`，再把审批改成 `approved`。没有条件更新，也没有 `EXECUTING`。
5. `makePlan` 和 `planHash` 只哈希 `eventID` 与 `steps` 的 JSON。`Verify`、`OnFailure`、风险和前置条件不参与哈希。
6. operation 记录在 `SaveDrained` 成功之后才 `Begin`，并且忽略 `Begin` 以外的失败。任务更新错误被 `_` 丢掉。
7. `closeRecovered` 用相同的 `snapshot_started_at` 关闭所有未完成的 `unreachable` 任务。
8. `ReadStatus` 在库文件不存在时返回 `not_running`，库能打开时返回 `present`。没有心跳。
9. `readNewLines` 只在文件变短时把偏移归零。
10. `main` 的循环不能取消，`runOnce` 里的子进程和 HTTP 没有统一的退出上下文。
11. `OpenStore` 每次都执行 `ALTER TABLE agent_event ADD COLUMN snapshot_started_at`，并忽略错误。

## 2. 修改前的关键代码

本批没有修改后的代码。

事件和任务分两次写入，重复事件直接跳过：

```120:145:agent/store.go
func (s *Store) InsertEvent(ev lineEvent, id string, now time.Time) (bool, error) {
	// ...
	if n == 0 {
		return false, nil
	}
	_, err = s.db.Exec(`
INSERT INTO agent_task (task_id, event_id, status, attempts, created_at, updated_at)
VALUES (?, ?, 'PENDING', 0, ?, ?)`,
		"t_"+id, id, at, at)
	return err == nil, err
}
```

健康等于累计错误是否为零，且只有出过错的地址才会出现：

```117:123:modules/proxy/metrics.go
rows = append(rows, AddrStats{
	Address:          hosts[i],
	DialFailTotal:    counters[i].dial.Load(),
	SendTimeoutTotal: counters[i].send.Load(),
	ReadTimeoutTotal: counters[i].read.Load(),
	Healthy:          counters[i].dial.Load()+counters[i].send.Load()+counters[i].read.Load() == 0,
})
```

无步骤的审批类故障仍进入 `WAITING_APPROVAL`：

```58:69:agent/policy.go
func statusFor(classification, decision string) string {
	switch classification {
	case "collect_error", "certificate_read_error", "unknown", "service_unavailable":
		return "ESCALATED"
	}
	if decision == policyApproval {
		return "WAITING_APPROVAL"
	}
	// ...
}
```

审批先执行摘除，再改状态；operation 写在动作之后：

```66:125:cmd/agent_approve.go
if approvalStatus != "pending" {
	return &CLIError{Command: "agent.approve", Code: CodeConflict, Message: "plan is not pending"}
}
// ... 校验哈希、执行 SaveDrained ...
if _, err := db.Exec(`UPDATE agent_approval SET status = 'approved' WHERE plan_id = ?`, planID); err != nil {
	return &CLIError{Command: "agent.approve", Code: CodeInternal, Message: err.Error(), Cause: err}
}
_, _ = db.Exec(`UPDATE agent_task SET status = 'COMPLETED', updated_at = ? WHERE task_id = ?`, now, taskID)
store := operation.Store{Dir: stateDir()}
rec, err := store.Begin(operation.BeginInput{ /* ... */ })
```

哈希只覆盖事件 ID 和步骤文本：

```21:27:agent/plan.go
func makePlan(taskID, eventID, classification string, steps []string) Plan {
	raw, _ := json.Marshal(steps)
	sum := sha256.Sum256(append([]byte(eventID+"\n"), raw...))
```

在线判断只看库文件是否能打开：

```62:75:management/agentview/agentview.go
func ReadStatus(dir string) (Status, error) {
	out := Status{Status: "not_running", Source: "agent_db"}
	db, err := openReadOnly(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return Status{}, err
	}
	out.Status = "present"
```

轮转只识别“文件变短”：

```27:29:agent/tail.go
if info.Size() < offset {
	offset = 0
}
```

恢复事件按同一次进程启动时间关闭全部不可达任务：

```81:90:agent/decide.go
UPDATE agent_task SET status = 'COMPLETED', updated_at = ?
WHERE task_id IN (
  SELECT t.task_id FROM agent_task t
  JOIN agent_event e ON e.event_id = t.event_id
  WHERE e.monitor_type = 'unreachable'
    AND e.snapshot_started_at = ?
```

## 3. 需要修改的文件

Batch 1：

- `modules/proxy/metrics.go` 及代理成功路径上能记到一次成功的位置
- `agent/plan.go`、`agent/policy.go`、`agent/decide.go`、`agent/store.go`、`agent/ingest.go`
- `cmd/agent_approve.go`
- 对应 `*_test.go`

Batch 2 再动任务状态转换、执行后验证和 `closeRecovered`。Batch 3 再动 `agent/main.go`、`agent/tail.go`、`agent/runtime.go`、`agent/collect.go`、`management/agentview/agentview.go` 和 schema 迁移。Batch 4 再改文档。

## 4. 数据库迁移

当前没有版本表。`OpenStore` 用 `CREATE TABLE IF NOT EXISTS` 建表，并用一次被忽略错误的 `ALTER TABLE` 补 `snapshot_started_at`。

Batch 1 需要的列，优先加在现有表上，不新建平行表：

- `agent_approval`：能表达执行中的状态；审批人、审批时间可以加列。现有状态值是小写 `pending` / `approved`，新代码要兼容这两个旧值。
- `agent_task`：Batch 1 先增加 `WAITING_HUMAN` 这个状态值。`EXECUTING`、`VERIFYING` 留到 Batch 2，避免两批同时改状态机。
- operation 继续用 Server 已有的 `management/operation` 文件记录。不在 Agent SQLite 里再造一张 `agent_operation`。Batch 1 要把它挪到 `SaveDrained` 之前，并处理已创建但动作未完成的记录。

正式的 `schema_version` 表按方案留在 Batch 3。Batch 1 如果加列，用可重复执行的 `ALTER`，并在 Batch 3 收成版本迁移。这一点是顺序上的风险：Batch 1 不能再增加“忽略错误的 ALTER”而不留记录。实施时会把新增列写成单独函数，Batch 3 再纳入版本号。

## 5. 兼容性

- `/runtime` 的 `healthy` 今天表示“累计错误为 0”。改成时间窗口后，旧消费者会看到不同的布尔值。要额外增加状态字段，而不是删掉 `dial_fail_total` 等累计计数。
- 任务状态会新增 `WAITING_HUMAN`。只认识 `WAITING_APPROVAL` 的查询仍能读到真正有计划的任务。历史库里“待审批但步骤为空”的行需要一次性迁到 `WAITING_HUMAN`，审批行保留。
- `planHash` 一旦把更多字段算进去，已保存的旧哈希会和重算结果不一致，旧计划不能被批准。这是方案要求的安全结果，不是误伤。
- Agent 不能 import `cmd`、`runtime`、`core`、`management`。健康数据必须经 `/runtime` JSON 进入 `drainStep`，不能让 Agent 直接读 `modules/proxy`。
- 没有主动健康检查。本批之后的实现也不能新做探测循环。

## 6. Git diff

相对 `017187e` 的源码 diff 为空。`ce13313` 只新增方案文档，1328 行，没有代码。本批再新增的只有本报告。方案正文不在这里重复粘贴，用 `git show ce13313` 可以取出。

## 7. 新增测试

无。本批按方案不改代码，也不新增测试。

## 8. 实际测试

作为基线执行，不是本批新测试：

```text
go test ./agent/ ./cmd/ ./modules/proxy/ ./management/agentview/ ./management/operation/
ok  fast-https/agent
ok  fast-https/cmd
ok  fast-https/modules/proxy
ok  fast-https/management/operation
management/agentview 没有测试文件
```

## 9. 尚未解决

Batch 1 到 Batch 4 的全部改造项都还没做。下一批只做方案第三节的七项：健康判断、事件与任务事务、`WAITING_HUMAN`、审批原子占用、Plan Hash、Operation 前置、审批幂等。做完后停住，不进入 Batch 2。
