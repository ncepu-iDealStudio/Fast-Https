# fast-https 核心补齐计划（为 AI Agent）

修订日期：2026-11-09。状态：**C1–C5 已实施，C6、C7 未做。**

依据：[fast-https-AI-Agent源码扫描报告.md](../../fast-https-AI-Agent源码扫描报告.md)。  
Agent 进程用 Go 实现，见规范第 1.4 节。本文不改语言选择，只列 Server、Monitor 与管理核心上还缺的观测和受控操作。Agent 任务在 [ai-agent-execution-plan.md](ai-agent-execution-plan.md)。

排序同时满足两件事：实现从简单到复杂；对请求路径的影响从小到大。前四步不进入 `runtime/events` 和 `modules/proxy`。热路径计数放在中间。多上游和证书写通道放在最后，因为它们改变转发或新增写接口。

`schema_version` 保持 `"1"`。新字段只追加，旧字段含义不变。读不到的字段省略，不填 0 冒充。

## 1. 要补的缺口

| 缺口 | 现在的代码 | 影响的场景 |
| --- | --- | --- |
| `monitor events` 丢掉 `snapshot` | `management/monview/monview.go` 的 `rawEvent` 只解 `time/type/severity/message`，文件里其实已有 `snapshot` | Agent 经 CLI 取事件时看不到累计请求和 5xx |
| 事件没有稳定 `event_id` | `monitor/events.go` 的 `Event` | 去重只能靠内容哈希 |
| 没有证书即将过期事件 | Monitor 不读证书；`cert list` 已能给出 `days_left` | 规范 12.3 |
| 5xx 没有拆开 502 / 504 | `runtime/metrics/requests.go` 的 `RequestEnd` 只按百位加一 | 规范 12.2 |
| 上游失败不按地址拆 | `modules/proxy/metrics.go` 四个进程级计数，`noteDialFail` 不带地址 | 规范 12.1 的诊断，仍不能摘除节点 |
| 没有多上游、没有摘除命令 | `docs/feature/7-proxy.md`：`proxy_pass` 单地址 | 规范 12.1 的 L2 修复 |
| 没有 `cert renew` | `cmd/cert.go` 只有 `list` / `check`。续期在 `modules/certmanager` 进程内 | 规范 12.3 里经审批后的执行 |

配置应用失败已经在 `management/operation` 里，状态为 `failed`。不再为它加 Monitor 事件，Agent 读 `operation list`。

CPU、内存、磁盘不放进 Server。若要做，只在 Monitor 里读操作系统，读不到就写未知，不写 0。这不阻塞三个场景，本计划不单列步骤。

`modules/safe/control.go` 继续只在进程内。不把它做成网络接口，也不把它当成 Upstream 摘除的替代品。

不新增 Management HTTP API。查询仍走现有 CLI 和 `GET /runtime`，写配置仍走 `config apply`。

## 2. 步骤

### C1 把已落盘的事件摘要交回 CLI

状态：已实施（`management/monview`，`TestEventSnapshotWhitelist`）。

| 项 | 内容 |
| --- | --- |
| 影响 | 无。不改 Server，不改 Monitor 写盘格式 |
| 改动 | `management/monview/monview.go`：`Event` 增加可选 `snapshot`，字段与 `monitor.EventSummary` 一致（`state`、`pid`、`started_at`、`requests_total`、`responses_5xx_total`、`requests_delta`、`responses_5xx_delta`）。缺省时省略 |
| 不改 | `events.jsonl` 的现有键。旧文件没有 `snapshot` 时查询仍成功 |
| 测试 | `monview` 单测：有摘要则出现在 JSON；无摘要则键不存在；注入用的坏字段不进入输出 |
| 验收 | `monitor events --json` 能看到摘要。`cmd` 的日志注入测试仍把 `message` 当数据 |

### C2 事件追加 `event_id`

状态：已实施（`monitor/events.go` 的 `eventID`，`TestEventIDStable`）。CLI 只接受 32 位十六进制。

| 项 | 内容 |
| --- | --- |
| 影响 | 无请求路径。只改 Monitor 写事件 |
| 改动 | `monitor/events.go` 的 `Event` 增加 `event_id`。内容是 16 字节十六进制，对 `type`、`severity`、`message`、`snapshot` 里的 pid 与 `started_at` 做哈希。同一条逻辑事件重复写入时 ID 相同 |
| 不改 | 不重写历史文件。`schema_version` 仍是 `"1"`。`monview` 不认识该字段时必须忽略，C1 的解析结构可同时收下这个字段并原样返回 |
| 测试 | `monitor/events_test.go`：相同输入得到相同 ID；`message` 变化则 ID 变化。`TestEventFieldsWhitelist` 仍禁止 URI、证书路径、上游地址进入事件 |
| 验收 | 新产生的行能被现有 CLI 读出，旧行没有 `event_id` 也不报错 |

