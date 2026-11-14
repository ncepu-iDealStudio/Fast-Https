# Batch 4 执行结果：文档同步

日期：2026-11-14。上一批是 `ee8e72e`。

## 1. 改了什么

对照当前代码改了仍在描述旧行为的文档：

- `docs/feature/22-ai-agent.md`：写明唯一可批准的动作是一条 `upstream drain` 或 `upstream undrain`；没有步骤的任务是 `WAITING_HUMAN`；`recovered` 只关闭一条对应的 `unreachable`；状态按心跳分为 `online`、`offline`、`unknown`。
- `docs/develop/plans/ai-agent-execution-plan.md`：开头标明第一阶段原文和后来的可靠性改造不是同一批，并改掉「没有 `agent approve`」「证书过期进入待审批」「status 为 not_running」这几处现况描述。
- `docs/develop/plans/ai-agent-server-gap-plan.md`：指向 Batch 0–3 的结果，并保留「Agent 不能批准 cert renew」。
- `docs/usage.md` 与生成的 `docs/usage.html`：`agent status` 的三种状态。
- `docs/feature/7-proxy.md`：`health` 使用最近一分钟，不再写失败一次后保持到进程重启。

## 2. 仍未实现，文档里保持为未实现

- Agent 不能批准 `cert renew`。
- 没有权重、会话保持、主动健康检查。
- 模型不能改步骤或策略，也不能执行 Shell。
- Agent 没有打进 deb/rpm，也没有 systemd 单元。

## 3. 新增测试

无代码改动。生成文档后执行：

```text
go test ./tools/docgen/ ./tools/manpage/ -count=1
ok  fast-https/tools/docgen
ok  fast-https/tools/manpage
```

## 4. 尚未解决

- 历史扫描报告和设计规范仍按当时的第一阶段写法保留，没有改成现状说明。
- `undrain` 的健康未确认路径没有单独的 CLI 夹具。
- 日志轮转用文件开头哈希，不是 inode。

## 5. 完整 Git diff

相对 `ee8e72e`，不含本报告。

