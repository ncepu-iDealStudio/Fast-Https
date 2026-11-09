# fast-https AI Agent 源码扫描报告

日期：2026-11-09。分支：`fast-https-ai-agent`。

本文只读核对当前仓库，回答规范第 17.1–17.3 节：现有能力、差距、需要新增的接口。设计目标以 [fast-https AI agent功能设计与开发规范.md](fast-https%20AI%20agent功能设计与开发规范.md) 为准。可执行任务分两份：Agent 进程见 [ai-agent-execution-plan.md](develop/plans/ai-agent-execution-plan.md)；Server、Monitor 与管理核心要补的能力见 [ai-agent-server-gap-plan.md](develop/plans/ai-agent-server-gap-plan.md)。

扫描方法：沿调用链读源码与已落地的功能文档，不改业务代码，不启动服务。静态阅读确认不了的运行时行为标为「待运行验证」。

已撤回的 [agent-refactor-plan.md](develop/plans/agent-refactor-plan.md) 不是本报告的设计依据。该文保留的 `modules/safe/control.go` 是进程内黑名单与限流入口，下面单独说明。

## 1. 结论

fast-https 已经有独立的 Server、只读 `/runtime`、独立 Monitor、以及一套走 CLI 的管理核心（配置事务、operation、日志查询、证书读取）。这些可以直接作为 Agent 的观测面和 L1 查询面。

规范第一阶段想自动完成的三件事，源码对不上：

| 规范场景 | 源码事实 | 结论 |
| --- | --- | --- |
| 12.1 摘除故障 Upstream | `proxy_pass` 是单个地址。没有节点列表、健康检查、摘除或恢复命令。上游失败只进入进程级累计计数 | 自动摘除不能做。前置是尚未实施的 [upstream-failover-plan.md](develop/plans/upstream-failover-plan.md) |
| 12.2 502/504 升高 | `/runtime` 有 `responses_5xx_total`，以及单独的 `responses_502_total`、`responses_504_total`。Monitor 事件仍是 `5xx_ratio`，不按 502/504 再告警 | 可以在诊断里引用这两个累计值。还没有对应的 Monitor 事件 |
| 12.3 证书即将过期或续期失败 | `cert list` / `cert check` 能读 PEM。Monitor 在配置了 `cert_files` 时写 `certificate_expiring`。`cert renew` 不存在。ACME 续期在 Server 进程内 | 事件流里能看到即将过期的证书。不能自动续期 |

仓库里没有 Management HTTP API，没有审批存储，没有 Agent 进程，没有数据库。管理写操作的现成出口是 CLI 加上 `management/txn`。

因此第一阶段能落地的闭环是：读 Monitor 已有事件文件，采集现有 CLI 的只读 JSON，做规则诊断，生成计划，策略层拒绝一切写操作或标成待审批，把审计落盘，并提供只读查询。自动修复留到出现真实的低风险写接口之后。

## 2. 现有能力清单

标记：已具备 / 部分具备 / 不具备 / 待确认 / 不适用。

### 2.1 Server 生命周期与运行状态

**已具备。**

| 证据 | 说明 |
| --- | --- |
| `runtime/metrics/snapshot.go` 的 `ServerInfo` | `state`、`pid`、`version`、`started_at`、`config_generation`、`last_reload_ok`、`last_reload_at`、`reload_failed_total`、`stop_timeout_total` |
| `cmd/runtime_endpoint.go` 的 `StartRuntimeEndpoint` | `GET http://127.0.0.1:9088/runtime`。非 GET 返回 404。绑定失败只记日志，不阻止业务端口 |
| `cmd/service_status.go` | `service status --json`：`running` / `stopped` / `degraded` / `unknown`，外加端口探测 |
| `management/svcctl` | `service reload` 在快照可读时做强验证（生成号加一，pid 与 `started_at` 不变） |

`/runtime` 端口写死 `127.0.0.1:9088`，没有配置项（`docs/feature/20-runtime.md`）。

### 2.2 Runtime 指标

**部分具备。** 有累计计数，没有规范示例里的分项错误码、分上游健康、系统资源。

`TakeSnapshot`（`runtime/metrics/snapshot.go`）固定字段：

