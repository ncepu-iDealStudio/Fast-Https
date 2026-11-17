# 测试文档索引

本目录存放「测过什么、结论是什么、怎么复跑」的记录，按用途分三类：
**报告**（结论）、**用例与方案**（怎么测）、**实机验证**（真跑过的结果）。

日常测试命令与测试分层说明见 [develop/1-testing.md](../develop/1-testing.md)。

## 目录结构

```text
docs/test/
├── reports/          评估与核验结论：覆盖率、缺陷反馈核验
├── cases/            怎么测：边界用例集、压力测试方案
└── verification/     实测记录：发行版装包、桌面端手工验证
```

## 文档索引

| 类别 | 文档 | 内容 | 最近结果 |
| --- | --- | --- | --- |
| 报告 | [reports/coverage-assessment.md](reports/coverage-assessment.md) | 覆盖率评估与薄弱模块清单 | 2026-10-25 合并覆盖率 68.5%，35 个包通过 `-race` |
| 报告 | [reports/edge-case-coverage.md](reports/edge-case-coverage.md) | **缺陷反馈核验（第一轮）**：按风险面自查 B-01 ~ B-17，逐条给证据与复跑命令 | 18 条核验：3 条已修复（B-05 / B-08 / B-17），1 条待修（B-12），其余有回归用例锁定 |
| 报告 | [reports/upstream-feedback-coverage.md](reports/upstream-feedback-coverage.md) | **缺陷反馈核验（第二轮）**：对照主流 HTTP 服务器公开 issue/PR，过滤 C 层与 QUIC 专用条目 | 5 条核验、2 处修复 |
| 报告 | [reports/r08-acceptance.md](reports/r08-acceptance.md) | **R-08 验收报告**：连接与超时边界（100-continue、keep-alive、431/413、限额与超时配置） | 5 条验收全部通过，扩展项与待修项清零 |
| 报告 | [reports/r09-acceptance.md](reports/r09-acceptance.md) | **R-09 验收报告**：reload 的原子性与可见性（串行化、在途请求、失败保留旧配置、`in_flight_requests`） | 需求与验收全部通过；真实服务 E2E 已加入 Linux CI，待首次流水线结果回填 |
| 报告 | [reports/r10-acceptance.md](reports/r10-acceptance.md) | **R-10 验收报告**：拒绝行为可观测性（统一字段、枚举与 rule、真实进程 E2E） | 三项验收全部通过；真实进程 E2E 本机 PASS，已加入 CI |
| 报告 | [reports/r11-acceptance.md](reports/r11-acceptance.md) | **R-11 验收报告**：header 超限返回 431（h1 一次到齐/分片、真实 TCP、h2、畸形头仍 400） | 五项验收全部通过，无需新增代码 |
| 报告 | [reports/r12-acceptance.md](reports/r12-acceptance.md) | **R-12 验收报告**：空 body 上游响应的连接复用（HEAD / 204 / 304 / 1xx interim，含完整链路 E2E） | 十三项验收全部通过；两次请求只 accept 一次 |
| 用例 | [cases/boundary-cases.md](cases/boundary-cases.md) | 边界测试用例集：协议边界、注入面、数值边界、并发边界、公开语料 | 用例已全部落库，见文内对应测试文件 |
| 方案 | [cases/stress-test-plan.md](cases/stress-test-plan.md) | 压力测试方案：4C4G3M 环境下的反代与静态资源负载 | 仅方案，压测脚本未实现（见 [v1.5.0-plan](../develop/archive/v1.5.0-plan.md)） |
| 实测 | [verification/distro-packages.md](verification/distro-packages.md) | 发行版安装包实机验证：deb / rpm / apk / 升级迁移 | 2026-11-05 四个场景 ALL-PASS |
| 实测 | [verification/desktop-tls-config.md](verification/desktop-tls-config.md) | 桌面端 TLS 默认证书配置：GUI 写入、清除、成对校验 | 2026-11-17 三项全部符合预期 |
| 实测 | [verification/desktop-proxy-config.md](verification/desktop-proxy-config.md) | 桌面端代理来源配置：写入、恢复默认、非法网段与负链长拦截 | 2026-11-17 四项全部符合预期 |
| 实测 | [verification/desktop-default-config.md](verification/desktop-default-config.md) | 桌面端读取与保存随包主配置：预设值、改四项、非法枚举与负链长 | 2026-11-17 五项全部符合预期 |
| 实测 | [verification/desktop-site-compression.md](verification/desktop-site-compression.md) | 桌面端新增站点压缩默认值：默认、自定义、关闭、代理、非法类型 | 2026-11-17 五项全部符合预期 |

