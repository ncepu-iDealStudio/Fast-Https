# Fast-Https AI Agent 最新可靠性改造方案与实施计划

**实施基线：** `fast-https-ai-agent` 分支  
**本轮核查 HEAD：** `017187e986087c40bf2f5711ebdb69e67a89860c`  
**改造类型：** 增量改造，不推翻现有架构  
**总体目标：** 提高 Agent 的运行可靠性、审批安全性和故障处理闭环能力。

---

# 一、项目现状与改造目标

## 1.1 当前项目现状

Fast-Https 已经具备一个初步可用的 AI Agent 架构。

当前主要能力包括：

1. Agent 作为独立进程运行。
2. 从 Monitor JSONL 接收事件。
3. 使用 SQLite 持久化事件、任务、计划和审批等信息。
4. 通过 Runtime HTTP 和固定 CLI 白名单采集上下文。
5. 使用规则进行主要故障诊断。
6. 支持可选的 OpenAI 兼容模型接口，用于辅助分析和解释。
7. Planner 根据故障类型生成有限的处理计划。
8. Policy 提供 ALLOW、REQUIRE_APPROVAL、DENY 三种决策。
9. 支持有限的 upstream drain / undrain 人工审批执行。
10. Management CLI 可以查询 Agent 任务和审批信息。

目前的整体架构是合理的，没有必要推翻重建。

## 1.2 当前发现的主要问题

经过对当前分支代码的静态核查，以下问题仍然需要处理。

### P0：核心可靠性和安全问题

1. upstream Healthy 判断依赖累计错误计数，健康状态数据来源不可靠。
2. Event 和 Task 创建没有使用同一个数据库事务。
3. WAITING_APPROVAL 与需要人工处理的任务语义混淆。
4. 审批流程没有原子占用机制，存在并发重复执行风险。
5. Operation 记录在实际操作完成后才创建，审计和异常恢复不可靠。
6. Plan Hash 没有完整覆盖影响执行安全性的字段。

### P1：执行闭环和运行可靠性问题

1. Task 缺少 EXECUTING、VERIFYING 等状态。
2. drain / undrain 操作缺少完整的执行后验证。
3. recovered 事件与故障任务的关联不够精确。
4. Agent 缺少可靠的心跳机制。
5. Management 通过数据库文件存在判断 Agent 状态。
6. JSONL 日志轮转识别不完整。
7. Agent 主循环缺少完整的优雅退出控制。
8. SQLite 缺少明确的 Schema Version 迁移机制。

## 1.3 本次改造原则

必须遵守：

- 保留现有 Agent 独立进程架构。
- 保留现有 SQLite 存储方案。
- 保留现有 Monitor、Runtime、Management 和 CLI 模块。
- 不引入 Kafka、Redis、RabbitMQ 等消息中间件。
- 不引入复杂工作流引擎。
- 不建设通用 Shell 执行器。
- 不让 LLM 自由生成并执行 Action。
- 不为了架构整洁而大规模调整目录。
- 优先复用已有代码和 Server 能力。
- 不增加没有明确使用场景的抽象层。
- 所有实际操作都必须经过 Policy、审批和执行安全检查。

**本次工作的重点是让现有功能可靠，而不是增加更多 Agent 功能。**

---

# 二、实施前置要求：重新核查当前代码

开始修改之前，必须重新检查当前工作区和 Git 分支。

此前的代码核查基于 HEAD：

`017187e986087c40bf2f5711ebdb69e67a89860c`

但如果当前分支已经产生新提交，应以当前 HEAD 为准。

首先检查：

- 当前分支和 HEAD。
- 工作区是否存在用户尚未提交的修改。
- 当前 Agent 代码和上次核查结果是否一致。
- 是否已经有人实施了部分改造。
- 当前数据库结构和测试覆盖情况。

重点检查以下文件：

```text
agent/
├── main.go
├── runtime.go
├── ingest.go
├── event.go
├── store.go
├── diagnose.go
├── decide.go
├── plan.go
├── policy.go
├── collect.go
└── tail.go

agent/model/

cmd/
├── agent.go
└── agent_approve.go

modules/proxy/
└── metrics.go

management/agentview/
└── agentview.go
```

