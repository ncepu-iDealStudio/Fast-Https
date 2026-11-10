# fast-https AI Agent 第一阶段执行计划

修订日期：2026-11-10。状态：**A1–A6 已实施**（只读诊断与待审批）。自动摘除与 `cert renew` 仍见补齐计划 C6、C7。

设计规范：[fast-https AI agent功能设计与开发规范.md](../../fast-https%20AI%20agent功能设计与开发规范.md)。  
能力以扫描为准：[fast-https-AI-Agent源码扫描报告.md](../../fast-https-AI-Agent源码扫描报告.md)。  
Server / Monitor 上要补的观测和写接口另见 [ai-agent-server-gap-plan.md](ai-agent-server-gap-plan.md)，不并进下面的 A1–A6。

本计划不假设这些能力已经存在：Upstream 多节点与摘除、502/504 的 Monitor 事件、证书续期命令、Management HTTP API、审批后的执行、本地模型。`/runtime` 已有 502/504 累计和按地址的失败次数，诊断可以引用。规范 12.1 的自动摘除不在本阶段。规范 15.3 里「执行一种低风险修复」在 C6 或 C7 之前标为做不到。

已撤回的 `agent-refactor-plan.md` 不执行。不把 Agent 链进 Server，不直接调用 `modules/safe/control.go`。

## 1. 本阶段交付

一个独立进程 `fast-https-agent`，代码在 `agent/`。语言、目录和表结构以规范第 1.4、1.5 节为准。它不 import `runtime/`、`core`、`cmd`、`management/`。

1. 尾读 Monitor 的 `events.jsonl`（默认 `<log_root>/monitor`，可用 `FASTHTTPS_MONITOR_DIR`）。
2. 把事件收成自己的任务，写入 `FASTHTTPS_AGENT_DIR/agent.db`（SQLite）。
3. 用固定参数调用本机 `fast-https --json`，并直接 `GET /runtime`，采集上下文。
4. 用规则写出诊断和计划。
5. 策略结果只有 `ALLOW` 的空操作、`REQUIRE_APPROVAL`、`DENY`。没有任何写命令进入 `ALLOW`。
6. 追加审计。进程重启后从 SQLite 恢复未完成任务。

CLI 增加只读的 `agent status|tasks|task get|approvals`。DSH 以后用 SSH 调这四条。本阶段没有 `agent approve`。

## 2. 不做

- 修改 `monitor/events.go` 的字段或新增 Monitor 事件类型。
- 实现 `cert renew`、Upstream 故障转移、缓存清理命令、资源指标。
- 调用 `config apply`、`service reload`、`site *` 写命令、顶层 `start`/`stop`。
- 引入 `github.com/openai/openai-go`。模型包只保留接口；未配置时路由器返回「模型不可用」，规则诊断继续。
- 使用 Python 作为生产进程，或引入通用 Agent 框架。
- 打包进 deb/rpm。进程先以 `go run ./agent` 运行，打包另案，避免和 Monitor 第 8 步的发行版验证缠在一起。
- 把待审批自动变成执行。

## 3. 模块边界

```text
monitor/events.jsonl  --尾读--->  agent/          写 FASTHTTPS_AGENT_DIR/agent.db
                                      |
                                      | GET 127.0.0.1:9088/runtime
                                      | exec fast-https <固定子命令> --json
                                      v
                               agent.db（SQLite）
                                      |
                                      | 只读
                                      v
                         management/agentview  -->  cmd/agent.go
```

`agent/` 的分层约束加进 `tests/arch/layering_test.go`：只允许 import 标准库和自己。  
`management/agentview` 与 `monview` 相同，不依赖 `runtime/`、`core`、`agent/`。

状态机只用这些值：`PENDING`、`COLLECTING`、`DIAGNOSING`、`PLANNING`、`POLICY_CHECK`、`WAITING_APPROVAL`、`COMPLETED`、`FAILED`、`ESCALATED`、`IGNORED`。

策略白名单（写死在 `agent/policy.go`，不从模型或事件文本加载）：