## 测试类型与入口

| 层级 | 命令 | 守什么 | 结论记在哪 |
| --- | --- | --- | --- |
| 单元 / 包内 | `go test ./...`（Windows 加 `-p 1`） | 白盒逻辑、解析边界 | 各包 `*_test.go`；覆盖率见 [coverage-assessment](reports/coverage-assessment.md) |
| 竞态 | `go test -race ./...` | 共享状态、热重载并发 | 同上 |
| 架构守卫 | `go test ./tests/arch` | 分层依赖方向 | [develop/1-testing.md](../develop/1-testing.md) |
| 打包守卫 | `go test ./tests/packaging` | LF 行尾、安装清单、logrotate 覆盖、脚本结构 | 同上 |
| 实机装包 | `sh tests/packaging/distro/run.sh all`（需 docker） | 真装、启动、轮转、卸载 | [verification/distro-packages.md](verification/distro-packages.md) |
| 桌面端 | `cd desktop && go test ./internal/...` | 设置页字段、站点参数翻译、CLI 转交 | [verification/desktop-tls-config.md](verification/desktop-tls-config.md)、[verification/desktop-proxy-config.md](verification/desktop-proxy-config.md)、[verification/desktop-default-config.md](verification/desktop-default-config.md)、[verification/desktop-site-compression.md](verification/desktop-site-compression.md) |
| 前端 | `cd desktop/frontend && npm run typecheck && npm run i18n:check` | 类型、双语字典键对齐 | 同上 |
| 手工验证 | 按 verification/ 下的步骤操作 GUI 或容器 | 图形界面、安装包这类自动化覆盖不到的部分 | `verification/` |

## 最近一次全量检查（2026-11-17）

命令与结果（`-race` 未在本次运行，提交前仍按 [develop/1-testing.md](../develop/1-testing.md) 执行）：

| 命令 | 结果 |
| --- | --- |
| `go test ./...` | 全部 `ok`（含 R-01 ~ R-11、R-13 边缘加固回归） |
| `cd desktop && go test ./internal/...` | `cli` / `configops` / `siteops` / `view` 全部 `ok` |
| `go test ./tests/packaging` | `ok` |
| `cd desktop/frontend && npm run typecheck` | 通过 |
| `cd desktop/frontend && npm run i18n:check` | 两份字典各 624 键，代码引用 291 键，全部覆盖 |
| 桌面端 TLS 配置手工验证 | 写入 / 清除 / 只填一项报错 三项符合预期，见 [verification/desktop-tls-config.md](verification/desktop-tls-config.md) |
| 桌面端代理来源配置验证 | 写入 / 恢复默认 / 非法网段 / 负链长 四项符合预期，见 [verification/desktop-proxy-config.md](verification/desktop-proxy-config.md) |
| 桌面端开箱即用主配置验证 | 十项预设读取 / 改四项保存 / 改回预设 / 非法枚举 / 负链长 五项符合预期，见 [verification/desktop-default-config.md](verification/desktop-default-config.md) |
| 桌面端新增站点压缩默认值验证 | 默认值 / 自定义 / 关闭 / 代理 / 非法类型 五项符合预期，见 [verification/desktop-site-compression.md](verification/desktop-site-compression.md) |

## 历史记录