同时检查：

```text
agent/*_test.go
cmd/*agent*_test.go
modules/proxy/*_test.go

docs/develop/plans/ai-agent-execution-plan.md
docs/develop/plans/ai-agent-server-gap-plan.md
docs/feature/22-ai-agent.md
```

### 前置核查要求

先输出：

1. 当前代码与本方案的差异。
2. 已经实现的改造项。
3. 尚未实现的改造项。
4. 需要修改的文件。
5. 需要执行的数据库迁移。
6. 可能影响现有功能的兼容性问题。

不要重复实现已经完成的功能。

不要覆盖或丢弃用户工作区中的修改。

核查完成后，按下面的批次执行。

---

# 三、第一批：P0 核心可靠性与审批安全改造

本批次是本次改造的重点。

建议单独完成一个开发阶段，完成测试后再进入第二批。

## 任务 1：修复 upstream Healthy 判断机制

**优先级：P0**

涉及：

```text
modules/proxy/metrics.go
agent/plan.go
相关测试文件
```

### 1.1 当前问题

目前 `byAddr` 主要在发生拨号失败、发送超时、读取超时等异常时创建记录。

Healthy 又通过累计错误计数是否为零进行判断。

这会导致：

- 正常 upstream 可能没有统计记录。
- 曾经发生过错误的 upstream 可能一直被判断为不健康。
- Agent 无法可靠地区分正常 upstream 和异常 upstream。
- Planner 的 drain 计划生成可能无法正常工作。

### 1.2 改造目标

明确区分：

- upstream 是否存在。
- upstream 是否启用。
- upstream 是否已经 drained。
- upstream 最近是否发生过错误。
- upstream 当前是否满足健康判断条件。

不能再把历史累计错误次数直接等同于当前健康状态。

### 1.3 实施要求

第一步：检查 upstream 的完整数据来源。

优先使用现有配置、注册信息或运行时状态，确保正常 upstream 也能出现在状态列表中。

不要仅依赖错误统计表生成 upstream 列表。

第二步：检查当前项目是否已经存在主动健康检查机制。

如果已经存在，优先复用。

如果没有，不要为了本次改造直接建设复杂的主动探测系统。

可以先采用最近时间窗口内的成功请求、失败请求和连续失败次数等信息形成明确的运行状态判断。

需要注意：

- 没有请求记录不代表健康。
- 历史失败不代表当前一定不健康。
- 配置存在不代表实际服务健康。
- drained 状态不能直接等同于 unhealthy。

如果当前缺少足够数据，状态应该允许为 UNKNOWN，而不是强行标记为健康。

第三步：调整 `agent/plan.go` 中的 `drainStep()`。

生成 drain 计划必须满足：

1. 目标 upstream 存在。
2. 目标 upstream 当前确实需要摘除。
3. 至少存在一个其他可用 upstream。
4. 目标 upstream 当前尚未 drained。
5. 不会排空最后一个健康 upstream。
6. 健康状态不确定时不能自动生成高风险操作计划。

### 1.4 测试要求

至少覆盖：

- 所有 upstream 正常。
- 一个 upstream 异常，其他 upstream 正常。
- 所有 upstream 异常。
- 一个 upstream 曾经异常但已经恢复。
- upstream 没有请求记录。
- upstream 已经 drained。
- 只有一个 upstream。
- 多个 upstream 中只有一个健康。
- 健康状态为 UNKNOWN。

### 1.5 验收标准

- 正常 upstream 能够被识别。
- 历史累计错误不会永久导致 unhealthy。
- 不会排空最后一个健康 upstream。
- 健康状态不确定时不会生成危险操作计划。
- 不破坏现有 Metrics 和 Runtime 状态输出的兼容性。

---

## 任务 2：修复 Event 和 Task 的事务一致性

**优先级：P0**

涉及：