```diff
diff --git a/docs/develop/plans/ai-agent-execution-plan.md b/docs/develop/plans/ai-agent-execution-plan.md
index b00e3fc..82a8837 100644
--- a/docs/develop/plans/ai-agent-execution-plan.md
+++ b/docs/develop/plans/ai-agent-execution-plan.md
@@ -1,6 +1,6 @@
 # fast-https AI Agent 第一阶段执行计划
 
-修订日期：2026-11-10。状态：**A1–A6 已实施**（只读诊断与待审批）。C6 的多上游与 `upstream drain`、C7 的 `cert renew` 通道已在 Server 侧落地。Agent 仍不能批准 `cert renew`。
+修订日期：2026-11-14。状态：**A1–A6 已实施**。其后又落地了人工批准的 `upstream drain` / `upstream undrain`、执行后名单核对、心跳和 schema 版本。Agent 仍不能批准 `cert renew`。下面各节里的「本阶段」描述的是当时的交付，和后面的可靠性改造不是同一批。
 
 设计规范：[fast-https AI agent功能设计与开发规范.md](../../fast-https%20AI%20agent功能设计与开发规范.md)。  
 能力以扫描为准：[fast-https-AI-Agent源码扫描报告.md](../../fast-https-AI-Agent源码扫描报告.md)。  
@@ -21,7 +21,7 @@ Server / Monitor 上要补的观测和写接口另见 [ai-agent-server-gap-plan.
 5. 策略结果只有 `ALLOW` 的空操作、`REQUIRE_APPROVAL`、`DENY`。没有任何写命令进入 `ALLOW`。
 6. 追加审计。进程重启后从 SQLite 恢复未完成任务。
 
-CLI 增加只读的 `agent status|tasks|task get|approvals`。DSH 以后用 SSH 调这四条。本阶段没有 `agent approve`。
+CLI 有 `agent status|tasks|task get|approvals`，以及后来加上的 `agent approve`。DSH 以后用 SSH 调这些命令。
 
 ## 2. 不做
 
@@ -113,7 +113,7 @@ monitor/events.jsonl  --尾读--->  agent/          写 FASTHTTPS_AGENT_DIR/agen
 | 现有基础 | Monitor 已经把 `5xx_ratio`、`unreachable` 等判断做完。Agent 不再重算阈值 |
 | 新增文件 | `agent/diagnose.go`、`agent/plan.go`、`agent/policy.go` 及测试 |
 | 接口 | 输入任务 + 上下文。输出诊断字段：分类、对象、原因、影响、证据（引用上下文键）、是否需要人工。计划字段：`plan_id`、`event_id`、步骤、风险、验证方式、失败处理 |
-| 数据 | 行写入 `agent_diagnosis`、`agent_plan`。`5xx_ratio` 与 `stuck_suspected`、`port_probe` 的计划为 `REQUIRE_APPROVAL` 且步骤为空，任务进入 `WAITING_APPROVAL` 并插入 `agent_approval`。`collect_error` 进入 `ESCALATED`。无法识别的 `type` 进入 `ESCALATED`，不执行 |
+| 数据 | 行写入 `agent_diagnosis`、`agent_plan`。没有可执行步骤时任务是 `WAITING_HUMAN`，不插入待审批。恰好一条当前可摘除的 `upstream drain` 才是 `WAITING_APPROVAL`。`collect_error` 与无法识别的 `type` 进入 `ESCALATED`，不执行 |
 | 依赖 | A2 |
 | 验收 | 构造一条 `5xx_ratio`，得到 `http_5xx_spike` 诊断，策略不是 `ALLOW` 的写操作。构造 `message` 含 `Ignore previous instructions` 的事件，审计与计划里保留原文，策略不因此出现新动作 |
 | 测试 | `TestDiagnoseMapping`、`TestPolicyDeniesMutations`、`TestInjectionIsData`、`TestRecoveredClosesUnreachable` |
@@ -133,7 +133,7 @@ monitor/events.jsonl  --尾读--->  agent/          写 FASTHTTPS_AGENT_DIR/agen
 | 接口 | 仅 `cert list --json`。阈值默认 30 天，配置项 `cert_warn_days` |
 | 数据 | 合成事件类型 `certificate_expiring`。若 `events.jsonl` 里已经有同一证书的该事件，不再插第二条任务。`event_id` 在需要合成时由域名 + `not_after` 哈希。同一证书同一到期日不重复开任务 |
 | 依赖 | A2、A3 |
-| 验收 | `days_left` 小于阈值生成 `WAITING_APPROVAL`。大于等于阈值不生成。CLI 失败时记审计，不编造证书状态 |
+| 验收 | `days_left` 小于阈值生成 `WAITING_HUMAN`（没有续期步骤）。大于等于阈值不生成。CLI 失败时记审计，不编造证书状态 |
 | 测试 | `TestCertPollThreshold`，CLI 替身返回夹具 JSON |
 
 ### A5 只读查询
@@ -146,7 +146,7 @@ monitor/events.jsonl  --尾读--->  agent/          写 FASTHTTPS_AGENT_DIR/agen
 | 目标 | DSH 能用现有 CLI 风格读到任务与待审批 |
 | 现有基础 | `management/monview`、`cmd/monitor.go`、`cmd/cli_output.go` |
 | 新增文件 | `management/agentview/agentview.go`、`cmd/agent.go`、`cmd/cli_agent_test.go`；在 `cmd/commands.go` 注册 |
-| 接口 | 扫描报告 4.5 的四条命令。未知子命令退出码 2。库文件不存在时 `agent status` 为 `not_running`，退出码 0。打开方式为只读 |
+| 接口 | 扫描报告 4.5 的四条命令，外加后来的 `agent approve`。未知子命令退出码 2。库文件不存在或没有心跳时 `agent status` 为 `unknown`，退出码 0。查询打开方式为只读 |
 | 数据 | 只读 `agent.db`。不创建任务 |
 | 依赖 | A1 的表结构稳定之后。可与 A3 并行编写读取测试 |
 | 验收 | `--json` 的 stdout 只有一个对象。待审批列表不含私钥、不含 `ssl_certificate_key`。只读打开不得改变库文件大小 |
@@ -183,7 +183,7 @@ A5 只依赖表结构，可以在 A3 之后立即做，让诊断结果能被看
 | --- | --- |
 | 15.1 独立启动、解耦、收事件、持久化、重启恢复、采集上下文、结构化诊断和计划 | A1–A3、A6 |
 | 15.2 独立策略、白名单、模型不能执行 Shell、未授权不执行、高风险须审批、审计 | A3。重试上限：同一任务自动流程只跑一遍，失败进入 `FAILED` 或 `ESCALATED`，不循环写操作。冷却沿用 Monitor 已有冷却，Agent 不再对同一 `event_id` 开第二次任务 |
-| 15.3 自动修复、修复验证、批准后执行 | **本阶段不做。** 没有可安全自动执行的写接口。待审批只记录、可查询 |
+| 15.3 自动修复、修复验证、批准后执行 | 第一阶段不做自动修复。后来的 `agent approve` 只执行一条已写好的 drain 或 undrain，并核对名单。没有其他自动写动作 |
 | 15.4 Agent 停止不影响 Server；模型不可用 | 无模型依赖，故降级条款不适用。解耦由分层测试与「Server 不 import agent」保证 |
 | 12.1 Upstream 自动摘除 | 阻塞于 `upstream-failover-plan.md`。该计划落地并提供明确的 CLI 之后，再单独立项把对应动作加入策略表 |
 | 12.2 502/504 | 诊断引用 `/runtime` 的 `responses_502_total` 与 `responses_504_total`。Monitor 仍只有 `5xx_ratio` 事件，不另写 502/504 规则 |
@@ -196,4 +196,4 @@ A5 只依赖表结构，可以在 A3 之后立即做，让诊断结果能被看
 1. Upstream 故障转移本身（已有计划），然后才是摘除动作的策略与验证。
 2. 审批后的 `cert renew`。回环令牌通道已经落地，策略表仍拒绝这条命令，要单独再开批准步骤。
 3. 本地模型只改写诊断说明。已实现：配置 `FASTHTTPS_AGENT_MODEL_URL` 后走 OpenAI 兼容接口；超时、失败或说明里出现写命令时保留规则原文。分类、步骤和策略仍由 Go 决定。
-4. `agent approve`：只执行计划里已经写好的一条 `upstream drain` 或 `upstream undrain`。批准时核对 `plan_id` 与步骤哈希。哈希不一致或会摘掉最后一个地址时不改名单。模型不能把该步骤改成自动执行。`cert renew` 仍不能批准。
+4. `agent approve` 已实现：只执行计划里已经写好的一条 `upstream drain` 或 `upstream undrain`。批准时重算计划哈希，用条件更新占用审批，写完名单后核对。哈希不一致、会摘掉最后一个地址，或健康状态不再允许摘除时不改名单。`undrain` 后若快照不是 `healthy`，任务停在 `WAITING_HUMAN`。模型不能把该步骤改成自动执行。`cert renew` 仍不能批准。
diff --git a/docs/develop/plans/ai-agent-server-gap-plan.md b/docs/develop/plans/ai-agent-server-gap-plan.md
index 49c3dc4..5a95351 100644
--- a/docs/develop/plans/ai-agent-server-gap-plan.md
+++ b/docs/develop/plans/ai-agent-server-gap-plan.md
@@ -1,6 +1,6 @@
 # fast-https 核心补齐计划（为 AI Agent）
 
-修订日期：2026-11-10。状态：**C1–C7 已实施。** C7 只开放 CLI 续期通道。Agent 批准 `cert renew` 仍是 DENY。
+修订日期：2026-11-14。状态：**C1–C7 已实施。** C7 只开放 CLI 续期通道。Agent 批准 `cert renew` 仍是 DENY。Agent 侧的事务、审批占用、名单核对、心跳和 schema 版本见 `ai-agent-reliability-batch-0-result.md` 到 `batch-3-result.md`，那些不改变本表里的 Server 缺口结论。
 
 依据：[fast-https-AI-Agent源码扫描报告.md](../../fast-https-AI-Agent源码扫描报告.md)。  
 Agent 进程用 Go 实现，见规范第 1.4 节。本文不改语言选择，只列 Server、Monitor 与管理核心上还缺的观测和受控操作。Agent 任务在 [ai-agent-execution-plan.md](ai-agent-execution-plan.md)。
diff --git a/docs/feature/22-ai-agent.md b/docs/feature/22-ai-agent.md
index 786623a..5d37162 100644
--- a/docs/feature/22-ai-agent.md
+++ b/docs/feature/22-ai-agent.md
@@ -1,20 +1,21 @@
 # 22. 独立 AI Agent
 
-状态：✅ 第一阶段已实现（只读诊断与待审批）。自动摘除上游和证书续期未做。
+状态：✅ 已实现只读诊断，以及经人工批准的 `upstream drain` / `upstream undrain`。证书续期不能由 Agent 批准。
 
 ## 功能
 
 `agent` 是独立于 Server 的进程。它尾读 Monitor 的 `events.jsonl`，用固定参数调用本机 `fast-https` 的只读命令，并直接 `GET /runtime`。然后按事件类型写诊断和计划。
 
-本阶段不调用模型，不执行 `config apply`、`service reload`、摘除上游、证书续期或重启。写操作在策略里是 `DENY`。需要人看的计划进入 `WAITING_APPROVAL`，可用 CLI 查询，还没有 `agent approve`。
+模型只改写诊断说明。`config apply`、`service reload`、证书续期和重启仍是 `DENY`。有一条合法摘除或恢复步骤时任务进入 `WAITING_APPROVAL`，要用 `agent approve <plan-id> --yes` 才会执行。没有可执行步骤时进入 `WAITING_HUMAN`，不进审批队列。
 
 停掉 Agent 不影响业务端口和 `/runtime`。Server 不 import `agent`。
 
 | Monitor `type` | 结果 |
 | --- | --- |
-| `5xx_ratio`、`port_probe`、`stuck_suspected`、`certificate_expiring` | `WAITING_APPROVAL`，步骤为空 |
+| `5xx_ratio`、`port_probe` | 恰好一个地址当前 `unhealthy`、另有 `healthy` 地址、且目标未摘除时，生成一条 `upstream drain`，任务为 `WAITING_APPROVAL`。否则 `WAITING_HUMAN`，步骤为空 |
+| `stuck_suspected`、`certificate_expiring` | `WAITING_HUMAN`，没有可执行步骤 |
 | `unreachable`、`collect_error`、无法识别的类型 | `ESCALATED` |
-| `recovered` | 把同一次启动里未关闭的 `unreachable` 标为 `COMPLETED` |
+| `recovered` | 只关闭同一 `started_at` 下恰好一条更早的 `unreachable`。多于一条时都不关闭，恢复任务进入 `WAITING_HUMAN` |
 | `restarted` | 只记录，`COMPLETED` |
 
 `days_left` 低于 30 时，若事件文件里还没有这张证书的 `certificate_expiring`，Agent 会用 `cert list --json` 补一条。`cert list` 失败只记审计。
@@ -56,7 +57,7 @@ fast-https agent task get t_<event_id> --json
 fast-https agent approvals --json
 ```
 