- 连接：`active`、`accepted_total`、`closed_total`、`rejected_total`
- 请求：`active`、`total`、`parse_errors_total`、`aborted_total`
- 响应：`responses_2xx_total` 到 `responses_5xx_total`、`responses_502_total`、`responses_504_total`，以及 `responses_unknown_total`
- 耗时：`request_duration` 直方图（`runtime/metrics/histogram.go`）
- 监听：`listeners[].port/ssl/h2`

上游分段由 `modules/proxy/metrics.go` 的 `Snapshot` 经 `RegisterSection` 挂入，字段只有：

- `dial_fail_total`
- `send_timeout_total`
- `read_timeout_total`
- `idle_conns`

这些是进程启动以来的总和，不按上游地址、不按状态码 502/504 拆开。`runtime stats` 的 `--since` / `--domain` 返回 `NOT_SUPPORTED`（`cmd/runtime.go`）。

CPU、内存、磁盘、打开文件数：Server 与 Monitor 都不采集（`docs/feature/21-monitor.md` 已知限制）。

### 2.3 Monitor

**已具备其既定规则，不具备规范第 4.2 节的事件类型。**

独立进程 `monitor/`，不 import 仓库内其他包（`tests/arch/layering_test.go` 的 `monitor` 层只允许自身）。周期 `GET /runtime`，写入：

| 文件 | 写入方 | 读取方 |
| --- | --- | --- |
| `snapshots.jsonl` | `monitor/events.go` `Sink` | 无 CLI |
| `events.jsonl` | 同上 | `management/monview`、`fast-https monitor events` |
| `status.json` | 同上，`schema_version: "1"` | `fast-https monitor status` |

事件类型常量（`monitor/events.go`）：`unreachable`、`recovered`、`restarted`、`collect_error`、`5xx_ratio`、`port_probe`、`stuck_suspected`。

事件 JSON 字段：`time`、`type`、`rule`、`severity`、`message`、落盘时生成的 `event_id`，以及可选 `snapshot`。没有 `resource`、`metrics`、`evidence_refs`、`status`。旧行可以没有 `event_id`。

CLI 查询（`management/monview/monview.go` 的 `Event`）返回 `time`、`type`、`severity`、`message`，以及 `snapshot` 白名单字段。摘要里没有的键（URI、上游地址、证书路径）被丢掉。

投递方式是追加文件，不是 HTTP，也不是 Unix socket。Monitor 不发邮件、不调 Webhook。

### 2.4 日志

**已具备只读、脱敏、限量查询。**

`cmd/logs.go`：`logs error`、`logs access`、`logs query`。实现在 `management/logquery`。没有 `--file`，不读轮转后的旧文件，查询串与凭据会脱敏。输出是数据，不执行日志文本。

### 2.5 配置管理与回滚

**已具备，入口是 CLI，不是 HTTP。**

| 能力 | 证据 |
| --- | --- |
| 只读配置 | `config show`，`management/view` + `management/redact` |
| 校验 | `config validate`，可选 `--candidate` |
| 差异与版本 | `config diff`、`config versions`，快照在 `management/cfgver`、`management/state` |
| 应用与回滚 | `config apply`、`config rollback`，引擎 `management/txn/txn.go` |
| 事务阶段 | `validated`、`restore_point_saved`、`files_switched`、`reloaded`、`verified`、失败时 `compensating` / `restored` |
| 确认 | 非 dry-run 必须 `--yes`。`--request-id` 幂等，冲突为退出码 10 |

`config show` 读磁盘，不读进程内正在生效的快照。`config_generation` 在 `/runtime` 里。

Agent 不能自己改 `fast-https.json`。写配置必须走上述命令。这些命令在规范权限表里属于 L3（改站点、改端口），第一阶段不应自动执行。

### 2.6 Upstream

**不具备节点管理。部分具备进程级失败计数。**

`docs/feature/7-proxy.md`：`proxy_pass` 为单地址，无负载均衡、健康检查、故障转移。未实施计划是 `docs/develop/plans/upstream-failover-plan.md`。

`modules/proxy/pool.go` 的连接池按单个上游地址复用连接，没有摘除接口。

规范 12.1 的「确认至少还有一个健康替代节点」在当前数据模型里没有对象可指。

### 2.7 证书

**部分具备。**