```text
agent/ingest.go
agent/store.go
agent/event.go
相关测试文件
```

### 2.1 当前问题

目前 Event 和 Task 的创建存在两个独立数据库操作。

如果发生：

1. Event 插入成功。
2. Task 插入失败。
3. Agent 再次处理同一事件。
4. Event 去重逻辑忽略重复事件。

那么 Task 就可能永久丢失。

### 2.2 改造要求

将 Event 和 Task 的创建放在同一个 SQLite 事务中。

建议提供一个统一的 Store 方法：

```go
CreateEventAndTask(...)
```

具体函数名称可以按照现有命名规范调整。

事务流程：

```text
BEGIN TRANSACTION

    INSERT EVENT

    如果 EVENT 已经存在：
        检查对应 TASK 是否存在
        如果 TASK 已存在：
            返回重复事件
        如果 TASK 不存在：
            安全补建 TASK

    如果 EVENT 是新事件：
        INSERT TASK

COMMIT
```

任意数据库操作失败时必须回滚。

### 2.3 注意事项

- 正确处理 SQLite 唯一约束。
- 并发重复事件不能创建多个 Task。
- 不要把数据库错误全部当成重复事件。
- 历史上已经存在 Event 但没有 Task 的记录，应能安全补偿。
- 事务提交失败时不能提前更新内存状态。

### 2.4 测试要求

- Event 和 Task 同时成功创建。
- 重复 Event 不创建重复 Task。
- Task 创建失败时 Event 不会单独提交。
- 历史孤立 Event 能够安全补建 Task。
- 并发提交同一个 Event。
- SQLite 异常后能够安全重试。

---

## 任务 3：区分 WAITING_HUMAN 和 WAITING_APPROVAL

**优先级：P0**

涉及：

```text
agent/store.go
agent/decide.go
agent/plan.go
agent/policy.go
cmd/agent_approve.go
```

### 3.1 当前问题

当前部分故障分类直接返回 REQUIRE_APPROVAL。

但是有些任务实际上没有可执行 Plan。

例如：

证书即将过期，Agent 发现问题，但并没有生成证书替换操作计划。

这类任务不应该进入人工审批队列。

### 3.2 明确两个状态

#### WAITING_HUMAN

含义：

Agent 已经发现问题，但没有可以安全执行的自动化 Plan，需要人工检查或处理。

适用场景：

- 证书即将过期，但没有可执行的续期计划。
- 故障原因不明确。
- 诊断证据不足。
- 需要人工检查运行环境。

#### WAITING_APPROVAL

含义：

Agent 已经生成一个有效的、可以执行的结构化 Plan，但该操作需要人工授权。

适用条件：

- Plan 存在。
- Action 属于系统支持的动作。
- Parameters 合法。
- 前置条件完整。
- Policy 返回 REQUIRE_APPROVAL。

### 3.3 改造要求

不能仅根据故障类型决定 WAITING_APPROVAL。

应该先检查是否存在有效 Plan，再根据 Policy 结果决定任务状态。

建议逻辑：

```text
Diagnosis
    |
    v
Planner
    |
    +-- 无可执行 Plan
    |       |
    |       v
    |   WAITING_HUMAN
    |
    +-- 有效 Plan
            |
            v
          Policy
            |
            +-- DENY
            |      |
            |      v
            |    CLOSED 或现有拒绝状态
            |
            +-- REQUIRE_APPROVAL
            |      |
            |      v
            |  WAITING_APPROVAL
            |
            +-- ALLOW
                   |
                   v
              进入受控执行流程
```

注意：本次不要因此扩大自动执行权限。原来需要审批的操作仍然需要审批。

### 3.4 历史数据兼容

检查历史 SQLite 数据。

对存在 WAITING_APPROVAL 但没有有效 Plan 的任务：

- 可以迁移为 WAITING_HUMAN。
- 必须保留原始事件和诊断信息。
- 不得丢失历史审批记录。
- 迁移必须幂等。

### 3.5 测试要求