| 动作 | 结果 |
| --- | --- |
| 无动作的诊断报告 | `ALLOW`（不产生子进程写操作） |
| 任何配置、reload、站点、证书写入、进程重启 | `DENY` |
| `certificate_expiring` 的处理建议 | `REQUIRE_APPROVAL`，步骤列表为空 |
| 事件 `message` 里出现的命令文本 | 忽略，不当作动作 |

`recovered`：若存在同一 `started_at` 周期内未关闭的 `unreachable` 任务，把它标为 `COMPLETED`，依据是后续事件本身，不再发请求。对不上则 `IGNORED`。

## 4. 任务

### A1 事件收件与 SQLite

状态：已实施（`agent/ingest.go`，`TestTailIdempotent`、`TestResumeOffset`、`TestSkipBadLine`）。

| 项 | 内容 |
| --- | --- |
| 模块 | `agent/` Event Receiver、Queue、Audit |
| 目标 | 尾读 JSONL。`event_id` 优先用 Monitor 写下的 32 位十六进制；旧行没有时按 `monitor/events.go` 的 `eventID` 补齐。写入规范第 1.5 节的表，重启后续跑 |
| 现有基础 | `monitor/events.go` 的 `Event` 字段。仓库目前没有数据库，SQLite 只给 Agent 用 |
| 新增文件 | `agent/main.go`、`agent/config.go`、`agent/tail.go`、`agent/store.go`、`agent/task.go`、`agent/model/provider.go`、`agent/model/router.go`，以及对应测试 |
| 接口 | 读：`events.jsonl`。写：`agent.db` 的 `agent_event`、`agent_task`、`agent_audit`、`agent_offset`。驱动用纯 Go 的 SQLite（`modernc.org/sqlite`），通过 `database/sql` 访问，构建不需要 CGO |
| 数据 | `event_id`、`monitor_offset`、任务状态、`attempts`。同一 `event_id` 再次出现不新建任务 |
| 依赖 | 无 |
| 验收 | 重复行只产生一条任务；进程杀掉再起，从 `agent_offset` 继续，不重跑已 `COMPLETED` 的任务；坏 JSON 行跳过并在 `agent_audit` 里记一笔 |
| 测试 | `TestTailIdempotent`、`TestResumeOffset`、`TestSkipBadLine` |

`main` 标志：`-monitor-dir`、`-state-dir`、`-once`（处理当前文件后退出，供测试）。默认周期用轮询文件，间隔 2 秒，不引入 fsnotify。

### A2 上下文采集

状态：已实施（`agent/collect.go`，`TestCLIAllowlistRejects`、`TestRuntimeSchema`、`TestCollectRuntime`）。

| 项 | 内容 |
| --- | --- |
| 模块 | Context Collector |
| 目标 | 每个非 `IGNORED` 任务拿到一份有界上下文 |
| 现有基础 | `cmd/runtime_endpoint.go`、`management/rtclient` 的 schema 校验规则（复制字段判断，不 import） |
| 新增文件 | `agent/collect.go`、`agent/runtime.go`、`agent/cli.go` |
| 接口 | `GET /runtime`，超时 1 秒。CLI 允许列表见扫描报告 4.3，超时 5 秒，stdout 必须是一个 JSON 对象 |
| 数据 | 上下文写入 `agent_task.context_json`。单份日志最多保留 CLI 已有的 `--limit`（建议 20）。采集失败记入 `warnings`，任务继续诊断 |
| 依赖 | A1 |
| 验收 | 命令不在白名单时 `cli.go` 拒绝执行。白名单调用不使用 `sh -c`。`/runtime` 的 `schema_version` 不是 `"1"` 时上下文标 `runtime: unavailable` |
| 测试 | `TestCLIAllowlistRejects`、`TestRuntimeSchema`、用 `httptest` 的 `TestCollectRuntime`。CLI 测试启动一个假的 `fast-https` 脚本不可行时，注入 `exec` 函数替身 |

假二进制只在测试里由替身函数代替，生产路径使用 `exec.Command(binary, args...)`，`args` 来自白名单表，不拼接 Shell。

