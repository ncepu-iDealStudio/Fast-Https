# R-10 拒绝行为的可观测性 —— 验收报告

- 日期：2026-11-17
- 范围：`docs/develop/plans/edge-case-hardening-plan.md` 的 R-10，以及同一条链路上的安全类拒绝与代理压缩跳过
- 结论：**通过**。统一字段、查询链路与真实进程 E2E 全部落地。

## 1. 验收依据

计划中 R-10 的需求与验收：

1. 被拒绝的请求留下结构化日志字段：`reject_reason`、`rule`、`remote_ip`
2. 日志不得出现完整请求路径的原始编码，只记归一化后的片段
3. 验收：`logquery` 能按 `reject_reason` 过滤出这些记录

## 2. 逐条验收结果

| 编号 | 验收项 | 结果 | 关键证据 |
| --- | --- | --- | --- |
| R10-1 | 结构化字段 | ✅ | `infra/logging.RejectWarn` / `RejectDebug`；`TestRejectLogsCarryUnifiedFields`、`TestPathRejectLogsCarryUnifiedFields`、`TestSymlinkRejectLogCarriesUnifiedFields`、`TestLogRejectCarriesUnifiedFields`、`TestHostCheckCarriesUnifiedFields`、`TestCompressionSkipLogs` |
| R10-2 | 不记录原始路径 | ✅ | 拒绝日志不写 `path`；`TestRejectLogE2E` 断言穿越记录里没有 `etc/passwd`，`TestRejectLogsCarryUnifiedFields` 断言 safe 日志无 `path` 字段 |
| R10-3 | 按 `reject_reason` 查询 | ✅ | `logquery` 的 `RejectReason` 过滤、`logs query --reject-reason`、桌面端参数；`TestRejectReasonFilter`、`TestLogsQueryFiltersByRejectReason`、`TestRejectLogE2E` |

## 3. 字段与枚举

日志固定字段：

| 字段 | 内容 |
| --- | --- |
| `reject_reason` | 枚举值，见下表 |
| `rule` | 命中的配置项 / 规则名 |
| `remote_ip` | 客户端 IP，不带端口 |
| `reason` | 安全类保留的既有字段，兼容 `--reason` 查询 |

拒绝类型与 rule：

| 场景 | reject_reason | rule | 级别 | 日志流 |
| --- | --- | --- | --- | --- |
| 路径穿越 | `path_traversal` | `static_root` | warn | safe |
| 隐藏路径 | `hidden_path` | `hidden_path_segment` | warn | safe |
| 符号链接策略（reject） | `symlink_rejected` | `symlink_policy` | warn | safe |
| header 超限（h1/h2） | `header_too_large` | `max_header_size` | warn | safe |
| body 超限（h1/h2） | `body_too_large` | `max_body_size` | warn | safe |
| 未知 SNI | `unknown_sni` | `tls.sni` | warn | safe + error |
| 黑名单 | `blacklist` | `security.blacklist` | warn | safe |
| 限流 | `rate_limit` | `security.rate_limit` | warn | safe |
| location 限流 | `location_limit` | `location.limit` | warn | safe |
| WAF | `waf` | `security.waf` | warn | safe |
| Host 校验（warn/enforce） | `host_check` | `security.host_check` | warn | safe |
| 压缩跳过：文件太小 | `compression_skipped` | `zip_min_length` | debug | safe |
| 压缩跳过：已压缩类型 | `compression_skipped` | `zip_skip_types` | debug | safe |
| 压缩跳过：客户端没要 | `compression_skipped` | `accept_encoding` | debug | safe |

压缩跳过覆盖静态文件与代理响应两条路径，rule 命名一致。

## 4. 查询链路

```bash
# 安全日志里的拒绝
fast-https logs query --kind safe --reject-reason path_traversal --json
fast-https logs query --kind safe --reject-reason body_too_large --json

# 未知 SNI 同时写 safe 与 error，两种 kind 都能查
fast-https logs query --kind safe --reject-reason unknown_sni --json
fast-https logs query --kind error --reject-reason unknown_sni --json

# 安全类拒绝：新字段与既有 --reason 等价
fast-https logs query --kind safe --reject-reason blacklist --json
fast-https logs query --kind safe --reason blacklist --json
```

返回记录新增 `reject_reason`、`rule`、`remote_ip`，与既有白名单字段一起脱敏、截断。

桌面端日志页：

