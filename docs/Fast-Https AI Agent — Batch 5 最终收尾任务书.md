# Fast-Https AI Agent — Batch 5 最终收尾任务书

**任务性质：** 可靠性与安全边界收尾  
**目标分支：** `fast-https-ai-agent`  
**实施方式：** 增量修复，不进行架构重构  
**执行原则：** 先检查、再修改；每项修复配套测试；不擅自提交或推送代码。

---

## 一、任务背景与目标

Fast-Https AI Agent 前期 Batch 0～4 的可靠性改造已经基本完成，主要包括：

- Event 与 Task 的事务一致性。
- 审批原子占用。
- Plan Hash 完整性校验。
- Operation 前置持久化。
- Task 状态机。
- 操作执行后的验证闭环。
- recovered 事件关联优化。
- Agent 心跳和 Management 在线状态。
- 日志轮转识别。
- SQLite Schema 版本迁移。
- 优雅退出。
- Agent 独立进程及发行包集成。

Batch 5 不再开展大规模改造，只针对最新代码检查中发现的几个剩余问题进行收尾。

### 本批次目标

1. 保证 Approval 与 Task 的状态一致性。
2. 保证 Task 状态转换结果得到正确检查。
3. 对旧版 Runtime 健康状态采用保守兼容策略。
4. 明确网络健康与 HTTP 业务健康的语义边界。
5. 修复 JSON 顶层尾随数据校验。
6. 更新 Agent 发行包及部署文档。
7. 补齐相关异常场景测试。

最终要求：Agent 在状态不确定、执行结果不确定或数据不兼容时，优先进入人工处理，而不是擅自执行可能影响正常服务的操作。

---

## 二、实施前置要求

### 2.1 核对最新代码

开始修改前：

1. 确认当前分支为 `fast-https-ai-agent`。
2. 获取最新代码，确认工作区没有未提交的用户修改。
3. 读取以下关键文件的当前版本：
   - `cmd/agent_approve.go`
   - `agent/store.go`
   - `agent/plan.go`
   - `agent/runtime.go`
   - `agent/cli.go`
   - `modules/proxy/metrics.go`
   - `modules/proxy/event_proxy.go`
   - `docs/feature/22-ai-agent.md`
   - `docs/develop/plans/ai-agent-execution-plan.md`
4. 检查现有测试，确认是否已经有人修复本任务涉及的问题。
5. 对已经修复的项目，不要重复修改。

注意：本任务书基于上一轮代码静态检查结果。Cursor 必须以实际最新代码为准，不得机械套用旧代码片段。

### 2.2 实施约束

- 不推翻现有 Agent 架构。
- 不引入 Redis、MQ、Kafka 等新依赖。
- 不引入新的通用任务调度框架。
- 不扩大 Agent 的 CLI 执行白名单。
- 不允许 LLM 直接执行系统操作。
- 不改变现有 Policy 的安全边界。
- 不允许在健康状态不明确时自动 drain upstream。
- 不擅自修改与 Batch 5 无关的业务逻辑。
- 不擅自 commit、push 或合并分支。

---

## 三、具体实施任务

### Task 5.1：修复 Approval 与 Task 状态一致性

**优先级：P0**

涉及文件：

- `cmd/agent_approve.go`
- `agent/store.go`
- 相关测试文件

#### 现状问题

当前审批执行流程中，可能出现 Approval 与 Task 状态不一致。

例如：

```text
Task:     EXECUTING
Approval: PENDING
```

或者：

```text
Task:     FAILED
Approval: PENDING
```

典型场景：

1. 审批记录被原子占用，状态变成 `EXECUTING`。
2. Task 状态被修改为 `EXECUTING`。
3. 后续 Operation 创建失败，或者操作执行失败。
4. 代码将 Approval 恢复为 `PENDING`，但 Task 没有同步恢复。

这会导致后续审批流程无法正常继续，也会使数据库状态难以解释。

#### 修复要求