- 无 Plan 进入 WAITING_HUMAN。
- 有有效 Plan 且要求审批时进入 WAITING_APPROVAL。
- 无效 Action 不进入审批队列。
- 空 steps 的 Plan 不能通过审批。
- 已拒绝的审批不能再次执行。

---

## 任务 4：审批原子占用与并发保护

**优先级：P0**

涉及：

```text
cmd/agent_approve.go
agent/store.go
agent/plan.go
agent/policy.go
```

### 4.1 当前问题

当前审批流程存在：

```text
读取 PENDING
    |
检查 Plan Hash
    |
执行操作
    |
更新 APPROVED
```

多个并发请求可能同时读取 PENDING，然后重复执行相同 Action。

### 4.2 审批状态调整

建议审批状态至少包含：

```text
PENDING
EXECUTING
APPROVED
REJECTED
EXPIRED
```

状态说明：

- PENDING：等待审批。
- EXECUTING：审批已被占用，执行流程正在进行。
- APPROVED：操作已经执行成功，审批流程正常结束。
- REJECTED：人工拒绝。
- EXPIRED：审批过期。

如果现有字段已经能够表达等价状态，可以复用，不必重复增加字段。

另外，应明确记录审批人和审批时间。

### 4.3 原子占用

禁止采用先 SELECT 检查 PENDING，再无条件 UPDATE 的方式。

必须使用条件更新：

```sql
UPDATE agent_approval
SET status = 'EXECUTING'
WHERE approval_id = ?
  AND status = 'PENDING';
```

具体表名和字段名以当前数据库结构为准。

然后检查受影响行数。

- 影响 1 行：当前请求获得执行权。
- 影响 0 行：审批已经被其他请求处理，拒绝执行。

状态占用必须在数据库事务中完成。

### 4.4 审批前重新检查

审批可能发生在 Plan 生成之后较长时间。

因此审批执行时必须重新检查：

- upstream 是否存在。
- upstream 是否已经 drained。
- 当前健康 upstream 数量。
- 目标状态是否发生变化。
- Plan 前置条件是否仍然满足。

不能依赖生成 Plan 时保存的旧上下文。

### 4.5 Plan Hash 完整性

目前 Plan Hash 覆盖范围不够完整。

需要覆盖所有影响执行安全性的字段：

- Action。
- Parameters。
- Preconditions。
- Verification。
- Failure Strategy。
- 其他影响执行行为的关键字段。

建议使用稳定、确定性的 JSON 序列化或规范化结构进行 Hash 计算。

必须保证：

相同 Plan 内容生成相同 Hash。

任何影响执行行为的字段发生变化，Hash 都应变化。

不要直接对不稳定的 map 序列化结果计算 Hash。

### 4.6 测试要求

- 两个并发请求审批同一个 Plan。
- 只有一个请求可以成功占用。
- 失败的请求不能触发 Action。
- 已审批任务不能重复审批。
- 已拒绝任务不能执行。
- Plan Hash 被修改后不能执行。
- 审批等待期间 upstream 状态发生变化时拒绝或重新评估。

---

## 任务 5：Operation 记录前置与异常恢复

**优先级：P0**

涉及：

```text
cmd/agent_approve.go
agent/store.go
相关测试文件
```

### 5.1 当前问题

目前实际操作发生后才创建 Operation 记录。

这会导致：

- 操作发生时没有事先审计记录。
- 执行成功但数据库更新失败时状态不一致。
- 进程中断后难以确认操作是否执行。

### 5.2 Operation 状态

建议使用：

```text
PREPARED
RUNNING
SUCCEEDED
FAILED
UNKNOWN
```

状态含义：

- PREPARED：操作记录已经建立，尚未开始执行。
- RUNNING：执行过程已经开始。
- SUCCEEDED：实际操作执行成功。
- FAILED：实际操作执行失败。
- UNKNOWN：操作结果无法确认。

### 5.3 Operation 字段

优先复用现有 Operation 表。

建议至少记录：

- operation_id。
- task_id。
- plan_id。
- action。
- parameters 或安全的参数摘要。
- status。
- started_at。
- finished_at。
- result。
- error。