| 能力 | 证据 | 状态 |
| --- | --- | --- |
| 列出与检查 | `cmd/cert.go`，`management/view` 调 `modules/certmanager.Inspect` / `Plan` | 已具备。`days_left` 向零取整，不验证证书链 |
| ACME 配置 | `cmd/acme.go`，写回 `config.WriteACMESection` | 已具备。这是改配置，属 L3 |
| 进程内续期 | `modules/certmanager/manager.go`，certmagic | 已具备，只在 Server 进程内 |
| `cert renew` | `cmd/` 无此子命令 | 不具备。见 CLI 计划 S13，需要回环写通道与令牌 |
| 证书事件 | 配置了 `cert_files` 时写 `certificate_expiring` 与 `certificate_read_error`。不读站点配置，也不写续期失败 | 部分具备 |

### 2.8 CLI

**已具备规范集成表里的只读项和配置事务。没有 Agent 子命令。**

管理类命令（`cmd/commands.go`）在 `--json` 下输出单一 JSON 封套：`ok`、`command`、`request_id`、`data` 或 `error`。退出码见 `docs/cli/command-spec.md`。

与 Agent 直接相关的只读命令：`service status|version`、`runtime status|stats`、`logs error|access|query`、`config show|validate|versions|diff`、`cert list|check`、`diagnose`、`operation list|get`、`monitor status|events`。

写命令：`service reload`、`config apply|rollback`、`site add|update|delete|enable|disable`。都要求 `--yes`（dry-run 除外）并写入 operation。

旧顶层 `start` / `stop` / `reload` / `status` 仍是文本输出，桌面端依赖它们。Agent 不应解析这四条的文本。

`tests/arch` 的 `TestNoExecInProduction` 约束管理命令不执行任意 Shell。

### 2.9 Management API

**不具备。**

对外的机器接口是 CLI 的 `--json`，加上 Server 上唯一的管理向 HTTP：`GET /runtime`（`cmd/runtime_endpoint.go` 的 `serveRuntime`）。没有带令牌的写接口，没有审批 API，没有事件推送 API。

### 2.10 操作持久化

**已具备 CLI 事务的 operation，不具备 Agent 任务或审批。**

`management/operation/operation.go` 的 `Record`：`operation_id`、`request_id`、`command`、`status`（`queued` / `running` / `succeeded` / `failed` / `cancelled` / `unknown` / `recovery_required`）、时间、`phase`、`args_hash`、`result`、`error`。

存储是状态目录里的 JSON 文件（`management/state/dir.go`：`operations/`、`requests.json`），用 `O_EXCL` 锁，不用数据库，不用 flock。同一 `request_id` 加相同参数返回已有记录，参数不同则冲突。

这套状态机服务配置事务，没有 `WAITING_APPROVAL`，也没有诊断、计划、策略结论字段。Agent 不应把自己的任务写进这个目录，避免和配置锁、幂等索引缠在一起。

### 2.11 部署

**Server 与 Monitor 已分进程。Agent 尚未存在。**

- Server：`fast-https`，systemd 单元 `build/packaging/systemd/fast-https.service`
- Monitor：`fast-https-monitor`，单元 `build/packaging/systemd/fast-https-monitor.service`，不 `Requires=` Server
- 包布局状态目录：`/var/lib/fast-https`。归档布局：`./state`
- Monitor 数据：`/var/log/fast-https/monitor` 或 `<log_root>/monitor`

### 2.12 进程内控制面（与新规范的关系）

**部分具备，且不能被独立 Agent 直接调用。**

`modules/safe/control.go` 提供进程内的 `AddBlacklist`、`RemoveBlacklist`、限流调整和 `Snapshot`。注释写明它模拟人执行 CLI，并准备给外部工具用。当前 `cmd/` 没有对应子命令，也没有把它暴露到 `/runtime` 以外的端口。

独立进程要使用它，必须新增受控接口。那不属于规范 12.1–12.3，也不在本阶段执行计划里。不得把 Agent 编进 Server 进程去直接调用这些函数。

## 3. 功能差距

对照规范第 3–6 节与第 12 节。