重新梳理整个审批生命周期：

```text
PENDING
   |
   v
EXECUTING
   |
   +----> APPROVED
   |
   +----> FAILED
   |
   +----> UNKNOWN / 人工处理
```

具体要求：

1. Approval 从 `PENDING` 进入 `EXECUTING` 后，不允许在没有明确判断操作是否执行的情况下，直接恢复为 `PENDING`。
2. Operation 创建失败且可以确定实际操作尚未发生时，允许将审批流程恢复到可重试状态，但必须同步处理 Task 状态。
3. 操作执行明确失败时，Approval 与 Task 必须进入语义一致的失败状态。
4. 操作执行结果不确定时，不允许自动恢复为 `PENDING`。
5. 操作执行成功但验证失败时，应进入人工处理或明确的失败状态。
6. 重复审批请求不能绕过已有状态。
7. 保留现有 Plan Hash、Approval 校验和 Policy 校验逻辑。

#### 状态处理原则

| 场景 | Approval | Task |
|---|---|---|
| 尚未审批 | PENDING | WAITING_APPROVAL |
| 正在执行 | EXECUTING | EXECUTING |
| 执行并验证成功 | APPROVED | COMPLETED |
| 明确执行失败 | FAILED | FAILED |
| 操作结果不确定 | 保留执行中或明确的 UNKNOWN 状态 | WAITING_HUMAN |
| 操作成功但验证无法确认 | 不得标记成功 | WAITING_HUMAN |

说明：

上述表格是期望的语义，不要求为了追求完全一致而强行增加大量新状态。如果当前 Approval 表没有 UNKNOWN 状态，应评估是否可以通过已有字段表达不确定状态；确实需要新增字段或状态时，保持最小改动，并提供 SQLite Schema 迁移。

#### 特别要求

考虑 SQLite 和真实 Server 操作无法构成一个跨系统原子事务。

不能简单地把所有异常都当作执行失败，更不能在不知道 Server 操作是否已经发生时自动重新执行。

---

### Task 5.2：修复 verifyAndFinish 状态转换检查

**优先级：P0**

涉及文件：

- `cmd/agent_approve.go`
- 相关状态机测试文件

#### 现状问题

`transitionTask()` 返回：

```go
(bool, error)
```

但部分调用方只检查 `error`，没有检查 `bool`。

例如：

```go
if _, err := transitionTask(
    db, taskID, "EXECUTING", "VERIFYING",
); err != nil {
    return err
}
```

即使 SQL 没有报错，但实际上没有更新任何记录，后续逻辑仍可能继续执行。

#### 修复要求

1. 所有调用 `transitionTask()` 的地方，必须同时检查：
   - error 是否为空。
   - 是否确实发生状态转换。
2. 对不允许的状态转换，必须返回明确错误。
3. 只有当前状态符合预期时，才允许进入下一阶段。
4. 不允许在 Task 状态转换失败时，仍将 Approval 标记为成功。
5. 对需要恢复执行的场景，明确区分：
   - 正常状态转换。
   - 已经处于目标状态。
   - 状态不匹配。
6. 如果允许幂等重入，必须显式处理，不能把 `RowsAffected == 0` 一律视为成功。

#### 验收场景

- EXECUTING → VERIFYING 正常成功。
- VERIFYING → COMPLETED 正常成功。
- Task 已经处于 WAITING_HUMAN 时，不能直接标记 COMPLETED。
- Task 不存在时，必须报错。
- Task 状态被并发修改时，不能继续执行错误的后续流程。

---

### Task 5.3：旧版 Runtime 健康状态兼容

**优先级：P0**

涉及文件：

- `agent/runtime.go`
- `agent/plan.go`
- `cmd/agent_approve.go`
- `modules/proxy/metrics.go`
- 相关测试文件

#### 现状问题

新版本已经使用：

```text
healthy
unhealthy
unknown
```

区分 upstream 健康状态。