如果现有表已具备相应字段，不要重复增加。

### 5.4 调整执行顺序

```text
读取审批
    |
校验 Plan Hash
    |
重新检查执行前置条件
    |
原子占用审批
    |
创建 Operation(PREPARED)
    |
更新 Operation(RUNNING)
    |
执行实际 Action
    |
记录执行结果
    |
进入验证流程
```

任何关键数据库操作失败，都不能静默忽略。

### 5.5 异常恢复

必须考虑：

A. Operation 创建后、实际操作前发生崩溃。

B. 实际操作成功、结果尚未写入数据库时发生崩溃。

C. 实际操作失败。

D. 操作执行结果不确定。

对于 UNKNOWN：

- 不要盲目重试。
- 重新读取 Server 真实状态。
- 根据实际状态决定是否可以完成。
- 无法确认时转为人工处理。

特别注意：

SQLite 事务不能和 Server 实际操作形成跨系统原子事务。

本次不要求实现分布式事务，只要求通过前置持久化、条件更新、幂等 Action 和异常恢复降低不一致风险。

### 5.6 幂等要求

对现有 drain / undrain：

- 重复 drain 不应产生额外副作用。
- 重复 undrain 不应产生额外副作用。
- 执行前检查目标当前状态。
- 已经达到预期状态时，记录为无需再次执行。
- 不允许因为进程重启而自动重复执行 UNKNOWN 操作。

---

# 四、第二批：P1 执行闭环改造

第一批全部完成并通过测试后，再开始本批次。

## 任务 6：完善 Task 状态管理

涉及：

```text
agent/store.go
agent/decide.go
agent/plan.go
cmd/agent_approve.go
```

建议明确以下 Task 状态：

```text
RECEIVED
COLLECTING
DIAGNOSING
PLANNED
WAITING_APPROVAL
WAITING_HUMAN
EXECUTING
VERIFYING
COMPLETED
FAILED
CLOSED
```

### 状态说明

| 状态 | 说明 |
|---|---|
| RECEIVED | 收到事件 |
| COLLECTING | 采集上下文 |
| DIAGNOSING | 正在诊断 |
| PLANNED | 已经生成处理计划 |
| WAITING_APPROVAL | 等待人工审批 |
| WAITING_HUMAN | 等待人工处理 |
| EXECUTING | 正在执行操作 |
| VERIFYING | 正在验证操作结果 |
| COMPLETED | 故障已经确认解决 |
| FAILED | 执行或验证失败 |
| CLOSED | 任务被关闭 |

### 实施要求

不要引入复杂的状态机框架。

只需要建立明确的状态转换方法。

例如：

```go
TransitionTaskStatus(
    taskID,
    expectedStatus,
    newStatus,
)
```

具体方法签名按现有代码规范调整。

状态更新必须支持条件更新，避免多个处理流程互相覆盖。

禁止随意直接更新 Task 状态而绕过状态转换规则。

注意：

- COMPLETED 必须建立在验证通过的基础上。
- CLOSED 不代表故障一定已经自动修复。
- WAITING_HUMAN 不应该进入普通审批执行流程。

---

## 任务 7：增加 drain / undrain 执行后验证

本次只实现现有两个 Action 的最小验证闭环，不建设通用验证框架。

### drain 验证

执行成功后检查：

1. 目标 upstream 是否确实处于 drained 状态。
2. Server 当前运行状态是否反映该变化。
3. 是否仍然存在其他可用 upstream。

### undrain 验证

执行成功后检查：

1. 目标 upstream 是否已经恢复为非 drained 状态。
2. Server 当前配置或运行状态是否反映恢复结果。

### 状态处理

```text
Action 执行成功
    |
    v
VERIFYING
    |
    +-- 验证成功
    |       |
    |       v
    |   COMPLETED
    |
    +-- 验证失败
    |       |
    |       v
    |   FAILED 或 WAITING_HUMAN
    |
    +-- 无法确认
            |
            v
        WAITING_HUMAN
```