| 规范能力 | 标记 | 差距 |
| --- | --- | --- |
| 事件驱动 | 部分具备 | 事件在 `events.jsonl`。无 `event_id`，无推送，CLI 丢掉 `snapshot` |
| 事件类型 `upstream_unhealthy` 等 8 种 | 不具备 | 现有 7 种类型见 2.3。无证书、磁盘、CPU、配置失败事件 |
| Agent 状态机与重启恢复 | 不具备 | 无 `agent/` 进程，无任务库 |
| 上下文：服务、日志、配置、operation、5xx 累计 | 已具备 | 经 CLI `--json` 与 `/runtime` |
| 上下文：分上游健康、502/504、CPU/内存/磁盘 | 不具备 | 见 2.2 |
| 规则诊断 | 部分具备 | Monitor 已做阈值判断。Agent 侧诊断对象还不存在 |
| AI 诊断与模型路由 | 不具备 | 仓库无模型客户端。第一阶段规则闭环不依赖它 |
| 计划 / 策略 / 审计 / 升级 | 不具备 | 无对应存储 |
| L1 查询 | 已具备 | 第 2.8 节命令 |
| L2 摘除或恢复 Upstream | 不具备 | 无多节点，无命令 |
| L2 清理缓存 | 不具备 | 无对应 CLI。缓存是代理响应缓存，不是临时目录清扫接口 |
| L3 改配置、reload | 已具备 | 必须人工审批后才允许执行；本阶段执行器不调用 |
| 证书替换、重启、删站点 | 已具备命令，权限上禁止自动执行 | `site delete`、顶层 `stop`/`start`。策略应 DENY |
| 任意 Shell | 不适用 | 禁止。现有架构测试已守管理代码 |
| 审批查询给 DSH | 不具备 | DSH 今天只能调现有 CLI。无待审批列表 |
| 幂等写操作 | 已具备 | 仅限现有事务命令的 `--request-id` |
| Management API | 不具备 | 不作为第一阶段前置。CLI 已覆盖集成表 |

## 4. 需要新增的接口

只列第一阶段闭环所缺的契约。不把 Upstream 摘除、证书续期、资源指标算进本阶段接口。

### 4.1 Monitor 事件 → Agent 事件（适配，不改 Monitor 文件格式）

Monitor 的 `Event` 保持不变，避免破坏 `management/monview` 与 `schema` 测试。

Agent 读到一行后生成自己的记录：

| 字段 | 来源 |
| --- | --- |
| `event_id` | 对 `time` + `type` + `message` + 文件偏移做稳定哈希。Monitor 今天没有可复用的 ID |
| `event_type` | 原样保存 Monitor 的 `type`，另给规范名一个只读映射，见下表 |
| `severity`、`summary` | `severity`、`message` |
| `source` | 固定 `fast_https_monitor` |
| `timestamp` | `time` |
| `resource` | 现有事件没有上游名或证书名。资源先记 `server`（pid / `started_at` 来自 `snapshot`） |
| `metrics` | 抄 `snapshot` 里已有的累计与增量。没有的键省略 |
| `status` | Agent 自己的处理状态，不写回 `events.jsonl` |

映射只用于诊断分类，不假装 Monitor 已经发出规范类型：

| Monitor `type` | 诊断分类 |
| --- | --- |
| `unreachable` | `service_unavailable` |
| `recovered` | 恢复，默认 `IGNORED` 或关闭对应不可达任务 |
| `restarted` | `service_restarted` |
| `5xx_ratio` | `http_5xx_spike`（不是 `http_error_spike` 的 502/504 专义） |
| `port_probe` | `port_unreachable` |
| `stuck_suspected` | `stuck_suspected` |
| `collect_error` | `collect_error` |
| `certificate_expiring` | `certificate_expiring`（仅当 Monitor 配置了 `cert_files`） |
| `certificate_read_error` | `certificate_read_error` |

`upstream_unhealthy`、`disk_space_low`、`high_resource_usage`、`config_apply_failed` 仍不在这张表里。未配置 `cert_files` 时，证书过期仍可由 Agent 主动执行 `cert list`。配置应用失败走已有 `operation get`。

### 4.2 Agent 状态目录（新）

与 `management/state` 分开。路径和表以规范第 1.5 节为准：单个 `agent.db`，纯 Go SQLite 驱动，不使用 Server 的 JSON operation 目录。

扫描时仓库没有数据库。这是 Agent 新增的存储，不是把 `management/state` 改成 SQLite。