冷却期内 Monitor 本来就不会重复写。ID 解决的是 Agent 重启后再次读到同一行，以及轮转后跨文件识别。

### C3 Monitor 写证书即将过期事件

状态：已实施（`monitor/certs.go`，`TestCertExpiringOnlyWarnsNearCert`、`TestCertReadErrorDoesNotPanic`）。未配置 `cert_files` 时不检查。

| 项 | 内容 |
| --- | --- |
| 影响 | 无请求路径。Monitor 仍不 import `config`、`certmanager`、`cmd` |
| 改动 | `monitor/config.go` 增加可选 `cert_files`（路径列表）和 `cert_warn_days`（默认 30）。`monitor` 自己用标准库解析 PEM 的 `NotAfter`。低于阈值写事件 `certificate_expiring`，`severity` 为 `warning`，`message` 只含文件名和剩余天数，不含 PEM 正文。同一文件、同一 `NotAfter` 在冷却期内只写一次 |
| 不改 | 不读 `fast-https.json`，避免 Monitor 和站点配置绑死。路径由部署配置显式列出。不写 `certificate_renew_failed`：Server 今天没有对外的续期失败记录 |
| 测试 | 临时目录放一张将过期和一张未过期的证书。只对将过期的那张出一条事件。解析失败写 `collect_error` 风格的证书读取失败，不 panic |
| 验收 | `go test ./monitor/` 通过。`tests/arch` 的 monitor 层仍然只允许 import 自己 |

`cert list` 继续保留。C3 只是让 Agent 的事件尾读能看到证书，不必等它自己再调 CLI。两边阈值各自配置，文档写明默认都是 30 天。

### C4 `/runtime` 增加 502 与 504 累计

状态：已实施（`runtime/metrics/requests.go`，`TestGatewayStatusBuckets`）。`schema_version` 仍为 `"1"`。

| 项 | 内容 |
| --- | --- |
| 影响 | 有，很小。`RequestEnd` 在已经解析出状态码之后，对 502、504 各多一次原子加 |
| 改动 | `runtime/metrics/requests.go` 增加 `responses_502_total`、`responses_504_total`。`runtime/metrics/snapshot.go` 在 5xx 合计旁边输出这两个字段。500、503 和其他 5xx 仍然只进 `responses_5xx_total` |
| 不改 | 不改变 2xx/3xx/4xx/5xx 的口径。不把每个状态码都做成映射。`schema_version` 仍为 `"1"` |
| 测试 | `runtime/events/request_metrics_test.go` 补一条：响应 502 与 504 时两个新计数加一，5xx 合计也加一；200 不加这两个计数 |
| 验收 | `go test ./runtime/metrics/ ./runtime/events/` 与 `management/rtclient` 通过。旧客户端忽略新字段 |

Monitor 的 `5xx_ratio` 继续用 5xx 合计。502/504 的比例规则放到 C4 稳定之后再加，避免同一次改动既碰热路径又改告警。

### C5 按上游地址累计失败

状态：已实施（`modules/proxy/metrics.go`，`TestDialFailSplitByAddress`、`TestMetricAddrDropsUserinfo`）。总和仍在，最多列出 64 个地址。不能据此摘除节点。

| 项 | 内容 |
| --- | --- |
| 影响 | 有。拨号失败、写超时、读超时发生在代理路径上，计数从进程级总和改为按地址一份 |
| 改动 | `modules/proxy/metrics.go`：`Stats` 增加 `upstreams` 数组，元素为 `address`、`dial_fail_total`、`send_timeout_total`、`read_timeout_total`。现有四个总和字段保留，等于各地址之和。地址用 `proxy_pass` 的主机端口字符串，不带路径、不带用户信息。`noteDialFail`、`noteSendTimeout`、`noteReadTimeout` 增加地址参数，调用点在 `modules/proxy` 内 |
| 不改 | 不引入健康检查，不摘除节点，不改变失败时返回给客户端的状态码。地址数量用固定上限（例如 64），超出的并进总和但不单列，避免无界 map |
| 测试 | 两个不同 `proxy_pass` 各失败一次，快照里两条地址计数分开，总和为 2。地址串里若有用户信息，快照中不出现 |
| 验收 | `go test ./modules/proxy/ ./runtime/metrics/` 通过。`/runtime` 在没有代理流量时 `upstreams` 省略或为空数组，总和仍为 0 |