必须区分：

- 配置状态验证成功。
- 实际运行健康验证成功。

不能把配置恢复等同于 upstream 已经恢复健康。

---

## 任务 8：改进 recovered 事件关联

涉及：

```text
agent/decide.go
agent/store.go
agent/event.go
相关测试文件
```

当前通过相同 started_at 关联故障任务存在误关闭风险。

建议优先根据：

- 故障类型。
- 目标对象。
- 故障事件标识。
- 故障发生区间。

建立对应关系。

如果 Monitor 事件当前没有足够信息建立精确关联，应先增强事件字段，而不是使用过于宽泛的匹配条件。

无法确定对应关系时：

- 记录 recovered 事件。
- 不要自动关闭多个可能无关的 Task。

测试必须覆盖：

同一 Server 启动周期内发生多个独立故障，之后分别恢复的情况。

---

# 五、第三批：P1 Agent 运行可靠性改造

## 任务 9：Agent 心跳

涉及：

```text
agent/main.go
agent/store.go
management/agentview/agentview.go
```

### 心跳字段

建议最小化保存：

- agent_id。
- pid。
- started_at。
- heartbeat_at。
- last_event_at。
- last_success_at。
- last_error。

建议心跳周期为 10～30 秒。

### Management 状态判断

不能再通过数据库文件是否存在判断 Agent 在线。

建议：

- 心跳在有效时间窗口内：ONLINE。
- 心跳超时：OFFLINE。
- 数据不存在或无法判断：UNKNOWN。

不需要建设独立的服务注册中心。

---

## 任务 10：JSONL 日志轮转识别

涉及：

```text
agent/tail.go
```

当前仅通过：

```go
if info.Size() < offset {
    offset = 0
}
```

识别文件截断。

需要进一步考虑：

- 文件追加。
- 文件截断。
- 文件被重命名。
- 新文件重新创建。
- 新文件大小已经超过旧 offset。

优先使用跨平台可行的文件身份信息。

如果不同平台无法统一获取 inode，应采用合理的降级方式。

测试必须覆盖：

新日志文件大小大于旧 offset 的情况。

---

## 任务 11：Agent 优雅退出

涉及：

```text
agent/main.go
agent/runtime.go
agent/collect.go
```

要求：

1. 主循环支持 context cancellation。
2. Ticker 正确 Stop。
3. 子进程支持取消。
4. HTTP 请求有合理超时。
5. SQLite 正常关闭。
6. 已经读取但尚未完成的事件不能无故丢失。
7. 执行中的 Operation 必须能够在重启后识别。

不要为了优雅退出引入复杂的后台任务管理框架。

---

## 任务 12：SQLite Schema Version

涉及：

```text
agent/store.go
```

当前数据库初始化中存在每次打开数据库都尝试 ALTER TABLE 并忽略错误的做法。

建议增加简单的 Schema Version 机制。

要求：

- 记录当前 Schema Version。
- 按版本顺序执行迁移。
- 每个迁移必须在事务中执行。
- 迁移成功后才更新版本号。
- 迁移失败必须返回错误。
- 已完成的迁移不能重复执行。
- 保留已有 SQLite 数据。
- 能够处理历史版本数据库。

如果项目已有迁移工具，优先复用。

如果没有，采用简单的版本表和 Go 迁移函数即可。

不要引入大型 ORM 或迁移框架。

---

# 六、数据库设计约束

本次改造涉及 Task、Approval、Operation 和 Agent 心跳。

但必须遵守：

**先检查现有表结构，优先扩展，不要重新设计整套数据库。**

重点检查：

- agent_event
- agent_task
- agent_plan
- agent_approval
- agent_operation
- 其他现有 Agent 表

根据实际表结构决定是否需要增加字段或表。

所有迁移必须：

1. 有明确版本号。
2. 支持事务。
3. 支持重复启动。
4. 不丢失历史记录。
5. 有迁移测试。
6. 有失败回滚测试。

不要为了实现状态管理而引入多张功能重复的表。