但是旧版本 Runtime 可能只有：

```json
{
  "healthy": false
}
```

旧字段可能代表累计发生过失败，而不是当前确定不健康。

如果新 Agent 直接把 `healthy=false` 解释成 `unhealthy`，可能错误地把正常 upstream 作为 drain 候选。

#### 修复要求

统一健康状态解释逻辑：

| 输入情况 | 解释结果 |
|---|---|
| 新版 health=healthy | healthy |
| 新版 health=unhealthy | unhealthy |
| 新版 health=unknown | unknown |
| 旧版只有 healthy=true | healthy |
| 旧版只有 healthy=false | unknown |
| health 字段缺失且没有旧字段 | unknown |
| health 字段值非法 | unknown |

具体要求：

1. 优先使用新版明确的 `health` 字段。
2. 对旧版 `healthy=false` 采用保守兼容，解释为 `unknown`。
3. 不得将未知状态自动升级为 unhealthy。
4. 只有明确 unhealthy 的 upstream 才能进入自动 drain 候选。
5. drain 计划生成前，必须保证至少存在明确 healthy 的备用 upstream。
6. 如果没有明确 healthy 的备用 upstream，不生成自动 drain 计划。
7. 审批执行时重新检查健康状态，不能仅依赖计划生成时的历史快照。
8. 新旧版本兼容不得影响现有配置和运行接口。

#### 兼容性测试

必须覆盖：

- 新版 Runtime 正常状态。
- 新版 Runtime 异常状态。
- 新版 Runtime 未知状态。
- 旧版 Runtime `healthy=true`。
- 旧版 Runtime `healthy=false`。
- 完全缺少健康字段。
- 非法健康字段。
- 多个 upstream 中存在 UNKNOWN。
- 所有 upstream 都是 UNKNOWN。
- 没有明确 healthy 备用节点。

---

### Task 5.4：明确网络健康与 HTTP 业务健康的语义边界

**优先级：P1**

涉及文件：

- `modules/proxy/metrics.go`
- `modules/proxy/event_proxy.go`
- `agent/runtime.go`
- `agent/plan.go`
- 相关测试文件
- 相关功能文档

#### 现状问题

当前：

```go
noteUpstreamOK(p.ProxyAddr)
```

是在收到并解析上游 HTTP 响应后调用的。

因此，即使上游返回 HTTP 500，只要成功收到响应，也可能被计入 OK。

这意味着当前健康指标主要反映连接和响应读取是否成功，而不是 HTTP 业务响应是否正常。

#### 本批次目标

先明确指标语义，不要贸然改变已有健康判断行为。

建议区分：

**1. 网络健康**

用于判断：

- DNS 解析是否成功。
- TCP 连接是否成功。
- 请求是否发送成功。
- 响应是否成功读取。
- 是否发生连接超时、发送超时、读取超时。

**2. HTTP 响应健康**

用于判断：

- HTTP 状态码。
- 5xx 比例。
- 其他异常响应比例。

**3. 业务健康**

用于判断：

- 业务接口是否正常。
- 业务返回是否符合预期。
- 是否存在业务级错误。

#### 修复要求

1. 明确当前 `health` 指标究竟表示什么。
2. 不要将 HTTP 500 直接等同于网络连接失败。
3. 如果需要增加 HTTP 状态码统计，优先使用现有统计结构扩展。
4. 不要为了 Batch 5 引入复杂的健康检查系统。
5. 保持已有 Monitor 事件兼容。
6. 在 Agent 生成 drain 计划时，明确当前依据的是网络健康还是业务健康。
7. 不允许仅凭一个业务错误率事件，就在缺乏充分证据时摘除 upstream。

如果现有指标结构不足以安全地区分上述语义，本批次可以只完成指标定义、文档和测试，不强行引入新的自动处置逻辑。

---

### Task 5.5：修复 JSON 顶层尾随数据校验

**优先级：P1**

涉及文件：