| 表 | 实体 |
| --- | --- |
| `agent_event` | 接收到的 Monitor 事件 |
| `agent_task` | 处理任务与状态机 |
| `agent_diagnosis` | 规则诊断 |
| `agent_plan` | 计划。本阶段步骤为空 |
| `agent_approval` | 仅 `pending`，没有批准后的执行 |
| `agent_audit` | 只插入 |
| `agent_offset` | Monitor 文件读偏移，固定一行 |

`agent_operation` 在本阶段不新建写操作记录。一旦将来执行 L3，操作记录继续用现有 `management/operation`，Agent 只保存它返回的 `operation_id`。

任务状态采用规范 5.1 的子集：`PENDING`、`COLLECTING`、`DIAGNOSING`、`PLANNING`、`POLICY_CHECK`、`WAITING_APPROVAL`、`COMPLETED`、`FAILED`、`ESCALATED`、`IGNORED`。本阶段没有 `EXECUTING` / `VERIFYING`，因为执行器不发出写命令。

### 4.3 Agent 调用的现有接口（复用，不新增）

上下文采集只允许下面这些参数固定的调用。实现上是 `exec` 本机 `fast-https` 二进制加固定参数数组，不经过 Shell，不接受模型拼出来的命令字符串。

- `service status --json`
- `service version --json`
- `runtime status --json`
- `runtime stats --json`
- `logs error --json`（带固定的 `--limit`）
- `config show --json`
- `config versions --json`
- `cert list --json`
- `diagnose --json`
- `operation list --json`
- `monitor status --json`

`/runtime` 也可以由 Agent 直接 `GET`，与 `management/rtclient` 的校验一致：`schema_version` 必须为 `"1"`。两条路二选一，执行计划里定为直接 `GET`，避免为读快照再起一次 CLI。

### 4.4 证书巡检（新的 Agent 内部周期任务，不是新 Server 接口）

`certificate_expiring` 在 Monitor 配置了 `cert_files` 时由 Monitor 写入。未配置时，Agent 仍可按自己的周期执行 `cert list --json`。`days_left` 低于策略阈值时生成内部事件，严重级别固定为 warning，计划标记 `REQUIRE_APPROVAL`，动作为空。不调用 `cert renew`，因为该命令不存在。

### 4.5 DSH 可读的查询命令（新 CLI，只读）

沿用 Monitor 的分工：写在独立进程，读在 CLI。

| 命令 | 作用 |
| --- | --- |
| `fast-https agent status --json` | Agent 进程是否在写状态、最后处理时间 |
| `fast-https agent tasks --json` | 任务列表，可按状态过滤 |
| `fast-https agent task get <id> --json` | 单个任务的诊断、计划、策略结论 |
| `fast-https agent approvals --json` | `pending` 计划 |

本阶段不提供 `agent approve`。批准意味着要执行写操作，而写操作还没有允许自动或远程执行的低风险接口。待审批记录先能被看见。

读取实现放在 `management/agentview`，只读打开 `agent.db`，依赖方向与 `management/monview` 相同：不 import `agent/`、`runtime/` 或 `core`。

### 4.6 明确不新增的接口

| 接口 | 原因 |
| --- | --- |
| Agent 写配置的 HTTP API | `config apply` 已是受控入口 |
| Upstream 摘除 API | 数据模型不存在。先做故障转移计划 |
| `cert renew` 控制通道 | 已有单独的安全评审要求，不属于本阶段 |
| 把 `control.go` 暴露成网络接口 | 黑名单与限流是另一条产品线，且会扩大攻击面 |
| Kafka / 数据库 / 模型服务 | 规范第 16 节排除。规则诊断不需要模型 |
| 修改 `events.jsonl` 字段 | 会破坏现有 CLI 与轮转测试 |

## 5. 对待运行验证项

以下不影响本报告的「有没有接口」结论，执行计划里也不把它们当成已通过：

- 停掉未来的 Agent 进程后，业务端口与 `/runtime` 仍可用。这与 Monitor 的设计相同，合入前要用进程测试证明 Agent 没有被 Server import。
- `5xx_ratio` 在真实流量下的事件文本是否足以支撑诊断。规则测试在 `monitor/rules_test.go`，生产阈值以 `monitor.json` 为准。
- DSH 通过 SSH 调用只读 JSON。CLI 收口时此项未做手工记录，见 `docs/develop/plans/cli-monitor-followup.md`。