- 2026-10-25 首轮覆盖评估 + 边界测试：发现并修复裸 CR/LF 头注入三面（`ec425cd`）；清理死代码（DecompressBytesGzip、PathQuery.Del/Set）。覆盖率 50.8% → 68.5%。
- 2026-11-05 发行版实机验证：ubuntu:24.04 / almalinux:9 / alpine:3.21 容器内装包各跑一遍，四个场景 ALL-PASS；同时补上 logrotate 配置与 `tests/packaging/distro/` 脚本，详见 [verification/distro-packages.md](verification/distro-packages.md)。
- 2026-11-17 边缘加固 R-01 ~ R-04 落地：符号链接策略、Windows 保留名/尾点、TLS 兜底证书、同名证书按客户端能力选择；每项都有回归用例，CLI 与桌面端入口同步，见 [edge-case-hardening-plan.md](../develop/archive/edge-case-hardening-plan.md)。
- 2026-11-17 桌面端 TLS 默认证书配置手工验证：GUI 写入成功、两项清空即删 `tls` 段、只填一项被成对校验拦下且配置未改写，详见 [verification/desktop-tls-config.md](verification/desktop-tls-config.md)。
- 2026-11-17 边缘加固 R-05 落地：顶层 `proxy` 段（`trusted_proxies`、`forwarded_for.max_hops`、`preserve_host`），默认只追加真实对端、丢弃客户端伪造的 `X-Forwarded-For`；`modules/proxy/testdata/forwarded-chain.jsonl` 覆盖无 XFF / 客户端伪造 / 多层代理 / 超跳数四类输入，CLI `config proxy` 与桌面端设置页同步。
- 2026-11-17 边缘加固 R-06 落地：代理转发前剥离 `Connection` / `Keep-Alive` / `Proxy-Authenticate` / `Proxy-Authorization` / `Proxy-Connection` / `TE` / `Trailer` / `Transfer-Encoding` / `Upgrade` 及 `Connection` 点名的扩展头；客户端要求关闭时给上游补 `Connection: close`，HTTP/1.0 keep-alive 与 websocket 升级各自重建连接头。`modules/proxy/testdata/hop-by-hop-headers.json` 七类输入由 `TestHopByHopHeaderCases` 逐项核对上游实际收到的头。
- 2026-11-17 桌面端代理来源配置验证：写入三项后 `proxy` 段落盘且 CLI 双入口一致，三项改回默认即删段，负 `max_hops` 与非法 CIDR 均在写盘前被拦下且配置哈希不变，详见 [verification/desktop-proxy-config.md](verification/desktop-proxy-config.md)。
- 2026-11-17 边缘加固 R-07 落地：压缩协商补齐 `zip_min_length` 与 `zip_skip_types`（内置已压缩类型清单，配置后整体替换），修复压缩后 `Content-Length` 仍用原文件大小的问题；`modules/static/testdata/accept-encoding.jsonl` 十条用例由 `TestAcceptEncodingCases` 逐条驱动真实响应链路，CLI `site update --zip-min-length/--zip-skip-types` 与桌面端站点编辑同步。
- 2026-11-17 桌面端开箱即用主配置验证：`ReadConfigForm` 读到十项预设值，保存四项后其余预设不被重置，改回预设后内容一致（JSON 被规范化重写），非法 WAF 模式在表单层拦截、负链长在配置校验层拦截且配置哈希不变，详见 [verification/desktop-default-config.md](verification/desktop-default-config.md)。
- 2026-11-17 桌面端新增站点压缩默认值验证：默认值写入 br+gzip / 1 KB / 整类跳过清单，自定义只写显式项，取消勾选后无 `zip*` 键，代理站点写入同一组默认值，非法跳过类型在写盘前被 `USAGE_ERROR` 拦下，详见 [verification/desktop-site-compression.md](verification/desktop-site-compression.md)。
- 2026-11-17 边缘加固 R-08 验收：`Expect: 100-continue`（含 chunked / 列表 / 417 / HTTP1.0）、keep-alive 请求与响应状态隔离、header 431 与 body 413（Content-Length / chunked / h2）、限额与超时配置（`max_*`、`read/write/body_timeout`、`proxy.upstream_timeout`、`proxy.tunnel_idle_timeout`）全部落地；新增真实 TCP E2E 与 `-race` 用例，详见 [reports/r08-acceptance.md](reports/r08-acceptance.md)。
- 2026-11-17 边缘加固 R-09 落地：并发 reload 串行化（`server.Reload` 与 loader 各持互斥锁），在途请求按端口变化区分「旧快照跑完」与「移除/重建时终止」，reload 日志带 `port` 与 `in_flight_requests`；新增 loader/server 并发 10 次 reload 的 `-race` 用例与在途计数用例，详见 [edge-case-hardening-plan.md](../develop/archive/edge-case-hardening-plan.md) 与 [4-reload.md](../feature/4-reload.md)。
- 2026-11-17 边缘加固 R-09 验收：需求与两条验收项全部通过；`/runtime` 新增 `listeners[].in_flight_requests`，reload 与 graceful stop 共用锁并跳过停止后的 reload；真实服务 E2E 因 Windows 不支持定向投递 reload 信号，已加入 Gitee Go Linux 流水线，待首次结果回填，详见 [reports/r09-acceptance.md](reports/r09-acceptance.md)。
- 2026-11-17 边缘加固 R-10 落地：所有拒绝统一写 `reject_reason` / `rule` / `remote_ip`，覆盖路径穿越（`path_traversal`）、Windows 非法路径（`invalid_path`）、隐藏路径（`hidden_path`）、符号链接策略（`symlink_rejected`）、超限（`header_too_large` / `body_too_large`，h1+h2）、未知 SNI（`unknown_sni`，safe.log 与控制台双写）与压缩跳过（`compression_skipped`：配置跳过和 `accept_encoding`，静态与代理，debug）；黑名单/限流/location 限流/WAF/host_check 补同一组字段。拒绝日志不记原始路径。`logs query` 新增 `--reject-reason`，记录输出带三个字段，桌面端同步，详见 [16-logging.md](../feature/16-logging.md)。
- 2026-11-17 边缘加固 R-10 验收：三项验收全部通过；真实进程 `TestRejectLogE2E` 覆盖穿越/超限/两种压缩跳过/未知 SNI 并用 `logs query --reject-reason` 查回，已加入 CI；host_check warn/enforce、代理压缩跳过与 Windows 非法路径（`invalid_path`）补齐；桌面端日志页提供拒绝原因枚举筛选与原因/规则展示列，详见 [reports/r10-acceptance.md](reports/r10-acceptance.md)。