- `agent/cli.go`
- 相关测试文件

#### 现状问题

目前代码使用：

```go
if dec.More() {
    return nil, fmt.Errorf("stdout is not one JSON object")
}
```

`Decoder.More()` 不适合检查顶层 JSON 后是否存在额外内容。

#### 修复要求

使用第二次 Decode 检查是否到达 `io.EOF`。

逻辑要求：

1. 第一次 Decode 必须成功。
2. 第一段内容必须是合法 JSON 对象。
3. 第二次 Decode：
   - 返回 `io.EOF`：合法。
   - 返回 nil：存在第二个 JSON 值，拒绝。
   - 返回其他错误：存在非法尾随内容，拒绝。
4. 保持现有 CLI 白名单逻辑不变。
5. 不接受 JSON 对象后附加任意文本。

#### 测试用例

合法：

```json
{"ok":true}
```

非法：

```json
{"ok":true}{"unexpected":true}
```

```text
{"ok":true}
unexpected
```

```text
{"ok":true} trailing
```

同时测试空输出、非法 JSON、JSON 数组、JSON 字符串和 JSON null。

---

### Task 5.6：同步 Agent 发行版和部署文档

**优先级：P1**

涉及文件：

- `docs/feature/22-ai-agent.md`
- `docs/develop/plans/ai-agent-execution-plan.md`
- `docs/develop/plans/ai-agent-server-gap-plan.md`
- `docs/help/package-layout.md`
- 必要时更新 `docs/usage.md`、`docs/usage.html`

#### 现状问题

最新代码已经增加：

- Agent Linux/Windows 构建。
- Linux deb/rpm/apk 打包。
- 独立 systemd 单元。
- Agent 配置文件。
- 安装、卸载处理。

但部分旧文档仍然描述 Agent 尚未打包、没有 systemd 单元。

#### 更新要求

1. 明确 Agent 已支持独立进程部署。
2. 明确 Agent 与 Server 的进程关系。
3. 说明 Linux systemd 单元名称：
   `fast-https-agent.service`
4. 说明配置文件：
   `/etc/fast-https/agent.json`
5. 说明状态目录：
   `/var/lib/fast-https/agent`
6. 说明 Monitor 事件目录：
   `/var/log/fast-https/monitor`
7. 提供启动、停止、查看状态命令。
8. 说明 Agent 与 Server 不存在强制 systemd 依赖。
9. 说明 Agent 需要读取 Monitor 事件，并调用 Server CLI 获取上下文。
10. 说明卸载和 purge 的数据保留差异。

Linux 操作示例：

```bash
# 查看状态
sudo systemctl status fast-https-agent

# 启动并设置开机启动
sudo systemctl enable --now fast-https-agent

# 停止
sudo systemctl stop fast-https-agent

# 查看日志
sudo journalctl -u fast-https-agent -f
```

特别说明：

当前安装脚本并不自动启用 Agent 服务，因此文档必须明确提示运维人员手动启用。

不要修改发行包安装行为，除非发现实际打包或启动错误。

---

## 四、测试要求

本批次必须为每项修复增加针对性测试。

### 4.1 审批状态一致性测试

至少覆盖：

- Operation 创建失败。
- Operation 创建成功但实际操作失败。
- 实际操作结果不确定。
- 操作成功但验证失败。
- 重复审批。
- 并发审批。
- Task 与 Approval 状态一致性。

要求：任何异常路径都不能留下无法解释的状态组合，也不能在执行结果不确定时自动重复执行。

### 4.2 Task 状态转换测试

至少覆盖：

- 正常状态转换。
- 状态不匹配。
- Task 不存在。
- RowsAffected 为 0。
- 并发修改状态。
- 非法状态转换。

### 4.3 Runtime 兼容测试

覆盖 Task 5.3 所列全部输入情况。

重点验证：

**UNKNOWN 不得被解释为 UNHEALTHY，更不能单独触发 drain。**

### 4.4 JSON 测试