---

# 七、测试和验收要求

本次改造不能只通过代码阅读验收，必须实际运行测试。

## 7.1 单元测试

至少覆盖：

- upstream Healthy 判断。
- drain 计划生成。
- Policy 三态判断。
- Task 状态转换。
- Plan Hash。
- Approval 状态转换。
- Operation 状态转换。
- drain / undrain 幂等。
- Event + Task 事务一致性。
- 日志轮转。
- recovered 事件关联。
- Agent 心跳状态判断。

## 7.2 并发测试

至少覆盖：

1. 两个并发请求审批同一个 Plan。
2. 两个并发请求 drain 同一个 upstream。
3. 两个并发请求 undrain 同一个 upstream。
4. 同一个 Event 被并发接收。
5. 多个处理流程并发更新同一个 Task。

核心验收要求：

同一个审批只能被一个请求成功占用，并且只能触发一次实际操作。

## 7.3 故障注入测试

模拟：

1. Event 插入成功后 Task 插入失败。
2. Operation 创建后进程中断。
3. Action 执行成功后数据库更新失败。
4. Action 执行失败。
5. Action 执行结果不确定。
6. Plan Hash 不匹配。
7. 审批等待期间 upstream 状态发生变化。
8. drain 执行成功但验证失败。
9. undrain 执行成功但验证失败。
10. 日志轮转时新文件大小大于旧 offset。

## 7.4 工程测试

每批完成后运行相关测试。

最终运行：

```bash
go test ./...
go test -race ./...
go vet ./...
```

如果项目存在平台差异或某项测试无法运行，必须明确记录。

不得声称没有实际执行的测试已经通过。

---

# 八、实施批次与交付顺序

严格按下面的顺序实施。

## Batch 0：代码核查

交付：

- 当前代码核查报告。
- 需要修改的文件清单。
- 数据库迁移清单。
- 已经完成和未完成项目清单。

此阶段不修改代码。

## Batch 1：P0 核心可靠性

实施：

1. upstream Healthy 判断修复。
2. Event + Task 事务一致性。
3. WAITING_HUMAN / WAITING_APPROVAL 区分。
4. 审批原子占用。
5. Plan Hash 完整性。
6. Operation 前置持久化。
7. 审批执行幂等保护。

交付：

- 修改代码。
- 数据库迁移。
- 单元测试。
- 并发测试。
- Batch 1 测试报告。

完成后暂停，检查代码差异和测试结果。

## Batch 2：执行闭环

实施：

1. Task 状态管理。
2. EXECUTING 和 VERIFYING。
3. drain / undrain 执行后验证。
4. recovered 事件精确关联。

交付：

- 修改代码。
- 状态转换测试。
- 执行验证测试。
- 故障关联测试。

完成后暂停，检查代码差异和测试结果。

## Batch 3：运行可靠性

实施：

1. Agent 心跳。
2. Management 在线状态判断。
3. 日志轮转识别。
4. 优雅退出。
5. SQLite Schema Version。

交付：

- 修改代码。
- 数据库迁移。
- 对应测试。
- 最终测试报告。

## Batch 4：文档同步

更新：

```text
docs/feature/22-ai-agent.md

docs/develop/plans/ai-agent-execution-plan.md

docs/develop/plans/ai-agent-server-gap-plan.md
```

要求：

- 文档与实际代码保持一致。
- 不再把已经实现的 agent approve 描述为尚未实现。
- 明确当前支持的 Action。
- 明确哪些操作需要人工审批。
- 明确哪些诊断任务需要人工处理。
- 明确执行验证的能力边界。
- 明确本次仍然没有实现的功能。

不要把计划中的功能描述成已经完成。

---

# 九、明确不在本次改造范围内的事项

本次不要实施以下内容：