-库文件不存在时 `agent status` 为 `not_running`，退出码 0。
+库文件不存在或没有心跳时 `agent status` 为 `unknown`，退出码 0。心跳在 30 秒内是 `online`，更旧是 `offline`。
 
 ## 实现方案
 
@@ -72,7 +73,7 @@ fast-https agent approvals --json
 
 ## 已知限制
 
-- 不自动修改配置，不摘除上游，不续期证书。
+- 支持的写动作只有人工批准后的一条 `upstream drain` 或 `upstream undrain`。批准会重算计划哈希，先占住 `pending` 审批，写 operation，再改名单。名单核对通过才把任务标成 `COMPLETED`。`undrain` 后快照不是 `healthy` 时停在 `WAITING_HUMAN`。不能批准证书续期。
 - 模型可选。设置 `FASTHTTPS_AGENT_MODEL_URL`（以及可选的 `FASTHTTPS_AGENT_MODEL_EXTERNAL_URL`）后，只重写诊断说明。没配置、超时，或说明里出现写命令时，仍用规则原文。策略和步骤不交给模型。
 - 不打包进 deb/rpm，也没有 systemd 单元。
 - 待审批可以查看。`agent approve <plan-id> --yes` 只执行计划里唯一的 `upstream drain` 或 `upstream undrain`。哈希对不上，或会摘掉最后一个地址时，名单不变。不能批准证书续期或其他写命令。