### A3 规则诊断、计划与策略

状态：已实施（`agent/diagnose.go`、`agent/plan.go`、`agent/policy.go`，`TestDiagnoseMapping`、`TestPolicyDeniesMutations`、`TestInjectionIsData`、`TestRecoveredClosesUnreachable`）。

| 项 | 内容 |
| --- | --- |
| 模块 | Diagnosis、Planner、Policy |
| 目标 | 对扫描报告 4.1 的映射给出结构化诊断；写操作一律 `DENY` |
| 现有基础 | Monitor 已经把 `5xx_ratio`、`unreachable` 等判断做完。Agent 不再重算阈值 |
| 新增文件 | `agent/diagnose.go`、`agent/plan.go`、`agent/policy.go` 及测试 |
| 接口 | 输入任务 + 上下文。输出诊断字段：分类、对象、原因、影响、证据（引用上下文键）、是否需要人工。计划字段：`plan_id`、`event_id`、步骤、风险、验证方式、失败处理 |
| 数据 | 行写入 `agent_diagnosis`、`agent_plan`。`5xx_ratio` 与 `stuck_suspected`、`port_probe` 的计划为 `REQUIRE_APPROVAL` 且步骤为空，任务进入 `WAITING_APPROVAL` 并插入 `agent_approval`。`collect_error` 进入 `ESCALATED`。无法识别的 `type` 进入 `ESCALATED`，不执行 |
| 依赖 | A2 |
| 验收 | 构造一条 `5xx_ratio`，得到 `http_5xx_spike` 诊断，策略不是 `ALLOW` 的写操作。构造 `message` 含 `Ignore previous instructions` 的事件，审计与计划里保留原文，策略不因此出现新动作 |
| 测试 | `TestDiagnoseMapping`、`TestPolicyDeniesMutations`、`TestInjectionIsData`、`TestRecoveredClosesUnreachable` |

诊断原因使用固定中文模板，引用事件 `message` 与上下文里的计数。不调用模型。

### A4 证书巡检

状态：已实施（`agent/certpoll.go`，`TestCertPollThreshold`、`TestCertPollSkipsWhenMonitorAlreadyWrote`、`TestCertPollFailureDoesNotInvent`）。

| 项 | 内容 |
| --- | --- |
| 模块 | 内部周期任务，复用 A1 的存储 |
| 目标 | 在 Monitor 没有证书事件的前提下，发现即将过期的证书并升级人工 |
| 现有基础 | `cert list --json` 的 `days_left` |
| 新增文件 | `agent/certpoll.go` |
| 接口 | 仅 `cert list --json`。阈值默认 30 天，配置项 `cert_warn_days` |
| 数据 | 合成事件类型 `certificate_expiring`。若 `events.jsonl` 里已经有同一证书的该事件，不再插第二条任务。`event_id` 在需要合成时由域名 + `not_after` 哈希。同一证书同一到期日不重复开任务 |
| 依赖 | A2、A3 |
| 验收 | `days_left` 小于阈值生成 `WAITING_APPROVAL`。大于等于阈值不生成。CLI 失败时记审计，不编造证书状态 |
| 测试 | `TestCertPollThreshold`，CLI 替身返回夹具 JSON |

### A5 只读查询

状态：已实施（`management/agentview`、`cmd/agent.go`，`TestAgentStatusMissing`、`TestAgentApprovalsReadOnly`、`TestAgentTaskJSON`）。

| 项 | 内容 |
| --- | --- |
| 模块 | `management/agentview`、`cmd/agent.go` |
| 目标 | DSH 能用现有 CLI 风格读到任务与待审批 |
| 现有基础 | `management/monview`、`cmd/monitor.go`、`cmd/cli_output.go` |
| 新增文件 | `management/agentview/agentview.go`、`cmd/agent.go`、`cmd/cli_agent_test.go`；在 `cmd/commands.go` 注册 |
| 接口 | 扫描报告 4.5 的四条命令。未知子命令退出码 2。库文件不存在时 `agent status` 为 `not_running`，退出码 0。打开方式为只读 |
| 数据 | 只读 `agent.db`。不创建任务 |
| 依赖 | A1 的表结构稳定之后。可与 A3 并行编写读取测试 |
| 验收 | `--json` 的 stdout 只有一个对象。待审批列表不含私钥、不含 `ssl_certificate_key`。只读打开不得改变库文件大小 |
| 测试 | `TestAgentStatusMissing`、`TestAgentApprovalsReadOnly`、`TestAgentTaskJSON` |