这只改善诊断。不能据此实现「摘除节点」。

### C6 多上游与受控摘除

状态：已实施首版。`proxy_pass` 字符串保持不变；数组按轮询，失败再试下一个。`upstream drain` 只写摘除名单，不改配置文件；最后一个地址拒绝摘除。不做权重、会话保持和主动探测。`healthy` 在该地址出现过拨号失败或超时后为 false，进程重启后清零。

| 项 | 内容 |
| --- | --- |
| 影响 | 大。改变反向代理的选路 |
| 前置 | 已有未实施计划 [upstream-failover-plan.md](upstream-failover-plan.md)。本步不另写一套选路，只规定 Agent 能看见、能调用的那一层 |
| 改动 | 在故障转移计划落地之后追加三件，缺一不可：`/runtime` 的每个上游地址带 `healthy`（由该计划的被动健康结果填充，不另做探测）；CLI `upstream drain <address> --yes --request-id` 与 `upstream undrain`，走 `management/txn` 或等价的受控写路径，写 operation；摘除前若该地址是配置里最后一个可用地址，命令失败，退出码 6，不改配置 |
| 不改 | Agent 不直接改 `proxy_pass` 文件。没有替代地址时不允许自动摘除 |
| 测试 | 单地址配置调用 drain 失败且文件不变。两地址时 drain 一个，另一个仍被选中；重复 `request_id` 不摘第二次。失败注入：drain 写到一半，`config rollback` 能回到原配置 |
| 验收 | 故障转移计划自身的测试，加上上述 CLI 测试。在此之前规范 12.1 的自动摘除保持不做 |

### C7 经审批的证书续期通道

| 项 | 内容 |
| --- | --- |
| 影响 | 大。Server 上新增本机写接口 |
| 前置 | CLI 计划 S13 的安全要求。本步是那一节的落地顺序，不降低要求 |
| 改动 | 与 `GET /runtime` 分开的本机监听。令牌文件在状态目录，权限 0600，启动时生成。请求必须带令牌且来自回环。`cert renew --domain <name> --yes --request-id` 把请求转给该接口。响应只含阶段 `obtained`、`written`、`reloaded`，不含私钥和 PEM。域名不在 `certmanager.Plan` 内则 404。失败不替换现有证书文件 |
| 不改 | 不在 `/runtime` 上加写方法。Agent 策略表在本通道完成前保持 DENY |
| 测试 | 无令牌、错令牌、非回环来源均拒绝。同一 `request_id` 不向 CA 发第二次。夹具私钥字符串不出现在响应、日志和 operation |
| 验收 | S13 列出的拒绝用例全部有测试名。未通过前 `cmd/cert.go` 不加一个会真正发 CA 请求的子命令 |

## 3. 顺序

C1 → C2 → C3 → C4 → C5 → C6 → C7。

C1 到 C3 可以在 Agent 进程开工之前做，也可以和 Agent 的只读闭环并行。C4、C5 各算一步，分开提交，提交说明里带上 `go test` 的包名。C6 必须先有故障转移的数据面，再加 CLI。C7 单独评审后再写代码。

每步只改该步列出的包。C4 之前禁止改 `runtime/events` 的请求处理，C5 之前禁止改 `proxy_pass` 的选路。

## 4. 做完时 Agent 侧能多做什么

| 步骤完成后 | Agent 可以多做的事 | 仍然不能做 |
| --- | --- | --- |
| C1、C2 | 用 CLI 拿到事件摘要和稳定 ID | 自动写配置 |
| C3 | 从事件流收到证书即将过期 | 续期 |
| C4、C5 | 诊断里引用 502/504 和分地址失败 | 摘除节点 |
| C6 | 在有替代地址时，经审批调用 drain | 无替代地址时自动摘除 |
| C7 | 经审批调用 `cert renew` | 自主覆盖私钥 |

C1–C5 不改变 [ai-agent-execution-plan.md](ai-agent-execution-plan.md) 里「写操作 DENY」的策略。只有 C6、C7 的命令经过安全评审并有测试之后，才允许把对应动作从 DENY 改成 REQUIRE_APPROVAL。