覆盖 Task 5.5 所列合法和非法 JSON 输出。

### 4.5 回归测试

必须执行：

```bash
go test ./...
go test -race ./...
go vet ./...
```

如果某项测试因环境或平台原因无法执行，必须说明原因，不得将未执行描述为通过。

---

## 五、验收标准

Batch 5 完成必须同时满足以下条件：

| 编号 | 验收项目 | 标准 |
|---|---|---|
| B5-01 | Approval 与 Task | 异常路径状态一致且可解释 |
| B5-02 | 审批幂等 | 不会因重复请求重复执行操作 |
| B5-03 | 状态转换 | 所有关键状态转换检查 bool 和 error |
| B5-04 | 旧版 Runtime | 旧版 false 保守解释为 unknown |
| B5-05 | Drain 安全 | 不允许基于 unknown 生成自动 drain |
| B5-06 | 健康语义 | 网络健康与 HTTP 业务健康定义清晰 |
| B5-07 | JSON 校验 | 拒绝第二段 JSON 和非法尾随内容 |
| B5-08 | 文档 | 发行包和 Agent 部署说明与代码一致 |
| B5-09 | 测试 | 新增测试和完整回归测试通过 |
| B5-10 | 架构约束 | 无无关架构重构和新增基础设施依赖 |

---

## 六、Cursor 执行流程

请严格按以下顺序实施。

### 阶段 A：代码核查

先只读代码，不修改。

输出：

1. 当前分支和 HEAD。
2. 工作区状态。
3. 六项任务当前是否已经部分实现。
4. 需要修改的具体文件。
5. 是否存在与本任务书不一致的现有实现。
6. 建议的最小修复方案。

完成后暂停，等待确认。

### 阶段 B：修复 P0 问题

实施：

- Task 5.1：Approval 与 Task 状态一致性。
- Task 5.2：状态转换检查。
- Task 5.3：旧版 Runtime 健康状态兼容。

完成后：

1. 输出修改文件列表。
2. 展示关键代码差异。
3. 说明异常状态如何处理。
4. 执行相关测试。
5. 暂停，等待审查。

### 阶段 C：修复 P1 问题

实施：

- Task 5.4：健康指标语义。
- Task 5.5：JSON 尾随数据校验。
- Task 5.6：部署文档同步。

完成后执行相关测试。

### 阶段 D：完整回归测试

执行：

```bash
go test ./...
go test -race ./...
go vet ./...
```

检查：

- 是否存在新增依赖。
- 是否改变原有 CLI 白名单。
- 是否改变原有 Agent 操作权限。
- 是否影响旧版本配置兼容。
- 是否影响现有发行包构建。
- 是否存在未处理的错误。
- 是否有无关文件被修改。

### 阶段 E：最终报告

输出一份 Batch 5 实施报告：

```text
1. 实际完成的任务
2. 未实施的任务及原因
3. 修改文件清单
4. 关键代码修改说明
5. 新增测试用例
6. go test ./... 结果
7. go test -race ./... 结果
8. go vet ./... 结果
9. 已知遗留问题
10. 最终验收结论
```

未经用户明确授权，不得 commit、push 或合并分支。

---

## 七、最终原则

本批次不是继续扩展 Agent 能力，而是确保现有能力足够可靠。

必须始终遵守：

1. **不确定时不自动执行。**
2. **执行结果不确定时不自动重试。**
3. **状态转换失败时不继续推进流程。**
4. **健康状态 UNKNOWN 不等于 UNHEALTHY。**
5. **LLM 只负责辅助分析和解释，不获得直接操作权限。**
6. **不为了局部问题引入新的复杂架构。**

Batch 5 完成后，Fast-Https AI Agent 的第一阶段可靠性改造即可进入验收和实际部署验证阶段。

后续工作重点应转向真实运行环境中的稳定性验证、故障案例积累和诊断效果评估，而不是继续进行没有明确收益的架构调整。