查询命令不负责启动 Agent。

### A6 收口

状态：已实施（`docs/feature/22-ai-agent.md`，`TestAgentDoesNotImportServer`）。

| 项 | 内容 |
| --- | --- |
| 模块 | 测试、分层、使用说明 |
| 目标 | 证明 Agent 不是 Server 的依赖，并把行为写进功能文档 |
| 现有基础 | `tests/arch/layering_test.go`、`docs/feature/21-monitor.md` 的写法 |
| 修改文件 | `tests/arch/layering_test.go`；新增 `docs/feature/22-ai-agent.md`；`docs/usage.md` 增加 `agent` 只读命令一段后执行 `go run ./tools/docgen`；`go run ./tools/manpage` |
| 验收 | `go test ./agent/ ./management/agentview/ ./cmd/ -count=1` 通过；`go test ./tests/arch/` 通过；`go test ./runtime/... ./monitor/` 不因 Agent 变红 |
| 测试 | 分层测试拒绝 `agent` import `fast-https/runtime` 或 `fast-https/cmd` |

不在 A6 跑全量 `-race ./...` 或 distro 安装。那些仍属于 `cli-monitor-followup.md` 的未完成项。

## 5. 建议顺序

A1 → A2 → A3 → A5 → A4 → A6。

A5 只依赖表结构，可以在 A3 之后立即做，让诊断结果能被看见。A4 依赖 CLI 替身与策略，放在 A3 之后。

一步一提交。每步的测试名就是上表「测试」列，不另起无法自动验证的手工项。

## 6. 和规范验收的对应

| 规范条款 | 本阶段 |
| --- | --- |
| 15.1 独立启动、解耦、收事件、持久化、重启恢复、采集上下文、结构化诊断和计划 | A1–A3、A6 |
| 15.2 独立策略、白名单、模型不能执行 Shell、未授权不执行、高风险须审批、审计 | A3。重试上限：同一任务自动流程只跑一遍，失败进入 `FAILED` 或 `ESCALATED`，不循环写操作。冷却沿用 Monitor 已有冷却，Agent 不再对同一 `event_id` 开第二次任务 |
| 15.3 自动修复、修复验证、批准后执行 | **本阶段不做。** 没有可安全自动执行的写接口。待审批只记录、可查询 |
| 15.4 Agent 停止不影响 Server；模型不可用 | 无模型依赖，故降级条款不适用。解耦由分层测试与「Server 不 import agent」保证 |
| 12.1 Upstream 自动摘除 | 阻塞于 `upstream-failover-plan.md`。该计划落地并提供明确的 CLI 之后，再单独立项把对应动作加入策略表 |
| 12.2 502/504 | 诊断引用 `/runtime` 的 `responses_502_total` 与 `responses_504_total`。Monitor 仍只有 `5xx_ratio` 事件，不另写 502/504 规则 |
| 12.3 证书 | A4 的 L1 巡检与待审批。不续期 |

## 7. 完成后再开的项

只在本阶段测试通过后考虑，不提前写进 A1–A6 的文件：

1. Upstream 故障转移本身（已有计划），然后才是摘除动作的策略与验证。
2. `cert renew` 的回环令牌通道（CLI 计划 S13），然后才允许审批后的续期执行。
3. 本地模型只改写诊断说明。已实现：配置 `FASTHTTPS_AGENT_MODEL_URL` 后走 OpenAI 兼容接口；超时、失败或说明里出现写命令时保留规则原文。分类、步骤和策略仍由 Go 决定。
4. `agent approve`：仅当第 1 或第 2 项的写命令已经存在，并且批准绑定 `plan_id` 与参数哈希。