## 旧文件名对照

早期文档按 0 ~ 5 编号，2026-11-17 按类别重组。文件名去掉了编号，正文标题与章节号（如 `1.8`、`4.2`、`5.6`）保持原样，方便与历史提交、旧链接和文内交叉引用对应。

| 旧路径 | 新路径 |
| --- | --- |
| `docs/test/0-coverage-assessment.md` | `docs/test/reports/coverage-assessment.md` |
| `docs/test/1-boundary-cases.md` | `docs/test/cases/boundary-cases.md` |
| `docs/test/2-stress-test-plan.md` | `docs/test/cases/stress-test-plan.md` |
| `docs/test/3-distro-verification.md` | `docs/test/verification/distro-packages.md` |
| `docs/test/4-edge-case-coverage.md` | `docs/test/reports/edge-case-coverage.md` |
| `docs/test/5-upstream-feedback-coverage.md` | `docs/test/reports/upstream-feedback-coverage.md` |

## 约定

- 本目录只放「结论」和「怎么复跑」，不复制代码；每条结论必须给出可执行的复跑命令或测试名。
- 新增验证记录：放到对应的 `verification/` 文件，或按主题新建一篇并在上面的索引表登记。
- 一次验证的结果要写清**环境、时间、产物版本/校验和、逐项结论**，方便判断是否需要重跑。