diff --git a/docs/feature/7-proxy.md b/docs/feature/7-proxy.md
index 029e71c..0779ff4 100644
--- a/docs/feature/7-proxy.md
+++ b/docs/feature/7-proxy.md
@@ -30,6 +30,6 @@ HTTP/HTTPS 反向代理：转发到 `proxy_pass` 上游、请求头改写、上
 
 ## 已知限制
 
-- 多地址只做轮询，以及拨号失败或超时后的下一次尝试。没有权重、会话保持和主动探测。摘除名单不改 `proxy_pass`，计数里的 `healthy` 在该地址失败过一次后保持 false，直到进程重启。
+- 多地址只做轮询，以及拨号失败或超时后的下一次尝试。没有权重、会话保持和主动探测。摘除名单不改 `proxy_pass`。`health` 看最近一分钟：成功晚于失败是 `healthy`，失败晚于成功是 `unhealthy`，没有样本或已过窗口是 `unknown`。`healthy` 只在 `health` 为 `healthy` 时为 true。累计失败次数仍然保留。
 - HTTPS 上游 `InsecureSkipVerify: true`（不校验上游证书）。
 - 响应不做压缩（客户端接受压缩而上游未压时透传原文）。
diff --git a/docs/usage.html b/docs/usage.html
index 087d760..07e5dcc 100644
--- a/docs/usage.html
+++ b/docs/usage.html
@@ -468,7 +468,7 @@ fast-https site remove blog --yes</code></pre>
 <tr><td><code>monitor status</code></td><td>只读 Monitor 的 <code>status.json</code>。文件不存在时 <code>status</code> 为 <code>not_installed</code>（退出码 0）；采集时间早于 30 秒为 <code>stale</code>。不连接 Monitor 进程，也不负责启停</td></tr>
 <tr><td><code>monitor events</code></td><td>只读 <code>events.jsonl</code>。<code>--since</code>、<code>--limit</code>（默认 100，上限 1000）、<code>--severity</code>（<code>info</code>、<code>warning</code>、<code>error</code>、<code>critical</code>）。坏行跳过并计数，事件文本只作为数据返回。有 <code>snapshot</code> 时 JSON 带回白名单摘要</td></tr>
 <tr><td><code>upstream drain &lt;address&gt;</code> / <code>upstream undrain &lt;address&gt;</code></td><td>把地址放进或移出摘除名单，必须 <code>--yes</code>。不改 <code>proxy_pass</code>。某个 location 因此没有剩余地址时退出码 6，名单不变</td></tr>