1. 不增加新的故障诊断类型。
2. 不增加新的自动修复 Action。
3. 不开放证书自动续期审批执行。
4. 不让 LLM 自由规划和执行操作。
5. 不增加通用 Shell 执行器。
6. 不建设多 Agent 协作体系。
7. 不引入 MQ。
8. 不增加 Redis、Kafka。
9. 不引入复杂工作流引擎。
10. 不重构 Management API。
11. 不重构整个 Agent 目录。
12. 不建设复杂的权限管理系统。
13. 不做与本次可靠性目标无关的性能优化。

特别说明：

当前 Server 已经增加了本地证书续期接口，但这不意味着 Agent 应当立即获得证书自动续期的执行权限。

在审批、Operation、异常恢复和验证闭环可靠之前，不要增加新的高风险写操作。

---

# 十、代码修改约束

Cursor 在执行过程中必须遵守：

1. 修改前先阅读相关完整代码，不要只看局部函数。
2. 优先复用现有接口和数据结构。
3. 不要随意改变 CLI 参数和输出格式。
4. 不要随意改变 Runtime API 和 JSON 数据结构。
5. 数据库字段变化必须考虑历史数据兼容。
6. 不要忽略数据库错误。
7. 不要通过大量兼容代码掩盖状态设计问题。
8. 不要把所有逻辑堆到 cmd/agent_approve.go。
9. 不要为了抽象而新增大量 package。
10. 新增 Action 必须有明确的 Policy 和验证规则。
11. 所有实际操作必须经过安全检查。
12. 如果发现当前代码与方案存在冲突，先分析并采用最小调整方式。
13. 不要覆盖用户未提交的修改。
14. 每个 Batch 完成后输出 Git diff 和测试结果。
15. 未经确认，不要擅自提交 Git commit 或推送远程分支。

---

# 十一、最终交付报告

全部批次完成后，输出完整报告。

至少包括：

## 11.1 修改文件清单

逐个说明：

- 文件路径。
- 修改原因。
- 修改内容。
- 对其他模块的影响。

## 11.2 数据库变化

说明：

- 新增表。
- 新增字段。
- 修改的状态。
- Schema Version。
- 历史数据迁移方式。
- 迁移失败时的处理方式。

## 11.3 核心流程变化

说明：

- Event 接收与 Task 创建。
- Task 状态转换。
- Diagnosis 和 Planner。
- Policy 决策。
- 人工审批。
- Operation 执行。
- 执行后验证。
- 异常恢复。

## 11.4 测试结果

列出：

- 新增测试数量。
- 测试命令。
- 通过数量。
- 失败数量。
- 无法执行的测试及原因。
- 并发测试结果。
- 竞态测试结果。

## 11.5 未完成事项

明确列出：

- 本次没有完成的功能。
- 仍然存在的技术风险。
- 需要后续处理的事项。

## 11.6 最终验收

只有在所有必要测试通过、关键状态和操作闭环正确的情况下，才可以宣布改造完成。

---

# 十二、最终验收标准

本次改造最终应达到：

1. upstream 健康状态具有可靠的数据来源。
2. 不会因为历史累计错误永久判断 upstream 不健康。
3. 健康状态不确定时不会生成危险操作计划。
4. Event 和 Task 创建具有事务一致性。
5. 没有有效 Plan 的任务不会进入审批队列。
6. 并发审批不会重复执行同一 Action。
7. Plan Hash 覆盖所有影响操作安全性的关键字段。
8. 审批执行前会重新检查运行条件。
9. 实际操作之前已经持久化 Operation 记录。
10. 执行结果不确定时不会盲目重复操作。
11. drain / undrain 具有幂等保护。
12. Task 只有经过执行后验证才能标记 COMPLETED。
13. recovered 事件不会错误关闭其他独立故障任务。
14. Agent 在线状态基于心跳而不是数据库文件存在。
15. 日志轮转能够正确识别文件替换。
16. Agent 能够优雅退出。
17. SQLite 数据库迁移具有明确版本管理。
18. 相关测试和全量测试通过。

**最终目标：在不推翻现有 Fast-Https AI Agent 架构的前提下，将现有的基础诊断和有限审批执行能力，提升为具备可靠事件处理、受控操作、持久化审计和执行结果验证能力的运维 Agent。**