- 高级筛选的「拒绝原因」是枚举下拉：`全部` + 12 个 `reject_reason`，选中后翻译为 `--reject-reason`。
- 表格新增 reason 列，显示 `reject_reason · rule`；悬浮 `title` 显示 `remote_ip`。
- `logEntry` 映射三个字段，`LogEntry`（Go/TS）同步；窄窗口自动收起该列。

## 5. 测试执行记录

包级用例：

```text
PASS: TestRejectLogsCarryUnifiedFields              (runtime/events)
PASS: TestPathRejectLogsCarryUnifiedFields          (modules/static)
PASS: TestSymlinkRejectLogCarriesUnifiedFields      (modules/static)
PASS: TestCompressibleReason                        (modules/static)
PASS: TestCompressionSkipLogs                       (modules/proxy)
PASS: TestLogRejectCarriesUnifiedFields             (modules/safe)
PASS: TestHostCheckCarriesUnifiedFields (warn/enforce) (runtime/filters)
PASS: TestRejectReasonFilter                        (management/logquery)
PASS: TestLogsQueryFiltersByRejectReason            (cmd)
PASS: TestLogsQueryArgs                             (desktop/internal/view)
PASS: TestLogEntryMapping                           (desktop/internal/view)
```

真实进程 E2E：

- 命令：`FASTHTTPS_E2E_REJECT_LOG=1 go test ./tests/client_test -run TestRejectLogE2E -v -count=1`
- 结果：

```text
=== RUN   TestRejectLogE2E
--- PASS: TestRejectLogE2E (3.15s)
PASS
```

E2E 在临时目录起真实进程（HTTP + TLS），触发路径穿越、header 超限、body 超限、
压缩跳过（配置与 `accept_encoding` 两种）与未知 SNI，再用同一二进制的
`logs query --reject-reason` 逐项查回字段。

全量与竞态：

- `go test -p 1 ./...`：全绿
- `go test -race ./modules/safe ./modules/appfirewall ./modules/static ./modules/proxy ./runtime/filters ./runtime/listener ./tests/client_test`：通过
- 桌面端 `cd desktop && go test ./internal/...`：通过
- `cd desktop/frontend && npm run typecheck`、`npm run i18n:check`：通过（624 键 / 引用 291 键）

## 6. 已知偏差与限制

1. **级别区分**：真正拒绝用 warn；压缩跳过用 debug，量级与正常请求相同，默认只在排障时关注。
2. **协议语义跳过不记录**：HEAD、304/204/206、上游已压缩、`Content-Range`、空 body、websocket 升级等不属于“配置或协商跳过”，不写 `compression_skipped`。
3. **未知 SNI 双写**：safe.log 一条 + 控制台 warn（镜像到 error.log）一条；这是为了保留原有运维可见性，代价是同一次握手在两个文件各有一行。
4. **`remote_addr` 保留**：安全类日志同时保留 `remote_addr`（既有字段）与 `remote_ip`（R-10 契约），两者内容相同。
5. **字段与路径**：日志只记 `reject_reason` / `rule` / `remote_ip`，不记原始请求路径；排查具体 URI 仍需结合访问日志。

## 7. 提交记录

| 提交 | 内容 |
| --- | --- |
| `d4d1344` | 统一字段与四类数据面拒绝；`logquery` / CLI / 桌面端过滤 |
| `76e3c02` | 桌面端日志页「拒绝原因」筛选入口与 i18n |
| `0e358c7` | 真实进程 E2E `TestRejectLogE2E`，并加入 CI |
| `32f84a3` | 安全类拒绝、symlink、accept_encoding、unknown_sni 补统一字段 |
| `6ad71e8` | host_check enforce 与代理压缩跳过日志 |
| `680e621` | 桌面端日志表展示拒绝原因与规则，筛选改为枚举下拉 |

## 8. 关键文件

- `infra/logging/reject.go`、`consts.go`：字段契约与 `RejectWarn` / `RejectDebug` / `RemoteIP`
- `runtime/events/events.go`、`events_h2.go`：header/body 超限
- `modules/static/static.go`、`compression.go`、`symlink.go`：穿越、隐藏路径、符号链接、压缩跳过
- `modules/proxy/compress.go`：代理响应压缩跳过
- `runtime/listener/certs.go`：未知 SNI
- `modules/safe/safe.go`、`runtime/filters/filters.go`、`modules/appfirewall/waf.go`：安全类拒绝
- `management/logquery/query.go`、`cmd/internal/inspectcmd/logs.go`、`desktop/internal/view/state.go`
- `tests/client_test/reject_log_e2e_test.go`：真实链路验证