-<tr><td><code>agent status</code></td><td>只读 Agent 状态库。库不存在时 <code>status</code> 为 <code>not_running</code>（退出码 0）。不启动 Agent，不创建任务</td></tr>
+<tr><td><code>agent status</code></td><td>只读 Agent 状态库。没有库或没有心跳时 <code>status</code> 为 <code>unknown</code>，心跳超过 30 秒为 <code>offline</code>，30 秒内为 <code>online</code>（退出码 0）。不启动 Agent，不创建任务</td></tr>
 <tr><td><code>agent tasks</code> / <code>agent task get</code> / <code>agent approvals</code></td><td>只读任务、一条任务的诊断与计划、以及 <code>pending</code> 审批。审批原因会脱敏</td></tr>
 <tr><td><code>agent approve &lt;plan-id&gt; --yes</code></td><td>只执行计划中唯一的 <code>upstream drain</code> 或 <code>upstream undrain</code>。步骤哈希不一致时退出码 10，名单不变。会摘掉最后一个地址时退出码 6，名单和 <code>proxy_pass</code> 都不变</td></tr>
 <tr><td><code>site init [name]</code></td><td>在 <code>http.include</code> 目录下生成 <code>&lt;name&gt;.json</code>（交互向导或 <code>--listen/--server-name/--type/--proxy-pass/--tls</code>；<code>--listen</code> 可重复以生成多端口站点）。只写文件，提示 reload</td></tr>
diff --git a/docs/usage.md b/docs/usage.md
index 14a53d5..e98b527 100644
--- a/docs/usage.md
+++ b/docs/usage.md
@@ -511,7 +511,7 @@ Linux 包会安装 `/etc/logrotate.d/fast-https`：每天轮转、保留 14 份
 | `monitor status` | 只读 Monitor 的 `status.json`。文件不存在时 `status` 为 `not_installed`（退出码 0）；采集时间早于 30 秒为 `stale`。不连接 Monitor 进程，也不负责启停 |
 | `monitor events` | 只读 `events.jsonl`。`--since`、`--limit`（默认 100，上限 1000）、`--severity`（`info`、`warning`、`error`、`critical`）。坏行跳过并计数，事件文本只作为数据返回。有 `snapshot` 时 JSON 带回白名单摘要 |
 | `upstream drain <address>` / `upstream undrain <address>` | 把地址放进或移出摘除名单，必须 `--yes`。不改 `proxy_pass`。某个 location 因此没有剩余地址时退出码 6，名单不变 |
-| `agent status` | 只读 Agent 状态库。库不存在时 `status` 为 `not_running`（退出码 0）。不启动 Agent，不创建任务 |
+| `agent status` | 只读 Agent 状态库。没有库或没有心跳时 `status` 为 `unknown`，心跳超过 30 秒为 `offline`，30 秒内为 `online`（退出码 0）。不启动 Agent，不创建任务 |
 | `agent tasks` / `agent task get` / `agent approvals` | 只读任务、一条任务的诊断与计划、以及 `pending` 审批。审批原因会脱敏 |
 | `agent approve <plan-id> --yes` | 只执行计划中唯一的 `upstream drain` 或 `upstream undrain`。步骤哈希不一致时退出码 10，名单不变。会摘掉最后一个地址时退出码 6，名单和 `proxy_pass` 都不变 |
 | `site init [name]` | 在 `http.include` 目录下生成 `<name>.json`（交互向导或 `--listen/--server-name/--type/--proxy-pass/--tls`；`--listen` 可重复以生成多端口站点）。只写文件，提示 reload |
```
